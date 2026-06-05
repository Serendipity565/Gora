package agent

import (
	"context"
	"testing"
)

func TestWithDisabledTools_RoundTrip(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	if IsToolDisabled(ctx, "anything") {
		t.Fatal("empty ctx should not flag any tool as disabled")
	}

	ctx = WithDisabledTools(ctx, []string{"http_get", "", "shell"})
	if !IsToolDisabled(ctx, "http_get") {
		t.Fatal("expected http_get to be disabled")
	}
	if !IsToolDisabled(ctx, "shell") {
		t.Fatal("expected shell to be disabled")
	}
	if IsToolDisabled(ctx, "calculator") {
		t.Fatal("calculator should not be disabled")
	}

	// 后写入的集合覆盖前一份。
	ctx = WithDisabledTools(ctx, []string{"calculator"})
	if !IsToolDisabled(ctx, "calculator") {
		t.Fatal("expected calculator to be disabled after override")
	}
	if IsToolDisabled(ctx, "http_get") {
		t.Fatal("http_get should be cleared by override")
	}
}

func TestWithDisabledTools_EmptyInputReturnsSameCtx(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	if WithDisabledTools(ctx, nil) != ctx {
		t.Fatal("nil input must return original ctx")
	}
	if WithDisabledTools(ctx, []string{}) != ctx {
		t.Fatal("empty slice must return original ctx")
	}
	if WithDisabledTools(ctx, []string{"", ""}) != ctx {
		t.Fatal("only-empty-name slice must return original ctx")
	}
}
