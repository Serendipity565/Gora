package agent

import "time"

// NewEvent 创建一个基础运行时事件。
func NewEvent(eventType EventType, agentID, content string) Event {
	return Event{
		Type:      eventType,
		AgentID:   agentID,
		Content:   content,
		Timestamp: time.Now(),
	}
}

// NewThinkingEvent 创建思考事件。
func NewThinkingEvent(agentID, content string) Event {
	return NewEvent(EventThinking, agentID, content)
}

// NewToolCallEvent 创建工具调用事件，并把调用参数写入元数据。
func NewToolCallEvent(agentID, toolName string, args map[string]any) Event {
	return Event{
		Type:      EventToolCall,
		AgentID:   agentID,
		Content:   toolName,
		Timestamp: time.Now(),
		Metadata:  map[string]any{"args": args},
	}
}

// NewToolResultEvent 创建工具返回事件，并记录对应工具名。
func NewToolResultEvent(agentID, toolName, result string) Event {
	return Event{
		Type:      EventToolResult,
		AgentID:   agentID,
		Content:   result,
		Timestamp: time.Now(),
		Metadata:  map[string]any{"tool": toolName},
	}
}

// NewChunkEvent 创建流式输出片段事件。
func NewChunkEvent(agentID, content string) Event {
	return NewEvent(EventChunk, agentID, content)
}

// NewDoneEvent 创建本轮完成事件。
func NewDoneEvent(agentID string) Event {
	return NewEvent(EventDone, agentID, "")
}

// NewErrorEvent 创建错误事件。
func NewErrorEvent(agentID string, err error) Event {
	return NewEvent(EventError, agentID, err.Error())
}

// NewToolPermissionRequestEvent 创建工具授权请求事件，
// 前端收到后应弹出"是否允许调用 toolName"的询问 UI，
// 拿到用户决定后通过 /api/chat/tool-permission 回写。
func NewToolPermissionRequestEvent(agentID, requestID, toolName string, args map[string]any) Event {
	return Event{
		Type:      EventToolPermissionRequest,
		AgentID:   agentID,
		Content:   toolName,
		Timestamp: time.Now(),
		Metadata: map[string]any{
			"request_id": requestID,
			"tool":       toolName,
			"args":       args,
		},
	}
}
