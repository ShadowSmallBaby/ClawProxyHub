// Package androidconn 接管经 Binder 传入的 socket 描述符。
package androidconn

import (
	"fmt"
	"net"
	"os"
	"sync"

	"github.com/ShadowSmallBaby/ClawProxyHub/sdk/transport"
)

func Take(fd int) (net.Conn, error) {
	if fd < 0 {
		return nil, fmt.Errorf("invalid socket descriptor")
	}
	f := os.NewFile(uintptr(fd), "binder-socket")
	defer f.Close()
	return net.FileConn(f)
}

// Registry 让 Binder 断连与原生传输关闭共享幂等清理。
type Registry struct {
	mu    sync.Mutex
	next  int64
	items map[int64]*transport.Session
}

func (r *Registry) Add(session *transport.Session) int64 {
	r.mu.Lock()
	if r.items == nil {
		r.items = make(map[int64]*transport.Session)
	}
	r.next++
	id := r.next
	r.items[id] = session
	r.mu.Unlock()
	go func() { <-session.Done(); r.Close(id) }()
	return id
}

func (r *Registry) Close(id int64) {
	r.mu.Lock()
	s := r.items[id]
	delete(r.items, id)
	r.mu.Unlock()
	if s != nil {
		s.Close()
	}
}
