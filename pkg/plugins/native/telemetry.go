package native

import (
	"context"
	"log/slog"
	"os"

	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/storage"
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
	// If OTEL_EXPORTER_OTLP_ENDPOINT is set, we could use that.
	// For now, let's keep STDOUT as default but check for a "silent" flag.
	
	var opts []stdouttrace.Option
	if os.Getenv("ALLOY_TELEMETRY_SILENT") == "true" {
		// No-op or very minimal output
	} else {
		opts = append(opts, stdouttrace.WithPrettyPrint())
	}

	exporter, err := stdouttrace.New(opts...)
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

func (t *TelemetryManager) ID() string { return "telemetry" }

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
