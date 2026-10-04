package admin

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type loginBucket struct {
	count int
	until time.Time
}
type loginLimiter struct {
	mu      sync.Mutex
	buckets map[string]loginBucket
}

// allow 使用连接地址，避免客户端伪造转发头绕过限制。
func (l *loginLimiter) allow(r *http.Request, username string) bool {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.buckets == nil {
		l.buckets = make(map[string]loginBucket)
	}
	for key, b := range l.buckets {
		if !now.Before(b.until) {
			delete(l.buckets, key)
		}
	}
	keys := []string{"ip:" + ip, "user:" + ip + ":" + strings.TrimSpace(username)}
	for i, key := range keys {
		limit := 30
		if i == 1 {
			limit = 5
		}
		if l.buckets[key].count >= limit {
			return false
		}
	}
	if len(l.buckets) >= 4096 {
		return false
	}
	for _, key := range keys {
		b := l.buckets[key]
		if b.count == 0 {
			b.until = now.Add(time.Minute)
		}
		b.count++
		l.buckets[key] = b
	}
	return true
}
