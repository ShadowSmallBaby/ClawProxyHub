// 输入侧上下文超窗保护：估算 token 并按需裁剪对话中段旧消息，避免长会话被上游硬拒。
package gateway

import (
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

// maxTruncateRounds 单次请求最多裁剪轮数（防止病态输入死循环）。
const maxTruncateRounds = 5

// truncateNotice 裁剪后注入的占位说明，告知模型早期上下文已丢失。
const truncateNotice = "[System: 较早的对话消息已被自动截断以适配模型上下文窗口，部分早期上下文已丢失，请基于剩余对话继续。]"

// estimateTokens 估算信封请求的输入 token 数（字节 / bytesPerToken 粗估，偏大更安全）。
func estimateTokens(req *pb.ChatRequest, bytesPerToken float64) int {
	if bytesPerToken < 1 {
		bytesPerToken = 1
	}
	total := 0
	for _, m := range req.GetMessages() {
		total += messageBytes(m)
	}
	for _, t := range req.GetTools() {
		total += len(t.GetName()) + len(t.GetDescription()) + len(t.GetParametersSchema())
	}
	return int(float64(total) / bytesPerToken)
}

// messageBytes 单条信封消息的估算字节：取结构化字段与原始 JSON 的较大者，覆盖工具调用体量。
func messageBytes(m *pb.EnvelopeMessage) int {
	structured := len(m.GetText()) + len(m.GetToolCallId())
	for _, p := range m.GetParts() {
		structured += len(p.GetText()) + len(p.GetData())
	}
	for _, tc := range m.GetToolCalls() {
		structured += len(tc.GetName()) + len(tc.GetArguments())
	}
	if raw := len(m.GetRaw()); raw > structured {
		return raw
	}
	return structured
}

// truncateForWindow 估算超过 window*ratio 时逐轮裁剪，返回执行轮数。
// window<=0（未知窗口）或未超阈值时不动，返回 0。
func truncateForWindow(req *pb.ChatRequest, window int32, ratio, bytesPerToken float64) int {
	if window <= 0 || ratio <= 0 {
		return 0
	}
	threshold := int(float64(window) * ratio)
	rounds := 0
	for rounds < maxTruncateRounds {
		if estimateTokens(req, bytesPerToken) <= threshold {
			return rounds
		}
		if !truncateOnce(req) {
			return rounds
		}
		rounds++
	}
	return rounds
}

// truncateOnce 删除对话中段最老的一批消息：保留首条 + 注入截断提示 + 保留尾部 ~60%，
// 并跳过保留区开头的孤儿工具结果（其工具调用已被裁掉）。返回是否实际裁剪。
func truncateOnce(req *pb.ChatRequest) bool {
	msgs := req.GetMessages()
	n := len(msgs)
	if n <= 4 {
		return false
	}
	keepFirst := 1
	keepLast := int(float64(n) * 0.6)
	if keepLast < 2 {
		keepLast = 2
	}
	cutEnd := n - keepLast
	if cutEnd <= keepFirst {
		return false
	}
	cutEnd = skipOrphanToolResults(msgs, cutEnd)
	if cutEnd <= keepFirst || cutEnd >= n {
		return false
	}

	out := make([]*pb.EnvelopeMessage, 0, keepFirst+1+(n-cutEnd))
	out = append(out, msgs[:keepFirst]...)
	out = append(out, &pb.EnvelopeMessage{Role: "assistant", Text: truncateNotice})
	out = append(out, msgs[cutEnd:]...)
	req.Messages = out
	return true
}

// skipOrphanToolResults 保留区若以 role=tool 起头，说明其对应的 tool_call 落在被裁区间，
// 会成为孤儿 tool_result 触发上游报错——逐条后移跳过这些消息。
func skipOrphanToolResults(msgs []*pb.EnvelopeMessage, cutEnd int) int {
	for cutEnd < len(msgs) && msgs[cutEnd].GetRole() == "tool" {
		cutEnd++
	}
	return cutEnd
}
