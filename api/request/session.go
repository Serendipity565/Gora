// Package request 定义客户端请求的数据传输对象。
package request

// ListSessions 是 GET /api/sessions 的查询参数。
type ListSessions struct {
	Limit  int `form:"limit"`
	Offset int `form:"offset"`
}

// ListMessages 是 GET /api/sessions/{sessionId}/messages 的查询参数。
type ListMessages struct {
	Cursor uint64 `form:"cursor"`
	Limit  int    `form:"limit"`
}
