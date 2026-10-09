// rpc.go — 通过宿主创建的管道双向通信，限制消息、并发并传递取消。
package extension

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

type RPCHandler func(context.Context, string, json.RawMessage) (any, error)
type frame struct {
	Type   string          `json:"type"`
	ID     uint64          `json:"id"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}
type reply struct {
	raw json.RawMessage
	err error
}
type Peer struct {
	reader     io.ReadCloser
	writer     io.WriteCloser
	handler    RPCHandler
	out        chan []byte
	done       chan struct{}
	once       sync.Once
	mu         sync.Mutex
	sendMu     sync.Mutex
	err        error
	serial     uint64
	incomingID uint64
	pending    map[uint64]chan reply
	incoming   map[uint64]context.CancelFunc
}

func NewPeer(reader io.ReadCloser, writer io.WriteCloser, handler RPCHandler) *Peer {
	p := &Peer{reader: reader, writer: writer, handler: handler, out: make(chan []byte, 32), done: make(chan struct{}), pending: map[uint64]chan reply{}, incoming: map[uint64]context.CancelFunc{}}
	go p.read()
	go p.write()
	return p
}
func (p *Peer) Done() <-chan struct{} { return p.done }
func (p *Peer) Err() error            { p.mu.Lock(); defer p.mu.Unlock(); return p.err }
func (p *Peer) Close() error          { p.fail(io.EOF); return nil }
func (p *Peer) fail(err error) {
	p.once.Do(func() {
		p.mu.Lock()
		p.err = err
		close(p.done)
		for _, cancel := range p.incoming {
			cancel()
		}
		for _, ch := range p.pending {
			ch <- reply{err: err}
		}
		p.pending = map[uint64]chan reply{}
		p.mu.Unlock()
		_ = p.reader.Close()
		_ = p.writer.Close()
	})
}
func (p *Peer) send(message frame) error {
	raw, err := json.Marshal(message)
	if err != nil {
		return err
	}
	if len(raw) > MaxJSONBytes {
		return fmt.Errorf("RPC message too large")
	}
	select {
	case <-p.done:
		return p.Err()
	default:
	}
	select {
	case p.out <- append(raw, '\n'):
		return nil
	case <-p.done:
		return p.Err()
	default:
		return fmt.Errorf("RPC queue full")
	}
}
func (p *Peer) Call(ctx context.Context, method string, input any) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	p.sendMu.Lock()
	p.mu.Lock()
	if p.err != nil {
		err := p.err
		p.mu.Unlock()
		p.sendMu.Unlock()
		return nil, err
	}
	if len(p.pending) >= 32 {
		p.mu.Unlock()
		p.sendMu.Unlock()
		return nil, fmt.Errorf("too many RPC calls")
	}
	p.serial++
	id := p.serial
	ch := make(chan reply, 1)
	p.pending[id] = ch
	p.mu.Unlock()
	defer func() { p.mu.Lock(); delete(p.pending, id); p.mu.Unlock() }()
	if err = p.send(frame{Type: "request", ID: id, Method: method, Params: raw}); err != nil {
		p.sendMu.Unlock()
		return nil, err
	}
	p.sendMu.Unlock()
	select {
	case result := <-ch:
		return result.raw, result.err
	case <-ctx.Done():
		_ = p.send(frame{Type: "cancel", ID: id})
		return nil, ctx.Err()
	case <-p.done:
		return nil, p.Err()
	}
}
func (p *Peer) write() {
	for {
		select {
		case <-p.done:
			return
		case raw := <-p.out:
			if _, err := p.writer.Write(raw); err != nil {
				p.fail(err)
				return
			}
		}
	}
}
func (p *Peer) read() {
	scanner := bufio.NewScanner(p.reader)
	scanner.Buffer(make([]byte, 4096), MaxJSONBytes+1)
	for scanner.Scan() {
		var message frame
		if err := DecodeStrict(scanner.Bytes(), &message); err != nil || message.ID == 0 {
			p.fail(fmt.Errorf("invalid RPC frame"))
			return
		}
		switch message.Type {
		case "response":
			p.mu.Lock()
			ch := p.pending[message.ID]
			delete(p.pending, message.ID)
			p.mu.Unlock()
			if ch != nil {
				var err error
				if message.Error != "" {
					err = errors.New(message.Error)
				}
				ch <- reply{message.Result, err}
			}
		case "cancel":
			p.mu.Lock()
			cancel := p.incoming[message.ID]
			p.mu.Unlock()
			if cancel != nil {
				cancel()
			}
		case "request":
			p.mu.Lock()
			if message.ID <= p.incomingID || message.Method == "" || len(message.Method) > 64 {
				p.mu.Unlock()
				p.fail(fmt.Errorf("invalid RPC request"))
				return
			}
			p.incomingID = message.ID
			if len(p.incoming) >= 32 {
				p.mu.Unlock()
				if err := p.send(frame{Type: "response", ID: message.ID, Error: "too many incoming RPC calls"}); err != nil {
					p.fail(err)
					return
				}
				continue
			}
			ctx, cancel := context.WithCancel(context.Background())
			p.incoming[message.ID] = cancel
			p.mu.Unlock()
			go p.handle(ctx, message)
		default:
			p.fail(fmt.Errorf("unsupported RPC frame"))
			return
		}
	}
	err := scanner.Err()
	if err == nil {
		err = io.EOF
	}
	p.fail(err)
}
func (p *Peer) handle(ctx context.Context, message frame) {
	response := frame{Type: "response", ID: message.ID}
	func() {
		defer func() {
			if recover() != nil {
				response.Error = "RPC handler failed"
			}
		}()
		result, err := p.handler(ctx, message.Method, message.Params)
		if err == nil {
			response.Result, err = json.Marshal(result)
		}
		if err != nil {
			response.Error = err.Error()
			if len(response.Error) > 1024 {
				response.Error = response.Error[:1024]
			}
			response.Result = nil
		}
		if len(response.Result) > MaxJSONBytes-2048 {
			response.Result = nil
			response.Error = "RPC result too large"
		}
	}()
	p.mu.Lock()
	cancel := p.incoming[message.ID]
	delete(p.incoming, message.ID)
	p.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if err := p.send(response); err != nil {
		p.fail(err)
	}
}
