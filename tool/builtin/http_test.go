package builtin

import (
	"context"
	"strings"
	"testing"
)

func TestHTTPTool_BlocksPrivateAddress(t *testing.T) {
	t.Parallel()

	httpTool := NewHTTPTool()
	_, err := httpTool.Execute(context.Background(), map[string]any{
		"url": "http://127.0.0.1:8080",
	})
	if err == nil {
		t.Fatal("expected private address to be blocked")
	}
	if !strings.Contains(err.Error(), "禁止访问") {
		t.Fatalf("expected block error, got %v", err)
	}
}

func TestHTTPTool_BlocksDisallowedMethod(t *testing.T) {
	t.Parallel()

	httpTool := NewHTTPTool(WithPrivateNetworkAccess(true))
	_, err := httpTool.Execute(context.Background(), map[string]any{
		"url":    "http://127.0.0.1:8080",
		"method": "DELETE",
	})
	if err == nil {
		t.Fatal("expected disallowed method error")
	}
	if !strings.Contains(err.Error(), "未被允许") {
		t.Fatalf("expected method error, got %v", err)
	}
}

func TestHTTPTool_AllowedMethodsSchema(t *testing.T) {
	t.Parallel()

	httpTool := NewHTTPTool(WithAllowedMethods("GET", "POST"))
	schema := httpTool.Parameters()
	properties := schema["properties"].(map[string]any)
	method := properties["method"].(map[string]any)
	enum := method["enum"].([]string)

	if len(enum) != 2 || enum[0] != "GET" || enum[1] != "POST" {
		t.Fatalf("unexpected method enum: %#v", enum)
	}
}
