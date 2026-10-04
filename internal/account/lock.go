// lock.go — 所有凭据轮换与任务写回共用账号锁；等待服从调用取消。
package account

import (
	"context"
	"fmt"
	"sync"
)

var credentialLocks = struct {
	sync.Mutex
	entries map[string]*credentialLock
}{entries: map[string]*credentialLock{}}

type credentialLock struct {
	token chan struct{}
	refs  int
}

func LockCredential(ctx context.Context, dataDir string, id int64) (func(), error) {
	key := fmt.Sprintf("%s:%d", dataDir, id)
	credentialLocks.Lock()
	entry := credentialLocks.entries[key]
	if entry == nil {
		entry = &credentialLock{token: make(chan struct{}, 1)}
		credentialLocks.entries[key] = entry
	}
	entry.refs++
	credentialLocks.Unlock()
	releaseRef := func() {
		credentialLocks.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(credentialLocks.entries, key)
		}
		credentialLocks.Unlock()
	}
	select {
	case entry.token <- struct{}{}:
		var once sync.Once
		return func() { once.Do(func() { <-entry.token; releaseRef() }) }, nil
	case <-ctx.Done():
		releaseRef()
		return nil, ctx.Err()
	}
}
