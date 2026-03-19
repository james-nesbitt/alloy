package native

import (
	"context"
	"log/slog"

	"github.com/jnesbitt/alloy-go/api"
	"github.com/jnesbitt/alloy-go/pkg/storage"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

// TelemetryManager manages the OpenTelemetry lifecycle and exports.
type TelemetryManager struct {
	tp *trace.TracerProvider
}

func NewTelemetryManager(ctx context.Context, logger *slog.Logger) (*TelemetryManager, error) {
	// For now, we use a simple STDOUT exporter. 
	// In the future, this can be swapped for OTLP/gRPC.
	exporter, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		return nil, err
	}

	tp := trace.NewTracerProvider(
		trace.WithBatcher(exporter),
		trace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("alloy-core"),
		)),
	)
	otel.SetTracerProvider(tp)

	return &TelemetryManager{tp: tp}, nil
}

func (t *TelemetryManager) ID() string { return "plugin-otel" }

func (t *TelemetryManager) Capabilities() []api.Capability {
	return []api.Capability{
		{Method: "trace", Description: "Internal tracing provider"},
	}
}

func (t *TelemetryManager) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	return api.Message{}, nil
}

func (t *TelemetryManager) Shutdown(ctx context.Context) error {
	if t.tp != nil {
		return t.tp.Shutdown(ctx)
	}
	return nil
}

// Ensure compatibility with the registry constructor pattern
func NewTelemetryManagerPlugin(ctx context.Context, logger *slog.Logger, state storage.StateStore) (any, error) {
	return NewTelemetryManager(ctx, logger)
}
