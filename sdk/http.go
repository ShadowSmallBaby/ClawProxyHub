// Package sdk — http.go：共享 HTTP 出站助手（debug 级统一记录原始请求/原始响应，可直接复制复现）。
package sdk

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
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
	// Proxy 出站代理（可选）：client 传 nil 时按此自建 client。
	Proxy *pb.ProxyConfig
}

// HTTPResponse 一次出站请求的结果（helper 返回值）。
type HTTPResponse struct {
	Status int
	Header http.Header
	Body   []byte // 原始响应体（流式响应为已读部分）
}

var defaultSensitive = []string{"api-key", "authorization", "cookie", "x-api-key", "set-cookie", "x-ima-cookie"}

// redactHeaders 打码敏感头（日志用；多值头以 "; " 合并）。
func redactHeaders(h http.Header, sensitive []string) map[string]string {
	mask := map[string]bool{}
	for _, k := range append(sensitive, defaultSensitive...) {
		mask[strings.ToLower(k)] = true
	}
	out := map[string]string{}
	for k, vs := range h {
		v := strings.Join(vs, "; ")
		if mask[strings.ToLower(k)] || sensitiveHeader(k) {
			v = redactValue(v)
		}
		out[k] = v
	}
	return out
}

// redactValue 敏感值统一隐藏。
func redactValue(v string) string { return "***" }

func sensitiveHeader(key string) bool {
	key = strings.ToLower(key)
	for _, part := range []string{"authorization", "cookie", "token", "secret", "password", "api-key", "api_key", "session"} {
		if strings.Contains(key, part) {
			return true
		}
	}
	return false
}

// logURL 不记录查询值或 URL 内的认证信息。
func logURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "[invalid URL]"
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

// HTTPPost 发送 JSON POST，debug 日志仅记录脱敏后的地址、请求头与响应状态。
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
	if client == nil && r.Proxy != nil {
		client = UpstreamClient(ProxyURL(r.Proxy))
	}
	h.logRequest(r, req.Header)
	resp, err := doHTTP(client, req)
	if err != nil {
		h.LogFields("debug", "http 响应错误: "+r.Method+" "+logURL(r.URL), map[string]string{"action": "http", "detail": err.Error()})
		return nil, err
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, (32<<20)+1))
	if len(body) > 32<<20 {
		return nil, fmt.Errorf("upstream response exceeds 32 MiB")
	}
	hr := &HTTPResponse{Status: resp.StatusCode, Header: resp.Header, Body: body}
	h.logResponse(r, hr, readErr)
	return hr, readErr
}

// HTTPStream 发送流式请求并逐行回调 SSE（event, data）或原始行。
// debug 日志不记录响应正文。
//
// Deprecated: 用 StreamSSE（行级 SSEParser + 统一日志 + 可选 Proxy 自建 client）。
func (h *Host) HTTPStream(ctx context.Context, r HTTPRequest, client *http.Client, onEvent func(string, string) error) (*HTTPResponse, error) {
	req, err := http.NewRequestWithContext(ctx, r.Method, r.URL, bytes.NewReader(r.Body))
	if err != nil {
		return nil, err
	}
	for k, v := range r.Headers {
		req.Header.Set(k, v)
	}
	if client == nil {
		client = UpstreamClient(ProxyURL(r.Proxy))
	}
	h.logRequest(r, req.Header)
	resp, err := doHTTP(client, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	hr := &HTTPResponse{Status: resp.StatusCode, Header: resp.Header}
	if resp.StatusCode != 200 {
		hr.Body = readLimited(resp.Body, 8192)
		h.logResponse(r, hr, nil)
		return hr, nil
	}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1<<20)
	var event string
	var data []string
	size := 0
	flush := func() error {
		if event == "" && len(data) == 0 {
			return nil
		}
		err := onEvent(event, strings.Join(data, "\n"))
		event = ""
		data = nil
		size = 0
		return err
	}
	for scanner.Scan() {
		line := scanner.Text()
		size += len(line)
		if size > 1<<20 {
			return hr, fmt.Errorf("SSE frame too large")
		}
		if line == "" {
			if err = flush(); err != nil {
				return hr, err
			}
		} else if v, ok := strings.CutPrefix(line, "event:"); ok {
			event = strings.TrimSpace(v)
		} else if v, ok := strings.CutPrefix(line, "data:"); ok {
			data = append(data, strings.TrimSpace(v))
		}
	}
	if err = scanner.Err(); err != nil {
		return hr, err
	}
	if err = flush(); err != nil {
		return hr, err
	}
	h.logResponse(r, hr, nil)
	return hr, nil
}

// logRequest / logResponse 记录 HTTP 概览与脱敏头，不保留正文。
func (h *Host) logRequest(r HTTPRequest, actual http.Header) {
	h.LogFields("debug", "http 请求: "+r.Method+" "+logURL(r.URL), map[string]string{
		"action": "http",
		"detail": "headers: " + jsonObject(redactHeaders(actual, r.Sensitive)) + "\nbody: [omitted]",
	})
}

func (h *Host) logResponse(r HTTPRequest, hr *HTTPResponse, readErr error) {
	note := ""
	if readErr != nil {
		note = "\n(response body read failed)"
	}
	h.LogFields("debug", "http 响应: "+r.Method+" "+logURL(r.URL)+" status="+strconv.Itoa(hr.Status), map[string]string{
		"action": "http",
		"detail": "headers: " + jsonObject(redactHeaders(hr.Header, r.Sensitive)) + "\nbody: [omitted]" + note,
	})
}

func doHTTP(client *http.Client, req *http.Request) (*http.Response, error) {
	if client == nil {
		client = http.DefaultClient
	}
	return client.Do(req)
}

// ProxyURL 把 ProxyConfig 渲染成 http 代理 URL，未配置时返回空串。
func ProxyURL(p *pb.ProxyConfig) string {
	if p == nil || p.GetHost() == "" {
		return ""
	}
	u := &url.URL{
		Scheme: orStr(p.GetScheme(), "http"),
		Host:   net.JoinHostPort(strings.Trim(p.GetHost(), "[]"), strconv.Itoa(int(p.GetPort()))),
	}
	if p.GetUsername() != "" {
		u.User = url.UserPassword(p.GetUsername(), p.GetPassword())
	}
	return u.String()
}

// UpstreamClient 构造访问上游的 HTTP 客户端，proxyURL 非空时走该代理。
func UpstreamClient(proxyURL string) *http.Client {
	transport := &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 15 * time.Second}).DialContext,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
		IdleConnTimeout:       90 * time.Second,
	}
	if proxyURL != "" {
		if u, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(u)
		}
	}
	return &http.Client{Transport: transport}
}

func orStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
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
