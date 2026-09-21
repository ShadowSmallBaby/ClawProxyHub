// stub — 测试用插件：按固定剧本回吐事件流，用于端到端验证核心链路。
package main

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/ShadowSmallBaby/ClawProxyHub/sdk"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

func main() { sdk.Serve(&stubPlugin{}) }

type stubPlugin struct {
	pb.UnimplementedClawPluginServer
	host *sdk.Host
}

// SetHost 接收核心注入的宿主回调。
func (s *stubPlugin) SetHost(host *sdk.Host) { s.host = host }

func (s *stubPlugin) Handshake(ctx context.Context, req *pb.HandshakeRequest) (*pb.HandshakeResponse, error) {
	if req.ProtocolVersion != sdk.ProtocolVersion {
		return &pb.HandshakeResponse{
			Error: &pb.Error{
				Code:    1,
				Message: fmt.Sprintf("protocol mismatch: core=%d plugin=%d", req.ProtocolVersion, sdk.ProtocolVersion),
			},
		}, nil
	}
	return &pb.HandshakeResponse{
		Manifest: &pb.Manifest{
			Name:            "stub",
			Version:         "0.1.0",
			Author:          "cph",
			Label:           map[string]string{"zh": "测试插件", "en": "Stub"},
			ProtocolVersion: sdk.ProtocolVersion,
			Capabilities:    []string{"chat", "models", "tasks"},
			Endpoints:       []string{"chat_completions", "messages", "responses"},
			AuthMethods: []*pb.AuthMethod{
				{
					Id: "auth_file", Label: map[string]string{"zh": "认证文件", "en": "Auth File"},
					Capabilities: []string{"refreshable", "profile"},
					Fields: []*pb.AuthField{
						{Name: "content", Label: map[string]string{"zh": "auth.json 内容"},
							Type: "textarea", Required: true, Placeholder: `{"token": "..."}`},
					},
				},
				{
					Id: "phone_otp", Label: map[string]string{"zh": "手机验证码", "en": "Phone OTP"},
					Capabilities: []string{"refreshable", "auto_relogin", "profile"},
					Fields: []*pb.AuthField{
						{Name: "phone", Label: map[string]string{"zh": "手机号"}, Type: "phone", Required: true},
					},
				},
				{
					Id: "oauth", Label: map[string]string{"zh": "浏览器登录", "en": "Browser OAuth"},
					Capabilities: []string{"refreshable", "profile"},
					Fields:       []*pb.AuthField{},
				},
			},
		},
	}, nil
}

func (s *stubPlugin) ListModels(ctx context.Context, cred *pb.CredentialBlob) (*pb.ModelList, error) {
	return &pb.ModelList{
		Models: []*pb.ModelInfo{
			{Id: "stub-mini", Label: map[string]string{"en": "Stub Mini"},
				ContextWindow: 8192, SupportsTools: true, SupportsStream: true},
			{Id: "stub-pro", Label: map[string]string{"en": "Stub Pro"},
				ContextWindow: 128000, SupportsTools: true, SupportsStream: true},
		},
	}, nil
}

// Chat 按剧本回吐：开场 → 文本增量 → （可选）工具调用 → 收尾。
// 凭据含 EXPIRED 标记时返回 401（换号重试链路验证）。
func (s *stubPlugin) Chat(req *pb.ChatRequest, stream pb.ClawPlugin_ChatServer) error {
	if cred := req.GetCredential(); cred != nil && strings.Contains(string(cred.Blob), "EXPIRED") {
		return stream.Send(&pb.StreamEvent{Event: &pb.StreamEvent_TaskFailed{
			TaskFailed: &pb.TaskFailed{Error: &pb.Error{Code: 401, Message: "credential expired"}},
		}})
	}
	if s.host != nil {
		s.host.Log("info", fmt.Sprintf("chat model=%s stream=%v msgs=%d", req.Model, req.Stream, len(req.Messages)))
	}
	send := func(ev *pb.StreamEvent) error { return stream.Send(ev) }

	if err := send(&pb.StreamEvent{Event: &pb.StreamEvent_MessageStart{
		MessageStart: &pb.MessageStart{Model: req.Model},
	}}); err != nil {
		return err
	}

	reply := fmt.Sprintf("你好，我是 stub 插件，模型 %s，收到 %d 条消息，最后一条：%q",
		req.Model, len(req.Messages), lastUserText(req))
	for _, seg := range splitRunes(reply) {
		if err := send(&pb.StreamEvent{Event: &pb.StreamEvent_ContentDelta{
			ContentDelta: &pb.ContentDelta{Text: seg},
		}}); err != nil {
			return err
		}
	}

	// 请求里带工具时回一个假调用，验证工具链路
	if len(req.Tools) > 0 {
		if err := send(&pb.StreamEvent{Event: &pb.StreamEvent_ToolCallDelta{
			ToolCallDelta: &pb.ToolCallDelta{
				Id: "call_stub_1", Name: req.Tools[0].Name,
				ArgumentsDelta: "{}",
			},
		}}); err != nil {
			return err
		}
	}

	return send(&pb.StreamEvent{Event: &pb.StreamEvent_MessageFinish{
		MessageFinish: &pb.MessageFinish{
			FinishReason: finishReason(len(req.Tools) > 0),
			Usage:        &pb.Usage{InputTokens: 12, OutputTokens: int64(len(reply))},
		},
	}})
}

func finishReason(hasTools bool) string {
	if hasTools {
		return "tool_calls"
	}
	return "stop"
}

func lastUserText(req *pb.ChatRequest) string {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			return req.Messages[i].Text
		}
	}
	return ""
}

// splitRunes 按字符切块，模拟真实模型的增量输出。
func splitRunes(s string) []string {
	segs := make([]string, 0, len(s)/8+1)
	runes := []rune(s)
	for i := 0; i < len(runes); i += 8 {
		end := min(i+8, len(runes))
		segs = append(segs, string(runes[i:end]))
	}
	return segs
}

// Login 多步登录演示：
//   - auth_file：单步（表单内容即凭据）
//   - phone_otp：两步（手机号 → 验证码，固定码 123456）
//   - oauth：链接 + 粘贴回调（state 存进程内）
func (s *stubPlugin) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResult, error) {
	switch req.MethodId {
	case "auth_file":
		content := req.Form["content"]
		if content == "" {
			return &pb.LoginResult{Error: &pb.Error{Code: 400, Message: "content required"}}, nil
		}
		return loginDone([]byte(content), "authfile-user"), nil

	case "phone_otp":
		if len(req.State) == 0 {
			// 第一步：收手机号，要求验证码
			return &pb.LoginResult{Next: &pb.LoginNextStep{
				Action: "input_form",
				Prompt: map[string]string{"zh": "验证码已发送（测试码 123456）"},
				Fields: []*pb.AuthField{
					{Name: "code", Label: map[string]string{"zh": "验证码"}, Type: "text", Required: true},
				},
				State: []byte("phone:" + req.Form["phone"]),
			}}, nil
		}
		// 第二步：校验验证码
		if req.Form["code"] != "123456" {
			return &pb.LoginResult{Error: &pb.Error{Code: 401, Message: "验证码错误"}}, nil
		}
		phone := string(req.State)[len("phone:"):]
		return loginDone([]byte(`{"phone":"`+phone+`"}`), "phone-user-"+phone), nil

	case "oauth":
		if len(req.State) == 0 {
			return &pb.LoginResult{Next: &pb.LoginNextStep{
				Action: "open_url",
				Url:    "https://example.com/login?state=stub-oauth-state",
				Prompt: map[string]string{"zh": "登录后把跳转到的完整地址粘贴回来"},
				Fields: []*pb.AuthField{
					{Name: "callback_url", Label: map[string]string{"zh": "回调地址"}, Type: "text", Required: true},
				},
				State: []byte("oauth-pending"),
			}}, nil
		}
		cb := req.Form["callback_url"]
		if !strings.Contains(cb, "code=") {
			return &pb.LoginResult{Error: &pb.Error{Code: 401, Message: "回调地址缺少 code 参数"}}, nil
		}
		return loginDone([]byte(`{"oauth":"`+cb+`"}`), "oauth-user"), nil
	}
	return nil, status.Error(codes.NotFound, "unknown auth method: "+req.MethodId)
}

func loginDone(blob []byte, name string) *pb.LoginResult {
	return &pb.LoginResult{
		Blob: blob,
		Profile: &pb.AccountProfile{DisplayName: name, Healthy: true,
			Quota: map[string]string{"credits": "500"}},
	}
}

func (s *stubPlugin) Refresh(ctx context.Context, cred *pb.CredentialBlob) (*pb.RefreshResult, error) {
	return &pb.RefreshResult{}, nil
}

func (s *stubPlugin) GetProfile(ctx context.Context, cred *pb.CredentialBlob) (*pb.AccountProfile, error) {
	return &pb.AccountProfile{DisplayName: "stub-account", Healthy: true}, nil
}

func (s *stubPlugin) ListTaskCapabilities(ctx context.Context, _ *pb.TaskCapabilitiesRequest) (*pb.TaskCapabilities, error) {
	return &pb.TaskCapabilities{
		Capabilities: []*pb.TaskCapability{
			{
				Id: "checkin", Label: map[string]string{"zh": "每日签到"},
				Kind: "recurring", PerAccount: true, DefaultSchedule: "daily 09:00",
			},
		},
	}, nil
}

// RunTask 模拟签到：回写一份新凭据（模拟 token 刷新）+ 摘要。
func (s *stubPlugin) RunTask(ctx context.Context, req *pb.RunTaskRequest) (*pb.RunTaskResponse, error) {
	if req.CapabilityId != "checkin" {
		return nil, status.Error(codes.NotFound, "unknown capability: "+req.CapabilityId)
	}
	if s.host != nil {
		s.host.Log("info", "checkin for account "+req.Credential.GetAccountId())
	}
	return &pb.RunTaskResponse{
		Changed: true,
		Blob:    append(req.Credential.GetBlob(), 0x01), // 模拟凭据刷新
		Summary: "签到成功，积分 +10",
	}, nil
}
