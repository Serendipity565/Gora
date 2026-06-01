package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Serendipity565/gora/tool"
)

// BaseAgent 提供一个最小可运行的 Agent 基础实现。
type BaseAgent struct {
	id      string
	state   State
	mu      sync.RWMutex
	tools   *tool.Registry
	running bool

	stopCh    chan struct{}
	doneCh    chan struct{}
	runCancel context.CancelFunc
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
func (a *BaseAgent) beginRun(ctx context.Context) (context.Context, <-chan struct{}, chan struct{}, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.running {
		return nil, nil, nil, false
	}

	runCtx, cancel := context.WithCancel(ctx)
	a.state = StateRunning
	a.running = true
	a.stopCh = make(chan struct{})
	a.doneCh = make(chan struct{})
	a.runCancel = cancel

	return runCtx, a.stopCh, a.doneCh, true
}

func (a *BaseAgent) finishRun(doneCh chan struct{}) {
	a.mu.Lock()
	if a.doneCh == doneCh {
		a.running = false
		a.runCancel = nil
		if a.state == StateRunning || a.state == StateWaiting {
			a.state = StateDone
		}
	}
	a.mu.Unlock()

	close(doneCh)
}

// Stop 请求 Agent 停止当前执行，并等待当前轮次退出。
func (a *BaseAgent) Stop() error {
	a.mu.Lock()
	if !a.running {
		a.state = StateDone
		a.mu.Unlock()
		return nil
	}

	stopCh := a.stopCh
	doneCh := a.doneCh
	cancel := a.runCancel
	a.state = StateDone
	select {
	case <-stopCh:
	default:
		close(stopCh)
	}
	a.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	<-doneCh
	return nil
}

// Run 默认实现会模拟一次思考和流式回复，便于在未接入 LLM 时验证事件流。
func (a *BaseAgent) Run(ctx context.Context, input string) <-chan Event {
	events := make(chan Event, 16)

	runCtx, stopCh, doneCh, ok := a.beginRun(ctx)
	if !ok {
		go func() {
			defer close(events)
			events <- NewErrorEvent(a.id, fmt.Errorf("agent %s is already running", a.id))
		}()
		return events
	}

	go func() {
		defer close(events)
		defer a.finishRun(doneCh)

		send := func(event Event) bool {
			select {
			case events <- event:
				return true
			case <-runCtx.Done():
				select {
				case <-stopCh:
					return false
				default:
				}
				a.setState(StateError)
				return false
			case <-stopCh:
				return false
			}
		}

		if !send(NewThinkingEvent(a.id, "正在分析输入...")) {
			return
		}

		if !sleepOrDone(runCtx, stopCh, 300*time.Millisecond) {
			return
		}

		if !send(NewChunkEvent(a.id, "你好！我是 ")) {
			return
		}

		if !sleepOrDone(runCtx, stopCh, 100*time.Millisecond) {
			return
		}

		if !send(NewChunkEvent(a.id, a.id)) {
			return
		}

		if !sleepOrDone(runCtx, stopCh, 100*time.Millisecond) {
			return
		}

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

func sleepOrDone(ctx context.Context, stopCh <-chan struct{}, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	case <-stopCh:
		return false
	}
}
