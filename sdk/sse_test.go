package sdk

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// capParser 记录 parser 回调用于断言。
type capParser struct {
	lines    []string
	finished bool
	errCode  int32
	errMsg   string
}

func (p *capParser) Feed(line string)                  { p.lines = append(p.lines, line) }
func (p *capParser) Finish()                           { p.finished = true }
func (p *capParser) FinishWithError(c int32, m string) { p.errCode, p.errMsg = c, m }

func TestScanSSE_Normal(t *testing.T) {
	p := &capParser{}
	body := "event: message\ndata: hello\n\ndata: [DONE]\n\n"
	_ = ScanSSE(strings.NewReader(body), p)
	if !p.finished || p.errCode != 0 {
		t.Fatalf("正常流应 Finish，无错误：finished=%v code=%d", p.finished, p.errCode)
	}
}

func TestScanSSE_Empty(t *testing.T) {
	p := &capParser{}
	_ = ScanSSE(strings.NewReader("event: ping\n\n"), p) // 无 data 事件
	if p.errCode != 502 || p.finished {
		t.Fatalf("空流应 FinishWithError(502)：code=%d finished=%v", p.errCode, p.finished)
	}
}

func TestStreamSSE_Non200NoFeed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		fmt.Fprint(w, "rate limited")
	}))
	defer srv.Close()
	p := &capParser{}
	hr, err := (&Host{}).StreamSSE(context.Background(), HTTPRequest{Method: "GET", URL: srv.URL}, srv.Client(), p)
	if err != nil {
		t.Fatal(err)
	}
	if hr.Status != 429 || len(p.lines) != 0 || p.finished {
		t.Fatalf("非200不应喂 parser：status=%d lines=%d finished=%v", hr.Status, len(p.lines), p.finished)
	}
}

func TestStreamSSE_200Feeds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: a\n\ndata: [DONE]\n\n")
	}))
	defer srv.Close()
	p := &capParser{}
	hr, err := (&Host{}).StreamSSE(context.Background(), HTTPRequest{Method: "GET", URL: srv.URL}, srv.Client(), p)
	if err != nil || hr.Status != 200 || !p.finished {
		t.Fatalf("200 流应 Feed+Finish：status=%d finished=%v err=%v", hr.Status, p.finished, err)
	}
}

func TestStreamRaw_HandsRawBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "frame1\n\nframe2") // 异形帧：插件自行分帧
	}))
	defer srv.Close()
	var got string
	hr, err := (&Host{}).StreamRaw(context.Background(), HTTPRequest{Method: "GET", URL: srv.URL}, srv.Client(),
		func(status int, body io.Reader) error {
			b, _ := io.ReadAll(body)
			got = string(b)
			return nil
		})
	if err != nil || hr.Status != 200 || got != "frame1\n\nframe2" {
		t.Fatalf("StreamRaw 应原样交付 body：status=%d got=%q err=%v", hr.Status, got, err)
	}
	if string(hr.Body) != got {
		t.Fatalf("日志旁路捕获应等于已读内容：%q vs %q", string(hr.Body), got)
	}
}
