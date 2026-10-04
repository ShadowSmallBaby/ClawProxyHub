// Package sdk — sse.go：统一 SSE 出站（行级 SSEParser 契约 + StreamSSE），
// 与 HTTPPost 共享打码日志。语义并入自 shared.ScanSSE：空流/断流报 502。
package sdk

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
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
		h.LogFields("debug", "http 流响应错误: "+r.Method+" "+logURL(r.URL), map[string]string{"action": "http", "detail": err.Error()})
		return nil, err
	}
	defer resp.Body.Close()
	hr := &HTTPResponse{Status: resp.StatusCode, Header: resp.Header}
	if resp.StatusCode != 200 {
		hr.Body = readLimited(resp.Body, 8192)
		h.logResponse(r, hr, nil)
		return hr, nil
	}
	readErr := scanSSELines(resp.Body, parser)
	h.logResponse(r, hr, readErr)
	return hr, readErr
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
		h.LogFields("debug", "http 流响应错误: "+r.Method+" "+logURL(r.URL), map[string]string{"action": "http", "detail": err.Error()})
		return nil, err
	}
	defer resp.Body.Close()
	hr := &HTTPResponse{Status: resp.StatusCode, Header: resp.Header}
	capture := &capWriter{cap: 64 << 10}
	cbErr := onStream(resp.StatusCode, io.TeeReader(resp.Body, capture))
	hr.Body = capture.buf.Bytes()
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
func ScanSSE(body io.Reader, parser SSEParser) error { return scanSSELines(body, parser) }

// ScanSSEWithLimit 为已约定较大帧的上游保留独立事件上限。
func ScanSSEWithLimit(body io.Reader, parser SSEParser, maxEventBytes int) error {
	if maxEventBytes <= 0 {
		maxEventBytes = 1 << 20
	}
	return scanSSELimit(body, parser, maxEventBytes)
}

// 按事件组装 data，终态合法性由各协议 parser 的 Finish 检查。
func scanSSELines(body io.Reader, parser SSEParser) error {
	return scanSSELimit(body, parser, 1<<20)
}

func scanSSELimit(body io.Reader, parser SSEParser, maxEventBytes int) error {
	err := ReadSSE(body, maxEventBytes, func(line string) error { parser.Feed(line); return nil })
	if err != nil {
		parser.FinishWithError(502, err.Error())
		return err
	}
	parser.Finish()
	return nil
}

// ReadSSE 合并多行 data；回调返回 io.EOF 可正常提前结束。
func ReadSSE(body io.Reader, maxEventBytes int, emit func(string) error) error {
	if maxEventBytes <= 0 {
		maxEventBytes = 1 << 20
	}
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, min(64*1024, maxEventBytes)), maxEventBytes)
	sawEvent := false
	var data []string
	size := 0
	flush := func() error {
		if len(data) > 0 {
			if err := emit("data: " + strings.Join(data, "\n")); err != nil {
				return err
			}
			sawEvent = true
		}
		if err := emit(""); err != nil {
			return err
		}
		data, size = nil, 0
		return nil
	}
	first := true
	for scanner.Scan() {
		line := scanner.Text()
		if first {
			line = strings.TrimPrefix(line, "\ufeff")
			first = false
		}
		if line == "" {
			if err := flush(); err != nil {
				if err == io.EOF {
					return nil
				}
				return err
			}
			continue
		}
		if strings.HasPrefix(line, "data:") || line == "data" {
			value := strings.TrimPrefix(strings.TrimPrefix(line, "data"), ":")
			value = strings.TrimPrefix(value, " ")
			size += len(value) + 1
			if size > maxEventBytes {
				err := fmt.Errorf("upstream SSE event exceeds %d bytes", maxEventBytes)
				return err
			}
			data = append(data, value)
			continue
		}
		if err := emit(line); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	// 兼容缺少末尾空行的上游；读取失败时不补发残缺事件。
	if len(data) > 0 {
		if err := flush(); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
	if !sawEvent {
		err := fmt.Errorf("upstream returned an empty stream")
		return err
	}
	return nil
}

// readLimited 最多读 limit 字节（内部用；shared.ReadLimited 的等价物）。
func readLimited(body io.Reader, limit int64) []byte {
	if body == nil {
		return nil
	}
	raw, _ := io.ReadAll(io.LimitReader(body, limit))
	return raw
}
