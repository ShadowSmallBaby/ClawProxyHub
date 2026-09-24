// output.go — 信封事件 → 协议输出的统一收尾：SSE、聚合、日志。
package gateway

import (
	"encoding/json"
	"io"
	"net/http"

	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

// streamEncoder 流式编码器：把信封事件编码为协议 SSE 文本。
type streamEncoder interface {
	convertEvent(ev *pb.StreamEvent) string
	// failure 流中途失败时的协议错误事件（客户端据此报错而非静默截断）。
	failure(message string) string
	// finish 流结束后的尾部输出（OpenAI 的 [DONE] 等）。
	finish() string
}

func (s *anthSSEState) finish() string   { return "" }
func (s *openaiSSEState) finish() string { return "data: [DONE]\n\n" }

func (s *anthSSEState) failure(message string) string {
	return anthEvent("error", map[string]interface{}{
		"error": map[string]interface{}{"type": "api_error", "message": message},
	})
}

func (s *openaiSSEState) failure(message string) string {
	return s.chunkRaw(map[string]interface{}{
		"error": map[string]interface{}{"type": "upstream_error", "message": message},
	})
}

func (s *responsesSSEState) failure(message string) string {
	return respEvent("response.failed", map[string]interface{}{
		"response": map[string]interface{}{
			"id": s.respID, "object": "response", "model": s.model, "status": "failed",
			"error": map[string]interface{}{"code": "server_error", "message": message},
		},
	})
}

// aggregate 非流式聚合器。
type aggregate interface {
	feed(ev *pb.StreamEvent)
	result() map[string]interface{}
}

// newEncoder 按协议构造流式编码器。
func newEncoder(protocol, model string) streamEncoder {
	switch protocol {
	case "chat_completions":
		return newOpenAISSEState()
	case "responses":
		return newResponsesSSEState(model)
	default:
		return newAnthSSEState(model)
	}
}

// newAggregate 按协议构造聚合器。
func newAggregate(protocol, model string) aggregate {
	switch protocol {
	case "chat_completions":
		return &openaiAggregate{model: model}
	case "responses":
		return &responsesAggregate{model: model}
	default:
		return &anthAggregate{model: model}
	}
}

// streamOut 流式：编码写回 + 失败短路 + 日志收尾。prefix 为首内容前已缓冲的事件（含首个内容帧）。
func (s *Server) streamOut(w http.ResponseWriter, events chan *pb.StreamEvent, prefix []*pb.StreamEvent, log *requestLogCtx, enc streamEncoder) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, _ := w.(http.Flusher)

	log.status = http.StatusOK
	filler := &toolIDFiller{}

	emit := func(ev *pb.StreamEvent) bool {
		if failed, ok := ev.Event.(*pb.StreamEvent_TaskFailed); ok && failed.TaskFailed != nil {
			log.status = http.StatusBadGateway
			log.errBrief = failed.TaskFailed.Error.GetMessage()
			io.WriteString(w, enc.failure(log.errBrief))
			return false
		}
		filler.fill(ev)
		collectUsage(log, ev)
		if out := enc.convertEvent(ev); out != "" {
			io.WriteString(w, out)
			if flusher != nil {
				flusher.Flush()
			}
		}
		return true
	}

	for _, ev := range prefix {
		if !emit(ev) {
			io.WriteString(w, enc.finish())
			log.write(s.db)
			return
		}
	}
	for ev := range events {
		if !emit(ev) {
			break
		}
	}
	io.WriteString(w, enc.finish())
	log.write(s.db)
}

// nonStreamOut 聚合：完整 JSON 一次写回。prefix 为首内容前已缓冲的事件（含首个内容帧）。
// 返回 failCode 非 0：聚合中途失败且响应/日志均未写（凭据失效或可降级错误），
// 交回 serve 层决定恢复重试或收尾；成功路径自行写响应与日志。
func (s *Server) nonStreamOut(w http.ResponseWriter, events chan *pb.StreamEvent, prefix []*pb.StreamEvent, log *requestLogCtx, aggr ...aggregate) (failCode int32, brief string) {
	if len(aggr) == 0 {
		return 0, ""
	}
	a := aggr[0]
	filler := &toolIDFiller{}

	handle := func(ev *pb.StreamEvent) bool {
		if failed, ok := ev.Event.(*pb.StreamEvent_TaskFailed); ok && failed.TaskFailed != nil {
			// 失败时响应尚未写出：返回码与摘要，恢复/降级决策归 serve 层
			failCode = failed.TaskFailed.Error.GetCode()
			brief = failed.TaskFailed.Error.GetMessage()
			return false
		}
		filler.fill(ev)
		collectUsage(log, ev)
		a.feed(ev)
		return true
	}
	for _, ev := range prefix {
		if !handle(ev) {
			drain(events)
			return failCode, brief
		}
	}
	for ev := range events {
		if !handle(ev) {
			drain(events)
			return failCode, brief
		}
	}
	log.status = http.StatusOK
	writeJSON(w, http.StatusOK, a.result())
	log.write(s.db)
	return 0, ""
}

// toolIDFiller 把工具调用续块的空 id 补成上一个非空 id（openaiup 契约：续块只带
// arguments）。出口层统一补齐，供所有按 id 索引的编码器/聚合器复用。
type toolIDFiller struct{ lastID string }

func (f *toolIDFiller) fill(ev *pb.StreamEvent) {
	tc, ok := ev.Event.(*pb.StreamEvent_ToolCallDelta)
	if !ok || tc.ToolCallDelta == nil {
		return
	}
	if tc.ToolCallDelta.Id != "" {
		f.lastID = tc.ToolCallDelta.Id
		return
	}
	tc.ToolCallDelta.Id = f.lastID // 续块归入上一个调用
}

// collectUsage 从 MessageFinish 事件提取用量。
func collectUsage(log *requestLogCtx, ev *pb.StreamEvent) {
	if fin, ok := ev.Event.(*pb.StreamEvent_MessageFinish); ok && fin.MessageFinish != nil {
		if u := fin.MessageFinish.Usage; u != nil {
			log.input, log.output = u.InputTokens, u.OutputTokens
			log.cached, log.cacheCreation = u.CachedTokens, u.CacheCreationTokens
		}
	}
}

// ---------- 信封 usage → 各协议 usage 对象 ----------
// 信封为 Anthropic 语义（input 不含缓存）；OpenAI 系 prompt/input 总量需把缓存读写合回。

// anthUsage Anthropic usage：四字段直出。
func anthUsage(u *pb.Usage) map[string]interface{} {
	if u == nil {
		u = &pb.Usage{}
	}
	return map[string]interface{}{
		"input_tokens":                u.InputTokens,
		"output_tokens":               u.OutputTokens,
		"cache_read_input_tokens":     u.CachedTokens,
		"cache_creation_input_tokens": u.CacheCreationTokens,
	}
}

// openaiUsage Chat Completions usage：prompt_tokens 含缓存，明细在 prompt_tokens_details。
func openaiUsage(u *pb.Usage) map[string]interface{} {
	if u == nil {
		u = &pb.Usage{}
	}
	prompt := u.InputTokens + u.CachedTokens + u.CacheCreationTokens
	return map[string]interface{}{
		"prompt_tokens":     prompt,
		"completion_tokens": u.OutputTokens,
		"total_tokens":      prompt + u.OutputTokens,
		"prompt_tokens_details": map[string]interface{}{
			"cached_tokens": u.CachedTokens, "cache_write_tokens": u.CacheCreationTokens,
		},
		"completion_tokens_details": map[string]interface{}{"reasoning_tokens": u.ReasoningTokens},
	}
}

// responsesUsage Responses usage：input_tokens 含缓存，明细在 input_tokens_details
// （Codex 从 cached_tokens 读缓存命中）。
func responsesUsage(u *pb.Usage) map[string]interface{} {
	if u == nil {
		u = &pb.Usage{}
	}
	input := u.InputTokens + u.CachedTokens + u.CacheCreationTokens
	return map[string]interface{}{
		"input_tokens":          input,
		"output_tokens":         u.OutputTokens,
		"total_tokens":          input + u.OutputTokens,
		"input_tokens_details":  map[string]interface{}{"cached_tokens": u.CachedTokens},
		"output_tokens_details": map[string]interface{}{"reasoning_tokens": u.ReasoningTokens},
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// errBody 错误响应体：OpenAI 形态的 error 对象 + Anthropic 要求的顶层 type=error（两家 SDK 都能解析）。
func errBody(errType string, err error) map[string]interface{} {
	return map[string]interface{}{
		"type":  "error",
		"error": map[string]string{"type": errType, "message": err.Error()},
	}
}

// drain 清空事件通道，让生产者（gRPC 流消费协程）能退出。
func drain(events chan *pb.StreamEvent) {
	for range events {
	}
}
