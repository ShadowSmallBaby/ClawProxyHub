package app

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/control"
)

func TestActionsAgreeAcrossControlAndMCP(t *testing.T) {
	a, err := New(testConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err = a.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer a.Shutdown(ctx)
	token, err := a.DeviceSession()
	if err != nil {
		t.Fatal(err)
	}
	c, err := control.New("http://"+a.Address(), token)
	if err != nil {
		t.Fatal(err)
	}
	direct, err := c.Invoke(ctx, "core.status", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.Request(ctx, "PUT", "/admin/mcp/config", "application/json", bytes.NewBufferString(`{"enabled":true,"allowed_actions":["core.status","core.tasks.list"]}`)); err != nil {
		t.Fatal(err)
	}
	keyJSON, err := c.Request(ctx, "POST", "/admin/mcp/key", "application/json", bytes.NewBufferString(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	var issued struct {
		Key string `json:"key"`
	}
	if err = json.Unmarshal(keyJSON, &issued); err != nil {
		t.Fatal(err)
	}
	mcp, err := control.New("http://"+a.Address(), issued.Key)
	if err != nil {
		t.Fatal(err)
	}
	_, session, err := mcp.MCP(ctx, "", bytes.NewBufferString(`{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-03-26"}}`))
	if err != nil || session == "" {
		t.Fatalf("initialize: %q %v", session, err)
	}
	raw, _, err := mcp.MCP(ctx, session, bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"core.status","arguments":{}}}`))
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if result.Result.IsError || len(result.Result.Content) != 1 || result.Result.Content[0].Text != strings.TrimSpace(string(direct)) {
		t.Fatalf("cross-entry result differs: %s / %s", direct, raw)
	}
	var count int64
	a.db.Table("action_audits").Where("action_id = ? AND outcome = ?", "core.status", "ok").Count(&count)
	if count != 2 {
		t.Fatalf("both entries must audit: %d", count)
	}
	if err := mcp.CloseMCP(ctx, session); err != nil {
		t.Fatal(err)
	}
}
