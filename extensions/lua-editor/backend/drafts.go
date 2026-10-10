// 草稿按修订分块写入，最后切换记录；中断写入不覆盖上一份完整草稿。
package luaeditor

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	ext "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
)

const draftChunkBytes = 32 << 10

type draftService struct{ gate chan struct{} }
type draftKey struct {
	Name string `json:"name"`
}
type draftVersion struct {
	Name     string `json:"name"`
	Revision string `json:"revision"`
}
type draftInput struct {
	Name       string `json:"name"`
	Revision   string `json:"revision"`
	Content    string `json:"content"`
	BaseSHA256 string `json:"base_sha256"`
}
type draftRecord struct {
	ID         string `json:"id"`
	Revision   string `json:"revision"`
	BaseSHA256 string `json:"base_sha256"`
	SHA256     string `json:"sha256"`
	UpdatedAt  string `json:"updated_at"`
}

func (d *draftService) serial(handler ext.ServiceHandler) ext.ServiceHandler {
	return func(ctx context.Context, host *ext.Host, raw json.RawMessage) (any, error) {
		select {
		case d.gate <- struct{}{}:
			defer func() { <-d.gate }()
			return handler(ctx, host, raw)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}
func draftID(name string) (string, error) {
	if name == "" {
		return "new", nil
	}
	if !pluginName.MatchString(name) {
		return "", fmt.Errorf("invalid draft plugin name")
	}
	return "plugin:" + name, nil
}
func rawValue(value any) json.RawMessage {
	raw, _ := json.Marshal(value)
	return raw
}
func equal(column string, value any) ext.Predicate {
	return ext.Predicate{Column: column, Op: "eq", Value: rawValue(value)}
}
func values(fields map[string]any) map[string]json.RawMessage {
	result := make(map[string]json.RawMessage, len(fields))
	for key, value := range fields {
		result[key] = rawValue(value)
	}
	return result
}
func lookupDraft(ctx context.Context, host *ext.Host, id string) (draftRecord, error) {
	result, err := host.Storage(ctx, ext.StorageRequest{Op: "query", Table: "drafts", Where: []ext.Predicate{equal("id", id)}, Limit: 1})
	var record draftRecord
	if err == nil && len(result.Rows) > 0 {
		err = ext.DecodeStrict(rawValue(result.Rows[0]), &record)
	}
	return record, err
}

func (d *draftService) read(ctx context.Context, host *ext.Host, raw json.RawMessage) (any, error) {
	var input draftKey
	if err := ext.DecodeStrict(raw, &input); err != nil {
		return nil, err
	}
	id, err := draftID(input.Name)
	if err != nil {
		return nil, err
	}
	record, err := lookupDraft(ctx, host, id)
	if err != nil || record.Revision == "" {
		return map[string]any{"found": false}, err
	}
	var source strings.Builder
	for offset := 0; ; offset += 24 {
		result, err := host.Storage(ctx, ext.StorageRequest{Op: "query", Table: "draft_chunks", Columns: []string{"position", "content"},
			Where: []ext.Predicate{equal("draft_id", id), equal("revision", record.Revision)}, Order: []ext.Order{{Column: "position"}}, Limit: 24, Offset: offset})
		if err != nil {
			return nil, err
		}
		for index, row := range result.Rows {
			var chunk struct {
				Position int    `json:"position"`
				Content  string `json:"content"`
			}
			if err = ext.DecodeStrict(rawValue(row), &chunk); err != nil {
				return nil, err
			}
			decoded, err := base64.StdEncoding.Strict().DecodeString(chunk.Content)
			if err != nil || chunk.Position != offset+index || len(decoded) > draftChunkBytes || source.Len()+len(decoded) > 2<<20 {
				return nil, fmt.Errorf("invalid draft chunk")
			}
			source.Write(decoded)
		}
		if len(result.Rows) < 24 {
			break
		}
	}
	content := source.String()
	if err = validateSource(content); err != nil || hashSource(content) != record.SHA256 {
		return nil, fmt.Errorf("draft content failed integrity check")
	}
	return map[string]any{"found": true, "draft": map[string]any{
		"name": input.Name, "content": content, "revision": record.Revision, "sha256": record.SHA256,
		"base_sha256": record.BaseSHA256, "updated_at": record.UpdatedAt,
	}}, nil
}

func (d *draftService) save(ctx context.Context, host *ext.Host, raw json.RawMessage) (any, error) {
	var input draftInput
	if err := ext.DecodeStrict(raw, &input); err != nil {
		return nil, err
	}
	id, err := draftID(input.Name)
	if err != nil {
		return nil, err
	}
	if err = validateSource(input.Content); err != nil {
		return nil, err
	}
	if !ext.ValidHash(input.BaseSHA256) || input.Revision != "" && !ext.ValidHash(input.Revision) {
		return nil, fmt.Errorf("invalid draft revision or source hash")
	}
	current, err := lookupDraft(ctx, host, id)
	if err != nil {
		return nil, err
	}
	if current.Revision != input.Revision {
		return nil, fmt.Errorf("draft changed in another editor; reopen it before saving a draft")
	}
	if current.SHA256 == hashSource(input.Content) && current.BaseSHA256 == input.BaseSHA256 {
		return map[string]any{"revision": current.Revision, "sha256": current.SHA256, "updated_at": current.UpdatedAt}, nil
	}
	// 上一次未提交的分块可清理；当前完整修订一直保留到最后的事务提交。
	_, err = host.Storage(ctx, ext.StorageRequest{Op: "delete", Table: "draft_chunks", Where: []ext.Predicate{
		equal("draft_id", id), {Column: "revision", Op: "ne", Value: rawValue(current.Revision)},
	}})
	if err != nil {
		return nil, err
	}
	var nonce [32]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return nil, err
	}
	revision := hex.EncodeToString(nonce[:])
	requests := []ext.StorageRequest{}
	for offset, position := 0, 0; offset < len(input.Content); offset, position = offset+draftChunkBytes, position+1 {
		chunk := []byte(input.Content[offset:min(offset+draftChunkBytes, len(input.Content))])
		requests = append(requests, ext.StorageRequest{Op: "insert", Table: "draft_chunks", Values: values(map[string]any{
			"id": revision + ":" + fmt.Sprint(position), "draft_id": id, "revision": revision,
			"position": position, "content": base64.StdEncoding.EncodeToString(chunk),
		})})
	}
	for len(requests) > 0 {
		count := min(24, len(requests))
		if _, err = host.Batch(ctx, requests[:count]); err != nil {
			return nil, err
		}
		requests = requests[count:]
	}
	hash, updated := hashSource(input.Content), time.Now().UTC().Format(time.RFC3339Nano)
	_, err = host.Batch(ctx, []ext.StorageRequest{
		{Op: "delete", Table: "drafts", Where: []ext.Predicate{equal("id", id)}},
		{Op: "insert", Table: "drafts", Values: values(map[string]any{
			"id": id, "revision": revision, "base_sha256": input.BaseSHA256, "sha256": hash, "updated_at": updated,
		})},
		{Op: "delete", Table: "draft_chunks", Where: []ext.Predicate{
			equal("draft_id", id), {Column: "revision", Op: "ne", Value: rawValue(revision)},
		}},
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"revision": revision, "sha256": hash, "updated_at": updated}, nil
}

func (d *draftService) remove(ctx context.Context, host *ext.Host, raw json.RawMessage) (any, error) {
	var input draftVersion
	if err := ext.DecodeStrict(raw, &input); err != nil {
		return nil, err
	}
	id, err := draftID(input.Name)
	if err != nil {
		return nil, err
	}
	current, err := lookupDraft(ctx, host, id)
	if err != nil {
		return nil, err
	}
	if current.Revision != input.Revision {
		return nil, fmt.Errorf("draft changed in another editor; reopen it before deleting a draft")
	}
	_, err = host.Batch(ctx, []ext.StorageRequest{
		{Op: "delete", Table: "draft_chunks", Where: []ext.Predicate{equal("draft_id", id)}},
		{Op: "delete", Table: "drafts", Where: []ext.Predicate{equal("id", id)}},
	})
	return map[string]bool{"deleted": err == nil}, err
}
