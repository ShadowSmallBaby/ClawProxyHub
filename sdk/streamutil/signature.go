package streamutil

import (
	"encoding/base64"
	"strings"
)

// ResponsesSignature 为信封中的不透明内容显式标注协议，不猜测上游密文格式。
func ResponsesSignature(raw string) string {
	if raw == "" {
		return ""
	}
	return "cph:responses:v1:" + base64.StdEncoding.EncodeToString([]byte(raw))
}
func DecodeResponsesSignature(value string) (string, bool) {
	const prefix = "cph:responses:v1:"
	if !strings.HasPrefix(value, prefix) {
		return "", false
	}
	b, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, prefix))
	return string(b), err == nil
}
