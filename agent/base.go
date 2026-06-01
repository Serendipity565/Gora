package agent

import (
	"context"
	"sync"
	"time"

	"github.com/Serendipity565/gora/tool"
)

// BaseAgent provides a minimal Agent implementation.
type BaseAgent struct {
	id    string
	state State
	mu    sync.RWMutex
	tools *tool.Registry

	stopCh chan struct{}
	doneCh chan struct{}
}

func NewBaseAgent(id string, registry *tool.Registry) *BaseAgent {
	doneCh := make(chan struct{})
	close(doneCh)

	return &BaseAgent{
		id:     id,
		state:  StateIdle,
		tools:  registry,
		stopCh: make(chan struct{}),
		doneCh: doneCh,
	}
}

func (a *BaseAgent) ID() string {
	return a.id
}

func (a *BaseAgent) State() State {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.state
}

func (a *BaseAgent) setState(s State) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state = s
}

func (a *BaseAgent) beginRun() (<-chan struct{}, chan struct{}) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.state = StateRunning
	a.stopCh = make(chan struct{})
	a.doneCh = make(chan struct{})

	return a.stopCh, a.doneCh
}

func (a *BaseAgent) Stop() error {
	a.setState(StateDone)
	select {
	case <-a.stopCh:
	default:
		close(a.stopCh)
	}
	<-a.doneCh
	return nil
}

// Run simulates an Agent thinking and streaming a response.
func (a *BaseAgent) Run(ctx context.Context, input string) <-chan Event {
	events := make(chan Event, 16)

	stopCh, doneCh := a.beginRun()

	go func() {
		defer close(events)
		defer close(doneCh)

		send := func(event Event) bool {
			select {
			case events <- event:
				return true
			case <-ctx.Done():
				a.setState(StateError)
				return false
			case <-stopCh:
				return false
			}
		}

		if !send(NewThinkingEvent(a.id, "正在分析输入...")) {
			return
		}

		time.Sleep(300 * time.Millisecond)

		if !send(NewChunkEvent(a.id, "你好！我是 ")) {
			return
		}

		time.Sleep(100 * time.Millisecond)

		if !send(NewChunkEvent(a.id, a.id)) {
			return
		}

		time.Sleep(100 * time.Millisecond)

		if !send(NewChunkEvent(a.id, "，收到了你的消息：「"+input+"」")) {
			return
		}

		if !send(NewDoneEvent(a.id)) {
			return
		}

		a.setState(StateDone)
	}()

	return events
}
