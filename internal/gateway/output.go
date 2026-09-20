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
	// finish 流结束后的尾部输出（OpenAI 的 [DONE] 等）。
	finish() string
}

func (s *anthSSEState) finish() string   { return "" }
func (s *openaiSSEState) finish() string { return "data: [DONE]\n\n" }

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

// streamOut 流式：编码写回 + 失败短路 + 日志收尾。first 为已取出的首事件。
func (s *Server) streamOut(w http.ResponseWriter, events chan *pb.StreamEvent, first *pb.StreamEvent, log *requestLogCtx, enc streamEncoder, _ interface{}) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, _ := w.(http.Flusher)

	log.status = http.StatusOK
	filler := &toolIDFiller{}

	emit := func(ev *pb.StreamEvent) bool {
		if failed, ok := ev.Event.(*pb.StreamEvent_TaskFailed); ok && failed.TaskFailed != nil {
			log.status = http.StatusBadGateway
			log.errBrief = failed.TaskFailed.Error.GetMessage()
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

	if first != nil && !emit(first) {
		io.WriteString(w, enc.finish())
		log.write(s.db)
		return
	}
	for ev := range events {
		if !emit(ev) {
			break
		}
	}
	io.WriteString(w, enc.finish())
	log.write(s.db)
}

// nonStreamOut 聚合：完整 JSON 一次写回。first 为已取出的首事件。
// 返回 failCode 非 0：聚合中途失败且响应/日志均未写（凭据失效或可降级错误），
// 交回 serve 层决定恢复重试或收尾；成功路径自行写响应与日志。
func (s *Server) nonStreamOut(w http.ResponseWriter, events chan *pb.StreamEvent, first *pb.StreamEvent, log *requestLogCtx, aggr ...aggregate) (failCode int32, brief string) {
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
	if first != nil && !handle(first) {
		drain(events)
		return failCode, brief
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
			log.input, log.output, log.cached = u.InputTokens, u.OutputTokens, u.CachedTokens
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func errBody(errType string, err error) map[string]interface{} {
	return map[string]interface{}{
		"error": map[string]string{"type": errType, "message": err.Error()},
	}
}

// drain 清空事件通道，让生产者（gRPC 流消费协程）能退出。
func drain(events chan *pb.StreamEvent) {
	for range events {
	}
}
