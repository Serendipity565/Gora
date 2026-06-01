package agent

import "time"

func NewEvent(eventType EventType, agentID, content string) Event {
	return Event{
		Type:      eventType,
		AgentID:   agentID,
		Content:   content,
		Timestamp: time.Now(),
	}
}

func NewThinkingEvent(agentID, content string) Event {
	return NewEvent(EventThinking, agentID, content)
}

func NewToolCallEvent(agentID, toolName string, args map[string]any) Event {
	return Event{
		Type:      EventToolCall,
		AgentID:   agentID,
		Content:   toolName,
		Timestamp: time.Now(),
		Metadata:  map[string]any{"args": args},
	}
}

func NewToolResultEvent(agentID, toolName, result string) Event {
	return Event{
		Type:      EventToolResult,
		AgentID:   agentID,
		Content:   result,
		Timestamp: time.Now(),
		Metadata:  map[string]any{"tool": toolName},
	}
}

func NewChunkEvent(agentID, content string) Event {
	return NewEvent(EventChunk, agentID, content)
}

func NewDoneEvent(agentID string) Event {
	return NewEvent(EventDone, agentID, "")
}

func NewErrorEvent(agentID string, err error) Event {
	return NewEvent(EventError, agentID, err.Error())
}
