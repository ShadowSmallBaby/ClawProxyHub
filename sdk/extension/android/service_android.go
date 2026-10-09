// Package android 接管私有 Service 传入的 socket；验签与进程管理由宿主负责。
package android

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"

	ext "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
)

var sessions = struct {
	sync.Mutex
	next  int64
	items map[int64]context.CancelFunc
}{items: map[int64]context.CancelFunc{}}

// Open 总是接管 fd；每个连接有独立的处理器与可撤销生命周期。
func Open(fd int, service ext.Service) (int64, error) {
	if fd < 0 {
		return 0, fmt.Errorf("invalid extension socket")
	}
	file := os.NewFile(uintptr(fd), "extension-socket")
	connection, err := net.FileConn(file)
	_ = file.Close()
	if err != nil {
		return 0, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	sessions.Lock()
	sessions.next++
	id := sessions.next
	sessions.items[id] = cancel
	sessions.Unlock()
	go func() {
		defer connection.Close()
		defer Close(id)
		_ = ext.ServeIO(ctx, connection, connection, service)
	}()
	return id, nil
}

func Close(id int64) {
	sessions.Lock()
	cancel := sessions.items[id]
	delete(sessions.items, id)
	sessions.Unlock()
	if cancel != nil {
		cancel()
	}
}
