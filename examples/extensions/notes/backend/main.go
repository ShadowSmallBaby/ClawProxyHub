// 笔记与标签分表，业务代码只向宿主提交结构化存储请求。
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"

	ext "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
)

func value(v string) json.RawMessage { raw, _ := json.Marshal(v); return raw }
func main() {
	err := ext.Serve(ext.Service{Actions: map[string]ext.ServiceHandler{
		"list": func(ctx context.Context, host *ext.Host, _ json.RawMessage) (any, error) {
			result, err := host.Storage(ctx, ext.StorageRequest{Op: "query", Table: "notes", Limit: 100})
			return map[string]any{"notes": result.Rows}, err
		},
		"create": func(ctx context.Context, host *ext.Host, raw json.RawMessage) (any, error) {
			var input struct {
				Content string `json:"content"`
				Label   string `json:"label"`
			}
			if err := ext.DecodeStrict(raw, &input); err != nil {
				return nil, err
			}
			id := make([]byte, 16)
			if _, err := rand.Read(id); err != nil {
				return nil, err
			}
			labelID := hex.EncodeToString(id)
			results, err := host.Batch(ctx, []ext.StorageRequest{
				{Op: "insert", Table: "labels", Values: map[string]json.RawMessage{"id": value(labelID), "name": value(input.Label)}},
				{Op: "insert", Table: "notes", Values: map[string]json.RawMessage{"content": value(input.Content), "label_id": value(labelID)}},
			})
			if err != nil {
				return nil, err
			}
			return map[string]any{"id": results[1].ID}, nil
		},
	}})
	if err != nil {
		log.Fatal(err)
	}
}
