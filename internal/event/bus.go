// Package event — 进程内轻量事件总线（pub/sub）。
// 非阻塞投递：订阅者 chan 满则丢弃该事件，不阻塞发布方。
package event

import "sync"

// Topic 事件主题。
type Topic string

const (
	TopicTaskCompleted  Topic = "task.completed"    // 任务成功完成（payload: AccountID）
	TopicAccountRefresh Topic = "account.refreshed" // 账号凭据/profile 刷新
	TopicModelsSynced   Topic = "account.models_synced"
)

// Event 单条事件。
type Event struct {
	Topic     Topic
	AccountID int64
}

// Bus 进程内事件总线。
type Bus struct {
	mu   sync.RWMutex
	subs map[Topic][]chan Event
}

func New() *Bus {
	return &Bus{subs: map[Topic][]chan Event{}}
}

// Subscribe 订阅主题，返回只读 chan（缓冲 64）。
func (b *Bus) Subscribe(topic Topic) <-chan Event {
	ch := make(chan Event, 64)
	b.mu.Lock()
	b.subs[topic] = append(b.subs[topic], ch)
	b.mu.Unlock()
	return ch
}

// Publish 向主题投递事件；订阅者 chan 满则跳过（非阻塞）。
func (b *Bus) Publish(ev Event) {
	b.mu.RLock()
	chans := b.subs[ev.Topic]
	b.mu.RUnlock()
	for _, ch := range chans {
		select {
		case ch <- ev:
		default:
		}
	}
}
