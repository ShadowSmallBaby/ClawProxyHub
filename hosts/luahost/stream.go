// stream.go — 传给 plugin.chat 的 stream 对象：typed 方法 → srv.Send(StreamEvent)，
// 与 Go 插件的 stream.Send(&pb.StreamEvent{...}) 同构。脚本按事件类型调对应方法。
package main

import (
	lua "github.com/yuin/gopher-lua"

	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

// newStreamTable 构造 stream 对象；每个方法收一个事件 table，转 StreamEvent 推给核心。
func newStreamTable(L *lua.LState, srv pb.ClawPlugin_ChatServer) *lua.LTable {
	send := func(ev *pb.StreamEvent) { _ = srv.Send(ev) }
	t := L.NewTable()
	set := func(name string, fn lua.LGFunction) { L.SetField(t, name, L.NewFunction(fn)) }

	set("message_start", func(L *lua.LState) int {
		a := L.CheckTable(1)
		send(&pb.StreamEvent{Event: &pb.StreamEvent_MessageStart{MessageStart: &pb.MessageStart{
			Model: strField(a, "model"), Usage: usageFromField(a, "usage"),
		}}})
		return 0
	})
	set("content_delta", func(L *lua.LState) int {
		send(&pb.StreamEvent{Event: &pb.StreamEvent_ContentDelta{ContentDelta: &pb.ContentDelta{
			Text: strField(L.CheckTable(1), "text"),
		}}})
		return 0
	})
	set("reasoning_delta", func(L *lua.LState) int {
		a := L.CheckTable(1)
		send(&pb.StreamEvent{Event: &pb.StreamEvent_ReasoningDelta{ReasoningDelta: &pb.ReasoningDelta{
			Text: strField(a, "text"), Signature: strField(a, "signature"),
		}}})
		return 0
	})
	set("tool_call_delta", func(L *lua.LState) int {
		a := L.CheckTable(1)
		send(&pb.StreamEvent{Event: &pb.StreamEvent_ToolCallDelta{ToolCallDelta: &pb.ToolCallDelta{
			Id: strField(a, "id"), Name: strField(a, "name"), ArgumentsDelta: strField(a, "arguments_delta"),
		}}})
		return 0
	})
	set("message_finish", func(L *lua.LState) int {
		a := L.CheckTable(1)
		send(&pb.StreamEvent{Event: &pb.StreamEvent_MessageFinish{MessageFinish: &pb.MessageFinish{
			FinishReason: strField(a, "finish_reason"), Usage: usageFromField(a, "usage"), StopSequence: strField(a, "stop_sequence"),
		}}})
		return 0
	})
	set("failed", func(L *lua.LState) int {
		a := L.CheckTable(1)
		send(&pb.StreamEvent{Event: &pb.StreamEvent_TaskFailed{TaskFailed: &pb.TaskFailed{
			Error: &pb.Error{Code: int32(numField(a, "code")), Message: strField(a, "message")},
		}}})
		return 0
	})
	return t
}
