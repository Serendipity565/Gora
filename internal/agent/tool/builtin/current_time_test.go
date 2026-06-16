package builtin

import (
	"context"
	"testing"
)

func TestCurrentTime_Name(t *testing.T) {
	ct := NewCurrentTimeTool()
	if ct.Name() != "current_time" {
		t.Errorf("unexpected name: %s", ct.Name())
	}
}

func TestCurrentTime_Parameters(t *testing.T) {
	ct := NewCurrentTimeTool()
	params := ct.Parameters()
	if params["type"] != "object" {
		t.Errorf("unexpected params type: %v", params["type"])
	}
}

func TestCurrentTime_UTC(t *testing.T) {
	ct := NewCurrentTimeTool()
	result, err := ct.Execute(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if result == "" {
		t.Error("expected non-empty result")
	}
	t.Logf("UTC: %s", result)
}

func TestCurrentTime_Timezone(t *testing.T) {
	ct := NewCurrentTimeTool()
	result, err := ct.Execute(context.Background(), map[string]any{
		"timezone": "Asia/Shanghai",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Asia/Shanghai: %s", result)
}

func TestCurrentTime_Format(t *testing.T) {
	ct := NewCurrentTimeTool()
	formats := []string{"datetime", "date", "time", "unix"}
	for _, f := range formats {
		result, err := ct.Execute(context.Background(), map[string]any{
			"format": f,
		})
		if err != nil {
			t.Errorf("format %s error: %v", f, err)
		}
		t.Logf("format=%s: %s", f, result)
	}
}

func TestCurrentTime_InvalidTimezone(t *testing.T) {
	ct := NewCurrentTimeTool()
	_, err := ct.Execute(context.Background(), map[string]any{
		"timezone": "Mars/Olympus",
	})
	if err == nil {
		t.Error("expected error for invalid timezone")
	}
}
