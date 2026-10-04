// output.go — 信封事件 → 协议输出的统一收尾：SSE、聚合、日志。
package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"google.golang.org/protobuf/proto"

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
func (s *Server) streamOut(w http.ResponseWriter, events <-chan *pb.StreamEvent, prefix []*pb.StreamEvent, log *requestLogCtx, enc streamEncoder) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, _ := w.(http.Flusher)

	log.status = http.StatusOK
	terminal := false
	emit := func(ev *pb.StreamEvent) bool {
		if failed := ev.GetTaskFailed(); failed != nil {
			log.status = int(errorStatus(failed.GetError().GetCode()))
			log.errBrief = failed.GetError().GetMessage()
			_, _ = io.WriteString(w, enc.failure(log.errBrief))
			return false
		}
		collectUsage(log, ev)
		if out := enc.convertEvent(ev); out != "" {
			if _, err := io.WriteString(w, out); err != nil {
				log.status = 499
				log.errBrief = "client write failed"
				return false
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		terminal = ev.GetMessageFinish() != nil
		return !terminal
	}
	consumeEvents(log.context(), events, prefix, emit)
	if terminal {
		_, _ = io.WriteString(w, enc.finish())
	}
	log.write(s.db)
}

// nonStreamOut 失败立即交回调用层；调用层先取消旧流再恢复。
func (s *Server) nonStreamOut(w http.ResponseWriter, events <-chan *pb.StreamEvent, prefix []*pb.StreamEvent, log *requestLogCtx, aggr ...aggregate) (failCode int32, brief string) {
	if len(aggr) == 0 {
		return 502, "missing aggregate"
	}
	a := aggr[0]
	consumeEvents(log.context(), events, prefix, func(ev *pb.StreamEvent) bool {
		if failed := ev.GetTaskFailed(); failed != nil {
			failCode = errorStatus(failed.GetError().GetCode())
			brief = failed.GetError().GetMessage()
			return false
		}
		collectUsage(log, ev)
		a.feed(ev)
		return ev.GetMessageFinish() == nil
	})
	if failCode != 0 {
		return failCode, brief
	}
	body, err := json.Marshal(a.result())
	if err != nil {
		return 502, "encode aggregate response: " + err.Error()
	}
	log.status = http.StatusOK
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(body); err != nil {
		log.status = 499
		log.errBrief = "client write failed"
	}
	log.write(s.db)
	return 0, ""
}

func (c *requestLogCtx) context() context.Context {
	if c.ctx != nil {
		return c.ctx
	}
	return context.Background()
}

func errorStatus(code int32) int32 {
	if code >= 400 && code <= 599 {
		return code
	}
	return 502
}
func failureEvent(code int32, message string) *pb.StreamEvent {
	return &pb.StreamEvent{Event: &pb.StreamEvent_TaskFailed{TaskFailed: &pb.TaskFailed{Error: &pb.Error{Code: code, Message: message}}}}
}

// consumeEvents 在终态、取消、写失败或空闲超时后立即退出，不依赖生产方关通道。
func consumeEvents(ctx context.Context, events <-chan *pb.StreamEvent, prefix []*pb.StreamEvent, emit func(*pb.StreamEvent) bool) {
	for _, ev := range prefix {
		if ev != nil && !emit(ev) {
			return
		}
	}
	timer := time.NewTimer(120 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			emit(failureEvent(499, "request canceled"))
			return
		case <-timer.C:
			emit(failureEvent(504, "upstream stream idle timeout"))
			return
		case ev, ok := <-events:
			if !ok {
				emit(failureEvent(502, "upstream ended before a terminal event"))
				return
			}
			if ev == nil {
				continue
			}
			if !emit(ev) {
				return
			}
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(120 * time.Second)
		}
	}
}

// normalizeEvents 校验生命周期与工具参数；旧插件的无 ID 续块沿用最近一次调用。
func normalizeEvents(ctx context.Context, events <-chan *pb.StreamEvent, model string) <-chan *pb.StreamEvent {
	out := make(chan *pb.StreamEvent)
	go func() {
		defer close(out)
		send := func(ev *pb.StreamEvent) bool {
			select {
			case out <- ev:
				return true
			case <-ctx.Done():
				return false
			}
		}
		started := false
		args := map[string]string{}
		names := map[string]string{}
		currentToolID := ""
		size := 0
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-events:
				if !ok {
					send(failureEvent(502, "upstream ended before a terminal event"))
					return
				}
				if ev == nil {
					continue
				}
				if ev.GetTaskFailed() != nil {
					send(failureEvent(errorStatus(ev.GetTaskFailed().GetError().GetCode()), ev.GetTaskFailed().GetError().GetMessage()))
					return
				}
				if ev.GetMessageStart() != nil {
					if started {
						continue
					}
					started = true
				} else if !started {
					started = true
					if !send(&pb.StreamEvent{Event: &pb.StreamEvent_MessageStart{MessageStart: &pb.MessageStart{Model: model}}}) {
						return
					}
				}
				if t := ev.GetToolCallDelta(); t != nil {
					if t.Id == "" {
						if currentToolID == "" {
							send(failureEvent(502, "tool continuation has no call identity"))
							return
						}
						// 只补内部信封，避免改写生产方持有的事件。
						ev = proto.Clone(ev).(*pb.StreamEvent)
						t = ev.GetToolCallDelta()
						t.Id = currentToolID
					}
					currentToolID = t.Id
					args[t.Id] += t.ArgumentsDelta
					if t.Name != "" {
						names[t.Id] = t.Name
					}
					size += len(t.ArgumentsDelta) + len(t.Name) + len(t.Id)
				}
				if d := ev.GetContentDelta(); d != nil {
					size += len(d.Text)
				}
				if d := ev.GetReasoningDelta(); d != nil {
					size += len(d.Text) + len(d.Signature)
				}
				if size > 32<<20 {
					send(failureEvent(502, "upstream response exceeds 32 MiB"))
					return
				}
				if f := ev.GetMessageFinish(); f != nil {
					switch f.FinishReason {
					case "end_turn", "":
						f.FinishReason = "stop"
					case "tool_use":
						f.FinishReason = "tool_calls"
					case "max_tokens":
						f.FinishReason = "length"
					case "stop", "stop_sequence", "tool_calls", "length", "content_filter":
					default:
						send(failureEvent(502, "unsupported upstream finish reason"))
						return
					}
					if f.FinishReason != "length" && f.FinishReason != "content_filter" {
						for id, a := range args {
							if names[id] == "" {
								send(failureEvent(502, "tool call has no name"))
								return
							}
							if a == "" {
								if !send(&pb.StreamEvent{Event: &pb.StreamEvent_ToolCallDelta{ToolCallDelta: &pb.ToolCallDelta{Id: id, ArgumentsDelta: "{}"}}}) {
									return
								}
							} else if !json.Valid([]byte(a)) {
								send(failureEvent(502, fmt.Sprintf("tool %q arguments are invalid JSON", id)))
								return
							}
						}
					}
					send(ev)
					return
				}
				if !send(ev) {
					return
				}
			}
		}
	}()
	return out
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
