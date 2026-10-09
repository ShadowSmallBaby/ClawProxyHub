// Package mcpserver 将显式授权的核心动作提供给外部 MCP 客户端。
package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/action"
)

const Protocol = "2025-03-26"

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}
type Server struct {
	actions   *action.Registry
	allow     map[string]bool
	authorize func(context.Context, action.Principal, string) bool
	mu        sync.Mutex
	calls     map[string]context.CancelFunc
}

// NewManaged 由当前请求的持久化授权决定工具范围，与静态工具列表共用协议实现。
func NewManaged(registry *action.Registry, authorize func(context.Context, action.Principal, string) bool) *Server {
	s := New(registry, nil)
	s.authorize = authorize
	return s
}
func (s *Server) permitted(ctx context.Context, p action.Principal, id string) bool {
	if s.authorize != nil {
		return s.authorize(ctx, p, id)
	}
	return s.allow[id]
}

func New(registry *action.Registry, allowed []string) *Server {
	s := &Server{actions: registry, allow: map[string]bool{}, calls: map[string]context.CancelFunc{}}
	for _, id := range allowed {
		if id = strings.TrimSpace(id); id != "" {
			s.allow[id] = true
		}
	}
	return s
}
func requestID(raw json.RawMessage) (string, bool) {
	var str string
	if json.Unmarshal(raw, &str) == nil && len(str) > 0 && len(str) <= 128 {
		return "s:" + str, true
	}
	var number int64
	if json.Unmarshal(raw, &number) == nil && len(raw) > 0 && string(raw) != "null" {
		return fmt.Sprintf("n:%d", number), true
	}
	return "", false
}
func (s *Server) HTTP(w http.ResponseWriter, r *http.Request, p action.Principal, session string) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if h := r.Header.Get("MCP-Protocol-Version"); h != "" && h != Protocol {
		http.Error(w, "unsupported MCP protocol", 400)
		return
	}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		http.Error(w, "application/json required", 415)
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, action.MaxJSONBytes+1))
	if err != nil || len(raw) > action.MaxJSONBytes {
		http.Error(w, "MCP request too large", 413)
		return
	}
	var req Request
	response := Response{JSONRPC: "2.0", ID: json.RawMessage("null")}
	if json.Unmarshal(raw, &req) != nil {
		response.Error = &RPCError{-32700, "parse error"}
	} else if req.JSONRPC != "2.0" || req.Method == "" {
		response.Error = &RPCError{-32600, "invalid request"}
	} else {
		if len(req.ID) == 0 {
			if req.Method == "notifications/cancelled" {
				var in struct {
					RequestID json.RawMessage `json:"requestId"`
				}
				if json.Unmarshal(req.Params, &in) == nil {
					if id, ok := requestID(in.RequestID); ok {
						s.mu.Lock()
						cancel := s.calls[session+"/"+id]
						s.mu.Unlock()
						if cancel != nil {
							cancel()
						}
					}
				}
			}
			w.WriteHeader(http.StatusAccepted)
			return
		}
		if _, ok := requestID(req.ID); !ok {
			response.Error = &RPCError{-32600, "invalid request id"}
		} else {
			response.ID = req.ID
			response.Result, response.Error = s.call(r.Context(), req, p, session)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
func (s *Server) call(ctx context.Context, req Request, p action.Principal, session string) (any, *RPCError) {
	switch req.Method {
	case "initialize":
		var in struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if json.Unmarshal(req.Params, &in) != nil || in.ProtocolVersion == "" {
			return nil, &RPCError{-32602, "missing protocolVersion"}
		}
		return map[string]any{"protocolVersion": Protocol, "capabilities": map[string]any{"tools": map[string]bool{"listChanged": false}}, "serverInfo": map[string]string{"name": "ClawProxyHub", "version": "1.0.0"}}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		tools := []any{}
		for _, d := range s.actions.List(p) {
			if s.permitted(ctx, p, d.ID) {
				tools = append(tools, map[string]any{"name": d.ID, "description": d.Title, "inputSchema": d.Input, "annotations": map[string]any{"readOnlyHint": d.Effect == "read", "destructiveHint": d.Effect != "read", "openWorldHint": true}})
			}
		}
		return map[string]any{"tools": tools}, nil
	case "tools/call":
		var in struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if json.Unmarshal(req.Params, &in) != nil || !s.permitted(ctx, p, in.Name) {
			return nil, &RPCError{-32602, "tool not exposed"}
		}
		visible := false
		for _, d := range s.actions.List(p) {
			visible = visible || d.ID == in.Name
		}
		if !visible {
			return nil, &RPCError{-32602, "tool unavailable or forbidden"}
		}
		if len(in.Arguments) == 0 {
			in.Arguments = json.RawMessage("{}")
		}
		id, _ := requestID(req.ID)
		key := session + "/" + id
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		s.mu.Lock()
		if _, exists := s.calls[key]; exists || len(s.calls) >= 128 {
			s.mu.Unlock()
			return nil, &RPCError{-32600, "duplicate request or server busy"}
		}
		s.calls[key] = cancel
		s.mu.Unlock()
		defer func() { s.mu.Lock(); delete(s.calls, key); s.mu.Unlock() }()
		result, err := s.actions.Invoke(ctx, p, in.Name, in.Arguments)
		if err != nil {
			return map[string]any{"content": []any{map[string]string{"type": "text", "text": err.Error()}}, "isError": true}, nil
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			return nil, &RPCError{-32603, "invalid tool result"}
		}
		return map[string]any{"content": []any{map[string]string{"type": "text", "text": string(encoded)}}, "isError": false}, nil
	default:
		return nil, &RPCError{-32601, "method not found"}
	}
}
