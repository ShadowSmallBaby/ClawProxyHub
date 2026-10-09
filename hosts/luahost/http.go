// http.go — cph.http：出站请求。luahost 是受信 Go 宿主进程，直接用 net/http；Lua 侧无裸 socket。
// request 一次性取回；stream 走 SSE，format="openai" 时由宿主解析并驱动 stream 对象。
package luahost

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/ShadowSmallBaby/ClawProxyHub/sdk"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
	"io"
	"net/http"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"
)

type credentialContextKey struct{}

func luaContext(L *lua.LState) context.Context {
	if ctx := L.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}
func bindCredential(L *lua.LState, c *pb.CredentialBlob) {
	L.SetContext(context.WithValue(luaContext(L), credentialContextKey{}, c))
}
func luaHTTPClient(L *lua.LState) *http.Client {
	cred, _ := luaContext(L).Value(credentialContextKey{}).(*pb.CredentialBlob)
	return sdk.UpstreamClient(sdk.ProxyURL(cred.GetProxy()))
}

func nowMillis() int64 { return time.Now().UnixMilli() }

// newHTTPModule 建 cph.http 子表：request / stream。
func newHTTPModule(L *lua.LState) *lua.LTable {
	return tableOf(L, map[string]lua.LGFunction{
		"request": cphHTTPRequest,
		"stream":  cphHTTPStream,
	})
}

// buildRequest 从 opts table 构造请求：method 默认 GET；body 为 string 直发，为 table 则 JSON 编码。
func buildRequest(opts *lua.LTable) (*http.Request, error) {
	method := strField(opts, "method")
	if method == "" {
		method = "GET"
	}
	var body io.Reader
	jsonBody := false
	switch b := opts.RawGetString("body").(type) {
	case lua.LString:
		if string(b) != "" {
			body = strings.NewReader(string(b))
		}
	case *lua.LTable:
		data, err := json.Marshal(luaToGo(b))
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(data)
		jsonBody = true
	}
	req, err := http.NewRequest(strings.ToUpper(method), strField(opts, "url"), body)
	if err != nil {
		return nil, err
	}
	if h, ok := opts.RawGetString("headers").(*lua.LTable); ok {
		h.ForEach(func(k, v lua.LValue) { req.Header.Set(k.String(), v.String()) })
	}
	if jsonBody && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

// cphHTTPRequest(opts) → resp 表 {body, status, headers}（镜像 Go http.Response）。
// 成功（任何 HTTP 状态码）回表；请求未完成（DNS/连接/TLS/超时/坏 URL）raise，脚本用 pcall 接。
// 支持 opts.timeout（毫秒）：走 context 截止，复用连接池。
func cphHTTPRequest(L *lua.LState) int {
	opts := L.CheckTable(1)
	req, err := buildRequest(opts)
	if err != nil {
		L.RaiseError("http.request: %v", err)
	}
	req = req.WithContext(luaContext(L))
	if ms := int(numField(opts, "timeout")); ms > 0 {
		ctx, cancel := context.WithTimeout(luaContext(L), time.Duration(ms)*time.Millisecond)
		defer cancel()
		req = req.WithContext(ctx)
	}
	if req.Context() == context.Background() {
		req = req.WithContext(luaContext(L))
	}
	client := luaHTTPClient(L)
	defer client.CloseIdleConnections()
	resp, err := client.Do(req)
	if err != nil {
		L.RaiseError("http.request: %v", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, (32<<20)+1))
	if err != nil || len(data) > 32<<20 {
		L.RaiseError("http.request: unreadable or oversized response")
	}
	t := L.NewTable()
	t.RawSetString("body", lua.LString(string(data)))
	t.RawSetString("status", lua.LNumber(resp.StatusCode))
	hdr := L.NewTable()
	for k := range resp.Header {
		hdr.RawSetString(k, lua.LString(resp.Header.Get(k)))
	}
	t.RawSetString("headers", hdr)
	L.Push(t)
	return 1
}

// cphHTTPStream(opts) → (true) | (false, err)。format="openai" 时宿主解析 SSE 驱动 opts.stream；
// 4xx/5xx 回 (false, "HTTP <code>: <body>")，供脚本 retry_status 解析状态码。
func cphHTTPStream(L *lua.LState) int {
	opts := L.CheckTable(1)
	req, err := buildRequest(opts)
	if err != nil {
		L.Push(lua.LFalse)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	if req.Context() == context.Background() {
		req = req.WithContext(luaContext(L))
	}
	client := luaHTTPClient(L)
	defer client.CloseIdleConnections()
	resp, err := client.Do(req)
	if err != nil {
		L.Push(lua.LFalse)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		L.Push(lua.LFalse)
		L.Push(lua.LString(fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(b))))
		return 2
	}
	if strField(opts, "format") == "openai" {
		if st, ok := opts.RawGetString("stream").(*lua.LTable); ok {
			streamOpenAISSE(L, st, resp.Body)
		}
	}
	L.Push(lua.LTrue)
	return 1
}

// cphTimeSleep(ms) 阻塞当前 VM 指定毫秒（脚本重试退避用）。
func cphTimeSleep(L *lua.LState) int {
	timer := time.NewTimer(time.Duration(L.CheckInt(1)) * time.Millisecond)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-luaContext(L).Done():
		L.RaiseError("sleep canceled")
	}
	return 0
}
