// Package sdk — http.go：共享 HTTP 出站助手（debug 级统一记录原始请求/原始响应，可直接复制复现）。
package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// HTTPRequest 一次出站请求的描述（helper 参数）。
type HTTPRequest struct {
	Method  string
	URL     string
	Headers map[string]string // 逐键 Set（日志中打码敏感头）
	Body    []byte
	// Sensitive 敏感请求头名（不区分大小写）：日志中值替换为打码占位。
	// 缺省打码 Authorization / Cookie / X-Api-Key / Set-Cookie。
	Sensitive []string
}

// HTTPResponse 一次出站请求的结果（helper 返回值）。
type HTTPResponse struct {
	Status int
	Header http.Header
	Body   []byte // 原始响应体（流式响应为已读部分）
}

var defaultSensitive = []string{"api-key", "authorization", "cookie", "x-api-key", "set-cookie"}

// redactHeaders 打码敏感头（日志用；多值头以 "; " 合并）。
func redactHeaders(h http.Header, sensitive []string) map[string]string {
	mask := map[string]bool{}
	for _, k := range append(sensitive, defaultSensitive...) {
		mask[strings.ToLower(k)] = true
	}
	out := map[string]string{}
	for k, vs := range h {
		v := strings.Join(vs, "; ")
		if mask[strings.ToLower(k)] {
			v = redactValue(v)
		}
		out[k] = v
	}
	return out
}

// redactValue 值打码：保留前 4 后 4 字符，中间 …（短值全打码）。
func redactValue(v string) string {
	if len(v) <= 12 {
		return "***"
	}
	return v[:4] + "…" + v[len(v)-4:]
}

// HTTPPost 发送 JSON POST（debug 级统一日志：原始请求/原始响应，可直接复制复现）。
// 响应体全量读回（调用方解析），错误一并返回。
func (h *Host) HTTPPost(ctx context.Context, r HTTPRequest, client *http.Client) (*HTTPResponse, error) {
	if r.Method == "" {
		r.Method = "POST"
	}
	req, err := http.NewRequestWithContext(ctx, r.Method, r.URL, bytes.NewReader(r.Body))
	if err != nil {
		return nil, err
	}
	for k, v := range r.Headers {
		req.Header.Set(k, v)
	}
	h.logRequest(r, req.Header)
	resp, err := doHTTP(client, req)
	if err != nil {
		h.LogFields("debug", "http 响应错误: "+r.Method+" "+r.URL, map[string]string{"action": "http", "detail": err.Error()})
		return nil, err
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(resp.Body)
	hr := &HTTPResponse{Status: resp.StatusCode, Header: resp.Header, Body: body}
	h.logResponse(r, hr, readErr)
	return hr, nil
}

// HTTPStream 发送流式请求并逐行回调 SSE（event, data）或原始行。
// debug 级记录请求 + 完整响应流（拼接后一次性落日志）。
func (h *Host) HTTPStream(ctx context.Context, r HTTPRequest, client *http.Client,
	onEvent func(event, data string) error) (*HTTPResponse, error) {
	req, err := http.NewRequestWithContext(ctx, r.Method, r.URL, bytes.NewReader(r.Body))
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
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		hr.Body = body
		h.logResponse(r, hr, nil)
		return hr, nil
	}
	// SSE 逐行扫描：event:/data: 行按空行分块；同时拼接原始流供日志
	var rawStream bytes.Buffer
	var event string
	var datas []string
	flush := func() error {
		if event == "" && len(datas) == 0 {
			return nil
		}
		rawStream.WriteString("event: " + event + "\ndata: " + strings.Join(datas, "\n") + "\n\n")
		e := onEvent(event, strings.Join(datas, "\n"))
		event = ""
		datas = nil
		return e
	}
	buf := make([]byte, 64*1024)
	var carry []byte
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			rawStream.Write(buf[:n])
			carry = append(carry, buf[:n]...)
			for {
				block, rest, ok := splitBlock(carry)
				if !ok {
					break
				}
				carry = rest
				ev, ds := parseBlock(block)
				event = ev
				datas = ds
				if err := flush(); err != nil {
					h.Log("debug", "cph-http 流结束(回调中断): "+r.Method+" "+r.URL+"\n"+rawStream.String())
					return hr, err
				}
			}
		}
		if readErr != nil {
			if len(carry) > 0 {
				ev, ds := parseBlock(string(carry))
				event, datas = ev, ds
				_ = flush()
			}
			break
		}
	}
	h.logResponse(r, &HTTPResponse{Status: resp.StatusCode, Header: resp.Header, Body: rawStream.Bytes()}, nil)
	return hr, nil
}

// logRequest / logResponse debug 级日志（请求/响应完整，敏感头中的凭据值打码）。
// 走 LogFields（action=http），message 带 URL 作出处，body 在 detail 里便于排查。
func (h *Host) logRequest(r HTTPRequest, actual http.Header) {
	h.LogFields("debug", "http 请求: "+r.Method+" "+r.URL, map[string]string{
		"action": "http",
		"detail": "headers: " + jsonObject(redactHeaders(actual, r.Sensitive)) + "\nbody: " + string(r.Body),
	})
}

func (h *Host) logResponse(r HTTPRequest, hr *HTTPResponse, readErr error) {
	note := ""
	if readErr != nil {
		note = "\n(读响应体出错: " + readErr.Error() + ")"
	}
	h.LogFields("debug", "http 响应: "+r.Method+" "+r.URL+" status="+strconv.Itoa(hr.Status), map[string]string{
		"action": "http",
		"detail": "headers: " + jsonObject(redactHeaders(hr.Header, r.Sensitive)) + "\nbody: " + string(hr.Body) + note,
	})
}

func doHTTP(client *http.Client, req *http.Request) (*http.Response, error) {
	if client == nil {
		client = http.DefaultClient
	}
	return client.Do(req)
}

// splitBlock SSE 按空行分块（\r\n\r\n 或 \n\n 双形态），返回 (块, 余下, 是否找到)。
func splitBlock(s []byte) (string, []byte, bool) {
	str := string(s)
	if i := strings.Index(str, "\r\n\r\n"); i >= 0 {
		return str[:i], []byte(str[i+4:]), true
	}
	if i := strings.Index(str, "\n\n"); i >= 0 {
		return str[:i], []byte(str[i+2:]), true
	}
	return "", s, false
}

// parseBlock 块 → (event, data 行合并)。
func parseBlock(block string) (string, []string) {
	var event string
	var datas []string
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if v, ok := strings.CutPrefix(line, "event:"); ok {
			event = strings.TrimSpace(v)
		} else if v, ok := strings.CutPrefix(line, "data:"); ok {
			datas = append(datas, strings.TrimSpace(v))
		}
	}
	return event, datas
}

// jsonObject JSON 序列化（headers 展示用；encoding/json 对 map 键自动升序，多次日志字段顺序稳定可对照）。
func jsonObject(m map[string]string) string {
	raw, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(raw)
}
