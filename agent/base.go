package agent

import (
	"context"
	"sync"
	"time"

	"github.com/Serendipity565/gora/tool"
)

// BaseAgent 提供一个最小可运行的 Agent 基础实现。
type BaseAgent struct {
	id    string
	state State
	mu    sync.RWMutex
	tools *tool.Registry

	stopCh chan struct{}
	doneCh chan struct{}
}

// NewBaseAgent 创建一个基础 Agent，并初始化生命周期通道。
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

// ID 返回 Agent 的唯一标识。
func (a *BaseAgent) ID() string {
	return a.id
}

// State 返回 Agent 当前状态。
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

// beginRun 为新一轮执行重置状态和停止信号。
func (a *BaseAgent) beginRun() (<-chan struct{}, chan struct{}) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.state = StateRunning
	a.stopCh = make(chan struct{})
	a.doneCh = make(chan struct{})

	return a.stopCh, a.doneCh
}

// Stop 请求 Agent 停止当前执行，并等待当前轮次退出。
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

// Run 默认实现会模拟一次思考和流式回复，便于在未接入 LLM 时验证事件流。
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
