// Package tracing provides OpenTelemetry setup and middleware for dotfiles-mcp.
//
// Tracing is opt-in: set OTEL_ENABLED=true to activate the stdout span exporter.
// When disabled, all functions are no-ops and no OTel dependencies are exercised
// at runtime.
package tracing

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/hairglasses-studio/mcpkit/registry"
)

const tracerName = "dotfiles-mcp"

// Enabled reports whether OTel tracing is active.
func Enabled() bool {
	return strings.EqualFold(os.Getenv("OTEL_ENABLED"), "true")
}

// Init sets up the global TracerProvider with a stdout exporter.
// It returns a shutdown function that should be deferred.
// If OTEL_ENABLED is not "true", Init is a no-op and returns a no-op shutdown.
func Init(ctx context.Context, version string) (shutdown func(context.Context) error) {
	noop := func(context.Context) error { return nil }
	if !Enabled() {
		return noop
	}

	exp, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		slog.Error("otel: failed to create stdout exporter", "error", err)
		return noop
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String("dotfiles-mcp"),
			semconv.ServiceVersionKey.String(version),
		),
	)
	if err != nil {
		slog.Error("otel: failed to create resource", "error", err)
		return noop
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	slog.Info("otel: tracing enabled", "exporter", "stdout")
	return tp.Shutdown
}

// Middleware returns a registry.Middleware that wraps each tool call in an OTel
// span. When tracing is disabled the returned middleware is a pass-through.
func Middleware() registry.Middleware {
	return func(name string, td registry.ToolDefinition, next registry.ToolHandlerFunc) registry.ToolHandlerFunc {
		if !Enabled() {
			return next
		}

		tracer := otel.Tracer(tracerName)

		return func(ctx context.Context, req registry.CallToolRequest) (*registry.CallToolResult, error) {
			ctx, span := tracer.Start(ctx, "tool/"+name,
				trace.WithAttributes(
					attribute.String("mcp.tool.name", name),
				),
				trace.WithSpanKind(trace.SpanKindServer),
			)
			defer span.End()

			result, err := next(ctx, req)

			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			} else if result != nil && registry.IsResultError(result) {
				span.SetStatus(codes.Error, "tool returned error result")
			} else {
				span.SetStatus(codes.Ok, "")
			}

			return result, err
		}
	}
}
