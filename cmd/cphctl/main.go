// cphctl 的业务命令来自核心动作，stdio stdout 严格只写 MCP 协议。
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/control"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/mcpserver"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	err := run(ctx, os.Args[1:], os.Stdin, os.Stdout)
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, err)
	code := 1
	var h *control.HTTPError
	if errors.As(err, &h) {
		if h.Status == 401 || h.Status == 403 {
			code = 3
		} else if h.Status == 404 {
			code = 4
		} else {
			code = 5
		}
	}
	if errors.Is(err, context.Canceled) {
		code = 130
	}
	os.Exit(code)
}
func run(ctx context.Context, args []string, in io.Reader, out io.Writer) error {
	flags := flag.NewFlagSet("cli", flag.ContinueOnError)
	address := flags.String("url", os.Getenv("CPH_BACKEND"), "backend base URL")
	tokenFile := flags.String("token-file", "", "read bearer token from file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	args = flags.Args()
	if len(args) == 0 || args[0] == "help" {
		_, err := fmt.Fprintln(out, "cli --url URL [--token-file FILE] status | actions | describe ID | invoke ID [JSON|-] | install FILE [GRANTS_CSV] | enable ID | disable ID | uninstall ID | mcp\nCPH_TOKEN supplies the token; all results are JSON. Exit: 0 success, 1 local failure, 3 auth, 4 unavailable, 5 backend error, 130 cancelled.\nUse actions/describe for available core commands.")
		return err
	}
	token := os.Getenv("CPH_TOKEN")
	if *tokenFile != "" {
		b, e := os.ReadFile(*tokenFile)
		if e != nil {
			return e
		}
		token = string(b)
	}
	client, err := control.New(*address, token)
	if err != nil {
		return err
	}
	var raw json.RawMessage
	switch args[0] {
	case "mcp":
		return bridge(ctx, client, in, out)
	case "status":
		raw, err = client.Invoke(ctx, "core.status", nil)
	case "actions", "describe":
		raw, err = client.Request(ctx, "GET", "/admin/actions", "application/json", nil)
		if err == nil && args[0] == "describe" {
			if len(args) != 2 {
				return fmt.Errorf("describe ID")
			}
			var list struct {
				Actions []json.RawMessage `json:"actions"`
			}
			if err = json.Unmarshal(raw, &list); err != nil {
				return err
			}
			found := false
			for _, item := range list.Actions {
				var d struct{ ID string }
				json.Unmarshal(item, &d)
				if d.ID == args[1] {
					raw = item
					found = true
					break
				}
			}
			if !found {
				return &control.HTTPError{Status: 404, Body: "action unavailable; inspect installed extensions"}
			}
		}
	case "invoke":
		if len(args) < 2 || len(args) > 3 {
			return fmt.Errorf("invoke ID [JSON|-]")
		}
		input := []byte("{}")
		if len(args) == 3 {
			if args[2] == "-" {
				input, err = io.ReadAll(io.LimitReader(in, 2<<20+1))
				if err != nil {
					return err
				}
			} else {
				input = []byte(args[2])
			}
		}
		if len(input) > 2<<20 || !json.Valid(input) {
			return fmt.Errorf("invalid or oversized JSON")
		}
		raw, err = client.Invoke(ctx, args[1], input)
	case "enable", "disable", "uninstall":
		if len(args) != 2 {
			return fmt.Errorf("%s ID", args[0])
		}
		input, _ := json.Marshal(map[string]string{"id": args[1]})
		raw, err = client.Invoke(ctx, "core.extensions."+args[0], input)
	case "install":
		if len(args) < 2 || len(args) > 3 {
			return fmt.Errorf("install FILE [GRANTS_CSV]")
		}
		grants := []string{}
		if len(args) == 3 && args[2] != "" {
			grants = strings.Split(args[2], ",")
		}
		raw, err = client.Install(ctx, args[1], grants)
	default:
		return fmt.Errorf("unknown bootstrap command; use actions then invoke <namespaced-ID>")
	}
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(raw))
	return err
}

func bridge(ctx context.Context, c *control.Client, in io.Reader, out io.Writer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	lines := make(chan []byte)
	scanErr := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(in)
		scanner.Buffer(make([]byte, 4096), 2<<20)
		for scanner.Scan() {
			line := append([]byte{}, scanner.Bytes()...)
			select {
			case lines <- line:
			case <-ctx.Done():
				return
			}
		}
		scanErr <- scanner.Err()
		close(lines)
	}()
	var mu sync.Mutex
	var wg sync.WaitGroup
	defer wg.Wait()
	slots := make(chan struct{}, 32)
	reply := func(raw []byte) {
		mu.Lock()
		defer mu.Unlock()
		if _, err := fmt.Fprintln(out, string(raw)); err != nil {
			cancel()
		}
	}
	failure := func(id json.RawMessage, msg string) {
		if len(id) == 0 {
			id = json.RawMessage("null")
		}
		raw, _ := json.Marshal(mcpserver.Response{JSONRPC: "2.0", ID: id, Error: &mcpserver.RPCError{Code: -32603, Message: msg}})
		reply(raw)
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case line, ok := <-lines:
			if !ok {
				return <-scanErr
			}
			var req mcpserver.Request
			if json.Unmarshal(line, &req) != nil {
				failure(nil, "invalid JSON-RPC")
				continue
			}
			if len(req.ID) == 0 {
				// 即使工作槽已满，取消通知也必须能抵达核心。
				notifyCtx, done := context.WithTimeout(ctx, 5*time.Second)
				_, _ = c.Request(notifyCtx, "POST", "/admin/mcp", "application/json", bytes.NewReader(line))
				done()
				continue
			}
			select {
			case slots <- struct{}{}:
			default:
				if len(req.ID) > 0 {
					failure(req.ID, "bridge busy")
				}
				continue
			}
			wg.Add(1)
			go func(line []byte, req mcpserver.Request) {
				defer wg.Done()
				defer func() { <-slots }()
				raw, err := c.Request(ctx, "POST", "/admin/mcp", "application/json", bytes.NewReader(line))
				if err != nil {
					if len(req.ID) > 0 {
						failure(req.ID, err.Error())
					}
					return
				}
				if len(raw) > 0 {
					reply(raw)
				}
			}(line, req)
		}
	}
}
