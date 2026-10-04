// sse.go — 复用 SDK 解析器，保留工具身份、用量和失败终态。
package main

import (
	"bufio"
	"encoding/json"
	"github.com/ShadowSmallBaby/ClawProxyHub/sdk/openaiup"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
	lua "github.com/yuin/gopher-lua"
	"google.golang.org/protobuf/encoding/protojson"
	"io"
)

func streamOpenAISSE(L *lua.LState, streamTbl *lua.LTable, body io.Reader) {
	parser := openaiup.NewParser(func(ev *pb.StreamEvent) {
		data, err := (protojson.MarshalOptions{UseProtoNames: true}).Marshal(ev)
		if err != nil {
			L.RaiseError("stream: %v", err)
		}
		var fields map[string]interface{}
		if err = json.Unmarshal(data, &fields); err != nil {
			L.RaiseError("stream: %v", err)
		}
		for method, value := range fields {
			if method == "task_failed" {
				method = "failed"
				value = value.(map[string]interface{})["error"]
			}
			fn := L.GetField(streamTbl, method)
			if fn == lua.LNil {
				continue
			}
			if err := L.CallByParam(lua.P{Fn: fn, NRet: 0, Protect: true}, goToLua(L, value)); err != nil {
				L.RaiseError("stream: %v", err)
			}
		}
	})
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		parser.Feed(scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		parser.FinishWithError(502, err.Error())
		return
	}
	parser.Finish()
}
