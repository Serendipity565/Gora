package builtin

import (
	"context"
	"fmt"
	"time"
)

// CurrentTimeTool 获取当前日期和时间。
type CurrentTimeTool struct{}

func NewCurrentTimeTool() *CurrentTimeTool {
	return &CurrentTimeTool{}
}

func (c *CurrentTimeTool) Name() string {
	return "current_time"
}

func (c *CurrentTimeTool) Description() string {
	return "获取当前日期和时间，支持指定时区与输出格式"
}

func (c *CurrentTimeTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"timezone": map[string]any{
				"type":        "string",
				"description": "IANA 时区名称，如 Asia/Shanghai、America/New_York，默认为 UTC",
			},
			"format": map[string]any{
				"type":        "string",
				"description": "输出格式：datetime（默认）、date、time、unix",
				"enum":        []string{"datetime", "date", "time", "unix"},
			},
		},
	}
}

func (c *CurrentTimeTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	tzName := "UTC"
	if tz, ok := args["timezone"].(string); ok && tz != "" {
		tzName = tz
	}

	loc, err := time.LoadLocation(tzName)
	if err != nil {
		return "", fmt.Errorf("无效的时区: %s", tzName)
	}

	now := time.Now().In(loc)

	format := "datetime"
	if f, ok := args["format"].(string); ok && f != "" {
		format = f
	}

	switch format {
	case "date":
		return now.Format("2006-01-02"), nil
	case "time":
		return now.Format("15:04:05"), nil
	case "unix":
		return fmt.Sprintf("%d", now.Unix()), nil
	default:
		return now.Format("2006-01-02 15:04:05 MST"), nil
	}
}
