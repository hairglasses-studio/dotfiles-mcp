package tracing

import (
	"context"
	"errors"
	"testing"

	"github.com/hairglasses-studio/mcpkit/registry"
)

func TestEnabled(t *testing.T) {
	t.Setenv("OTEL_ENABLED", "")
	if Enabled() {
		t.Fatal("expected disabled when OTEL_ENABLED is empty")
	}

	t.Setenv("OTEL_ENABLED", "false")
	if Enabled() {
		t.Fatal("expected disabled when OTEL_ENABLED is false")
	}

	t.Setenv("OTEL_ENABLED", "true")
	if !Enabled() {
		t.Fatal("expected enabled when OTEL_ENABLED is true")
	}

	t.Setenv("OTEL_ENABLED", "TRUE")
	if !Enabled() {
		t.Fatal("expected enabled when OTEL_ENABLED is TRUE (case-insensitive)")
	}
}

func TestInit_Disabled(t *testing.T) {
	t.Setenv("OTEL_ENABLED", "")
	shutdown := Init(context.Background(), "test")
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("noop shutdown returned error: %v", err)
	}
}

func TestInit_Enabled(t *testing.T) {
	t.Setenv("OTEL_ENABLED", "true")
	shutdown := Init(context.Background(), "test")
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown returned error: %v", err)
	}
}

func TestMiddleware_Disabled(t *testing.T) {
	t.Setenv("OTEL_ENABLED", "")

	var called bool
	inner := func(ctx context.Context, req registry.CallToolRequest) (*registry.CallToolResult, error) {
		called = true
		return registry.MakeTextResult("ok"), nil
	}

	mw := Middleware()
	wrapped := mw("test_tool", registry.ToolDefinition{}, inner)
	result, err := wrapped(context.Background(), registry.CallToolRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("inner handler was not called")
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestMiddleware_Enabled(t *testing.T) {
	t.Setenv("OTEL_ENABLED", "true")
	shutdown := Init(context.Background(), "test")
	defer shutdown(context.Background())

	var called bool
	inner := func(ctx context.Context, req registry.CallToolRequest) (*registry.CallToolResult, error) {
		called = true
		return registry.MakeTextResult("ok"), nil
	}

	mw := Middleware()
	wrapped := mw("test_tool", registry.ToolDefinition{}, inner)
	result, err := wrapped(context.Background(), registry.CallToolRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("inner handler was not called")
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestMiddleware_Error(t *testing.T) {
	t.Setenv("OTEL_ENABLED", "true")
	shutdown := Init(context.Background(), "test")
	defer shutdown(context.Background())

	inner := func(ctx context.Context, req registry.CallToolRequest) (*registry.CallToolResult, error) {
		return nil, errors.New("boom")
	}

	mw := Middleware()
	wrapped := mw("test_tool", registry.ToolDefinition{}, inner)
	_, err := wrapped(context.Background(), registry.CallToolRequest{})
	if err == nil || err.Error() != "boom" {
		t.Fatalf("expected 'boom' error, got: %v", err)
	}
}
