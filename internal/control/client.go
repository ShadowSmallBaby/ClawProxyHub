// Package control 是无数据库的管理客户端，CLI 和 stdio 仅连接现有核心。
package control

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type HTTPError struct {
	Status int
	Body   string
}

func (e *HTTPError) Error() string { return fmt.Sprintf("HTTP %d: %s", e.Status, e.Body) }

type Client struct {
	URL, Token string
	HTTP       *http.Client
}

func New(address, token string) (*Client, error) {
	u, e := url.Parse(address)
	if e != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" {
		return nil, fmt.Errorf("backend must be a base URL")
	}
	local := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"
	if u.Scheme != "https" && !(u.Scheme == "http" && local) {
		return nil, fmt.Errorf("remote backend requires HTTPS")
	}
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("set CPH_TOKEN or --token-file")
	}
	return &Client{URL: strings.TrimRight(address, "/"), Token: strings.TrimSpace(token), HTTP: &http.Client{Timeout: 310 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *Client) Request(ctx context.Context, method, path, contentType string, body io.Reader) (json.RawMessage, error) {
	raw, _, err := c.request(ctx, method, path, contentType, body, nil)
	return raw, err
}

// MCP 会话头仅用于 MCP 端点，不影响管理 API 请求。
func (c *Client) MCP(ctx context.Context, session string, body io.Reader) (json.RawMessage, string, error) {
	headers := make(http.Header)
	if session != "" {
		headers.Set("Mcp-Session-Id", session)
	}
	raw, responseHeaders, err := c.request(ctx, "POST", "/admin/mcp", "application/json", body, headers)
	return raw, responseHeaders.Get("Mcp-Session-Id"), err
}

func (c *Client) CloseMCP(ctx context.Context, session string) error {
	headers := make(http.Header)
	headers.Set("Mcp-Session-Id", session)
	_, _, err := c.request(ctx, "DELETE", "/admin/mcp", "application/json", nil, headers)
	return err
}

func (c *Client) request(ctx context.Context, method, path, contentType string, body io.Reader, headers http.Header) (json.RawMessage, http.Header, error) {
	req, e := http.NewRequestWithContext(ctx, method, c.URL+path, body)
	if e != nil {
		return nil, nil, e
	}
	req.Header = headers.Clone()
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json, text/event-stream")
	res, e := c.HTTP.Do(req)
	if e != nil {
		return nil, nil, e
	}
	defer res.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(res.Body, 4<<20+1))
	if e != nil {
		return nil, nil, e
	}
	if len(raw) > 4<<20 {
		return nil, nil, fmt.Errorf("response too large")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, nil, &HTTPError{res.StatusCode, string(raw)}
	}
	if len(raw) > 0 && !json.Valid(raw) {
		return nil, nil, fmt.Errorf("backend returned invalid JSON")
	}
	return raw, res.Header, nil
}
func (c *Client) Invoke(ctx context.Context, id string, input json.RawMessage) (json.RawMessage, error) {
	if len(input) == 0 {
		input = json.RawMessage("{}")
	}
	return c.Request(ctx, "POST", "/admin/actions/"+url.PathEscape(id), "application/json", bytes.NewReader(input))
}
func (c *Client) Install(ctx context.Context, path string, grants []string) (json.RawMessage, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	stat, e := f.Stat()
	if e != nil {
		return nil, e
	}
	if stat.Size() > 128<<20 {
		return nil, fmt.Errorf("package too large")
	}
	// 安装为显式自举命令；流式上传交给相同扩展管理器和授权检查。
	reader, writer := io.Pipe()
	form := multipart.NewWriter(writer)
	done := make(chan error, 1)
	go func() {
		part, e := form.CreateFormFile("package", "extension.cphext")
		if e == nil {
			_, e = io.Copy(part, f)
		}
		if e == nil {
			b, _ := json.Marshal(grants)
			e = form.WriteField("grants", string(b))
		}
		if e == nil {
			e = form.Close()
		}
		writer.CloseWithError(e)
		done <- e
	}()
	raw, e := c.Request(ctx, "POST", "/admin/extensions/install", form.FormDataContentType(), reader)
	reader.CloseWithError(e)
	return raw, errors.Join(e, <-done)
}
