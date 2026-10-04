// Package streamutil 统一上游错误的分类，不靠消息正文猜测恢复策略。
package streamutil

import "strings"

func ErrorCode(kind string, status int32) int32 {
	if status >= 400 && status <= 599 {
		return status
	}
	switch strings.ToLower(kind) {
	case "authentication_error", "invalid_api_key", "unauthorized":
		return 401
	case "permission_error", "permission_denied":
		return 403
	case "rate_limit_error", "rate_limit_exceeded", "insufficient_quota":
		return 429
	case "invalid_request_error":
		return 400
	default:
		return 502
	}
}
