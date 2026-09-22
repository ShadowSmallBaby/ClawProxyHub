package fingerprint

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
) // codexTurnMetadataJSON 生成 x-codex-turn-metadata 头的 JSON 值。
func codexTurnMetadataJSON(installationID, sessionID, threadID, turnID, windowID string) string {
	b, _ := json.Marshal(map[string]any{
		"installation_id":         installationID,
		"session_id":              sessionID,
		"thread_id":               threadID,
		"turn_id":                 turnID,
		"window_id":               windowID,
		"request_kind":            "turn",
		"thread_source":           "user",
		"sandbox":                 "none",
		"turn_started_at_unix_ms": time.Now().UnixMilli(),
	})
	return string(b)
}

func uuidV7() string {
	if id, err := uuid.NewV7(); err == nil {
		return id.String()
	}
	return uuid.NewString()
}
