// 集成测试使用真实 Go 子进程验证宿主授权、启动和会话撤销。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"sync/atomic"

	ext "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
)

func main() {
	var startup context.Context
	var remembered context.Context
	var mu sync.Mutex
	var waiting atomic.Int32
	storage := func(ctx context.Context, host *ext.Host, raw json.RawMessage) (any, error) {
		var request ext.StorageRequest
		if err := ext.DecodeStrict(raw, &request); err != nil {
			return nil, err
		}
		return host.Storage(ctx, request)
	}
	err := ext.Serve(ext.Service{
		Start: func(ctx context.Context, host *ext.Host) error {
			startup = ctx
			if os.Getenv("CPH_ADMIN_PASSWORD") != "" || os.Getenv("CPH_SECRET_KEY") != "" {
				return fmt.Errorf("host credentials leaked to extension environment")
			}
			_, err := host.Storage(ctx, ext.StorageRequest{Op: "insert", Table: "notes", Values: map[string]json.RawMessage{"value": json.RawMessage(`"uncommitted"`)}})
			if err == nil {
				return fmt.Errorf("storage writes accepted before installation commit")
			}
			_, err = host.Storage(ctx, ext.StorageRequest{Op: "query", Table: "notes"})
			return err
		},
		Actions: map[string]ext.ServiceHandler{
			"read":  storage,
			"write": storage,
			"batch": func(ctx context.Context, host *ext.Host, raw json.RawMessage) (any, error) {
				var input struct {
					Requests []ext.StorageRequest `json:"requests"`
				}
				if err := ext.DecodeStrict(raw, &input); err != nil {
					return nil, err
				}
				result, err := host.Batch(ctx, input.Requests)
				return map[string]any{"results": result}, err
			},
			"inspect": func(context.Context, *ext.Host, json.RawMessage) (any, error) {
				return map[string]any{"background_alive": startup.Err() == nil, "waiting": waiting.Load()}, nil
			},
			"remember": func(ctx context.Context, _ *ext.Host, _ json.RawMessage) (any, error) {
				mu.Lock()
				remembered = context.WithoutCancel(ctx)
				mu.Unlock()
				return map[string]bool{"saved": true}, nil
			},
			"reuse": func(_ context.Context, host *ext.Host, _ json.RawMessage) (any, error) {
				mu.Lock()
				previous := remembered
				mu.Unlock()
				return host.Storage(previous, ext.StorageRequest{Op: "query", Table: "notes"})
			},
			"wait": func(ctx context.Context, _ *ext.Host, _ json.RawMessage) (any, error) {
				waiting.Add(1)
				defer waiting.Add(-1)
				<-ctx.Done()
				return nil, ctx.Err()
			},
			"crash": func(context.Context, *ext.Host, json.RawMessage) (any, error) {
				os.Exit(23)
				return nil, nil
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
