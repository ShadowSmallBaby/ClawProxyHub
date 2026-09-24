// Package sdk — sse.go：统一 SSE 出站（行级 SSEParser 契约 + StreamSSE），
// 与 HTTPPost 共享打码日志。语义并入自 shared.ScanSSE：空流/断流报 502。
package sdk

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
)

// SSEParser 消费上游 SSE 行流的解析器契约（各插件方言 parser 实现它）。
// 跨层公共依赖落在 sdk：plugins→core 单向依赖，sdk 不可反向 import plugins。
type SSEParser interface {
	Feed(line string)
	Finish()
	FinishWithError(code int32, msg string)
}

// StreamSSE 发起请求并按行驱动 parser。client 为 nil 时按 req.Proxy 自建。
// 非 200：读回 body、记日志、返回响应且不喂 parser（状态码映射交插件）。
// 200：逐行 Feed；无 data 事件视为空流(502)、断流(502)，正常结束 Finish。
func (h *Host) StreamSSE(ctx context.Context, r HTTPRequest, client *http.Client, parser SSEParser) (*HTTPResponse, error) {
	if r.Method == "" {
		r.Method = "POST"
	}
	if client == nil {
		client = UpstreamClient(ProxyURL(r.Proxy))
	}
	req, err := http.NewRequestWithContext(ctx, r.Method, r.URL, strings.NewReader(string(r.Body)))
	if err != nil {
		return nil, err
	}
	for k, v := range r.Headers {
		req.Header.Set(k, v)
	}
	h.logRequest(r, req.Header)
	resp, err := doHTTP(client, req)
	if err != nil {
		h.LogFields("debug", "http 流响应错误: "+r.Method+" "+r.URL, map[string]string{"action": "http", "detail": err.Error()})
		return nil, err
	}
	defer resp.Body.Close()
	hr := &HTTPResponse{Status: resp.StatusCode, Header: resp.Header}
	if resp.StatusCode != 200 {
		hr.Body = readLimited(resp.Body, 8192)
		h.logResponse(r, hr, nil)
		return hr, nil
	}
	raw := scanSSELines(resp.Body, parser)
	h.logResponse(r, &HTTPResponse{Status: resp.StatusCode, Header: resp.Header, Body: []byte(raw)}, nil)
	return hr, nil
}

// StreamRaw 异形帧上游的逃生口：发请求 + 打码日志 + 按 Proxy 自建 client，
// 把状态码与原始 body 交给插件自行分帧
// body 全量交插件读取，日志侧经 TeeReader 旁路捕获前 1MB（不截断插件数据）。
func (h *Host) StreamRaw(ctx context.Context, r HTTPRequest, client *http.Client,
	onStream func(status int, body io.Reader) error) (*HTTPResponse, error) {
	if r.Method == "" {
		r.Method = "POST"
	}
	if client == nil {
		client = UpstreamClient(ProxyURL(r.Proxy))
	}
	req, err := http.NewRequestWithContext(ctx, r.Method, r.URL, strings.NewReader(string(r.Body)))
	if err != nil {
		return nil, err
	}
	for k, v := range r.Headers {
		req.Header.Set(k, v)
	}
	h.logRequest(r, req.Header)
	resp, err := doHTTP(client, req)
	if err != nil {
		h.LogFields("debug", "http 流响应错误: "+r.Method+" "+r.URL, map[string]string{"action": "http", "detail": err.Error()})
		return nil, err
	}
	defer resp.Body.Close()
	hr := &HTTPResponse{Status: resp.StatusCode, Header: resp.Header}
	cw := &capWriter{cap: 1 << 20}
	cbErr := onStream(resp.StatusCode, io.TeeReader(resp.Body, cw))
	hr.Body = cw.buf.Bytes()
	h.logResponse(r, hr, nil)
	return hr, cbErr
}

// capWriter 旁路捕获前 cap 字节供日志，超出丢弃但仍如实返回写入量（不阻断 tee）。
type capWriter struct {
	buf bytes.Buffer
	cap int
}

func (c *capWriter) Write(p []byte) (int, error) {
	if rem := c.cap - c.buf.Len(); rem > 0 {
		if len(p) > rem {
			c.buf.Write(p[:rem])
		} else {
			c.buf.Write(p)
		}
	}
	return len(p), nil
}

// ScanSSE 逐行扫描一个已打开的 SSE body 并驱动 parser（不发请求、不记日志）。
// 供已自行发起请求的调用方复用；新代码优先用 StreamSSE（带统一日志）。
func ScanSSE(body io.Reader, parser SSEParser) error {
	scanSSELines(body, parser)
	return nil
}

// scanSSELines 逐行读 body 喂 parser，返回原始流（供日志）。
// 无 data 事件=空流(502)，非 EOF 断流(502)，正常结束 Finish。
func scanSSELines(body io.Reader, parser SSEParser) string {
	var rawStream strings.Builder
	tmp := make([]byte, 64*1024)
	var pending string
	sawEvent := false
	for {
		n, err := body.Read(tmp)
		if n > 0 {
			rawStream.Write(tmp[:n])
			scanned := pending + string(tmp[:n])
			pending = ""
			for {
				i := strings.IndexByte(scanned, '\n')
				if i < 0 {
					break
				}
				line := strings.TrimSuffix(scanned[:i], "\r")
				scanned = scanned[i+1:]
				if strings.HasPrefix(line, "data:") && !strings.Contains(line, "[DONE]") {
					sawEvent = true
				}
				parser.Feed(line)
			}
			pending = scanned
		}
		if err != nil {
			if len(pending) > 0 {
				parser.Feed(pending)
			}
			if err != io.EOF {
				parser.FinishWithError(502, "upstream stream broken: "+err.Error())
				return rawStream.String()
			}
			break
		}
	}
	if !sawEvent {
		parser.FinishWithError(502, "upstream returned an empty stream")
		return rawStream.String()
	}
	parser.Finish()
	return rawStream.String()
}

// readLimited 最多读 limit 字节（内部用；shared.ReadLimited 的等价物）。
func readLimited(body io.Reader, limit int64) []byte {
	if body == nil {
		return nil
	}
	raw, _ := io.ReadAll(io.LimitReader(body, limit))
	return raw
}
