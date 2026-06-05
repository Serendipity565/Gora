package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
)

// disabledToolsKey 用于把"禁用工具集合"写入 context，
// 以便 Eino 工具适配器在 InvokableRun 中读取。
type disabledToolsKey struct{}

// permissionGateKey 用于把 ToolPermissionGate 注入 context。
type permissionGateKey struct{}

// WithDisabledTools 在 context 中记录一组被用户关闭的工具名，
// goraInvokableTool 在执行时会拒绝命中该集合的工具调用，
// 或者（当 context 中也存在 ToolPermissionGate 时）发起一次授权询问。
//
// 该函数只读取 names，不会修改原 slice。空切片会原样返回 ctx，避免无意义的
// context 链增长。多次调用会以"最后一次"为准。
func WithDisabledTools(ctx context.Context, names []string) context.Context {
	if len(names) == 0 {
		return ctx
	}
	set := make(map[string]struct{}, len(names))
	for _, name := range names {
		if name == "" {
			continue
		}
		set[name] = struct{}{}
	}
	if len(set) == 0 {
		return ctx
	}
	return context.WithValue(ctx, disabledToolsKey{}, set)
}

// disabledToolsFromContext 返回 context 中保存的禁用工具集合。
// 第二个返回值表示集合是否存在且非空。
func disabledToolsFromContext(ctx context.Context) (map[string]struct{}, bool) {
	set, ok := ctx.Value(disabledToolsKey{}).(map[string]struct{})
	if !ok || len(set) == 0 {
		return nil, false
	}
	return set, true
}

// IsToolDisabled 判断给定工具名是否被 context 标记为禁用。
// 主要供测试和外部集成使用。
func IsToolDisabled(ctx context.Context, name string) bool {
	set, ok := disabledToolsFromContext(ctx)
	if !ok {
		return false
	}
	_, blocked := set[name]
	return blocked
}

// ToolPermissionDecision 表示用户对一次工具授权请求的决定。
type ToolPermissionDecision struct {
	// Approved 为 true 表示允许本次调用执行。
	Approved bool
	// Remember 为 true 表示用户希望记住该选择
	// （前端可据此把工具从 disabledTools 中移除/加入）。
	Remember bool
}

// ToolPermissionGate 维护"等待用户答复的工具调用"集合。
// 当 InvokableRun 命中被禁用的工具时会向 gate 注册一条等待项，
// 同时通过事件流通知前端；前端拿到结果后调用 Resolve 唤醒。
type ToolPermissionGate struct {
	mu      sync.Mutex
	pending map[string]chan ToolPermissionDecision
}

// NewToolPermissionGate 创建一个空的 gate。
func NewToolPermissionGate() *ToolPermissionGate {
	return &ToolPermissionGate{
		pending: make(map[string]chan ToolPermissionDecision),
	}
}

// Register 在 gate 中预约一个 request_id，并返回用于接收决定的 channel。
// 调用方负责生成唯一 request_id（推荐使用 NewRequestID）。
func (g *ToolPermissionGate) Register(requestID string) (<-chan ToolPermissionDecision, error) {
	if requestID == "" {
		return nil, errors.New("requestID 不能为空")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, exists := g.pending[requestID]; exists {
		return nil, errors.New("requestID 已存在")
	}
	ch := make(chan ToolPermissionDecision, 1)
	g.pending[requestID] = ch
	return ch, nil
}

// Cancel 移除一个尚未 resolve 的 request_id（例如调用方因 ctx 取消而退出）。
// 返回 true 表示 cancel 命中了一个待处理项。
func (g *ToolPermissionGate) Cancel(requestID string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	ch, ok := g.pending[requestID]
	if !ok {
		return false
	}
	delete(g.pending, requestID)
	close(ch)
	return true
}

// Resolve 把用户的决定传给等待的 InvokableRun。
// 第二个返回值表示是否命中了一个尚未处理的 request_id。
func (g *ToolPermissionGate) Resolve(requestID string, decision ToolPermissionDecision) bool {
	g.mu.Lock()
	ch, ok := g.pending[requestID]
	if ok {
		delete(g.pending, requestID)
	}
	g.mu.Unlock()
	if !ok {
		return false
	}
	// channel 容量为 1，写入不会阻塞。写入后立刻关闭，避免重复 Resolve。
	ch <- decision
	close(ch)
	return true
}

// WithToolPermissionGate 把 gate 注入 context，供 InvokableRun 读取。
func WithToolPermissionGate(ctx context.Context, gate *ToolPermissionGate) context.Context {
	if gate == nil {
		return ctx
	}
	return context.WithValue(ctx, permissionGateKey{}, gate)
}

// toolPermissionGateFromContext 取出 context 中的 gate。
func toolPermissionGateFromContext(ctx context.Context) (*ToolPermissionGate, bool) {
	gate, ok := ctx.Value(permissionGateKey{}).(*ToolPermissionGate)
	return gate, ok && gate != nil
}

// NewRequestID 生成一个 16 字节的随机 request_id（hex 编码）。
func NewRequestID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// crypto/rand 几乎不会失败；万一失败也得返回点东西，避免 panic。
		return "req-fallback"
	}
	return hex.EncodeToString(buf[:])
}
