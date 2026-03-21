package kernel

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

type Telemetry struct {
	meter metric.Meter
	
	msgCounter  metric.Int64Counter
	errCounter  metric.Int64Counter
	pluginGauge metric.Int64UpDownCounter
}

func initTelemetry() (*Telemetry, error) {
	exporter, err := prometheus.New()
	if err != nil {
		return nil, err
	}
	
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
	otel.SetMeterProvider(provider)
	
	meter := otel.Meter("alloy-kernel")
	
	msgCounter, _ := meter.Int64Counter("alloy_messages_total", 
		metric.WithDescription("Total number of messages routed by kernel"))
	
	errCounter, _ := meter.Int64Counter("alloy_errors_total", 
		metric.WithDescription("Total number of errors encountered during routing"))
	
	pluginGauge, _ := meter.Int64UpDownCounter("alloy_plugins_active",
		metric.WithDescription("Number of active plugins"))

	// Expose prometheus metrics
	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		fmt.Printf("Telemetry: Metrics server listening on :2112/metrics\n")
		// Use a unique port for Alloy metrics
		if err := http.ListenAndServe(":2112", mux); err != nil {
			log.Printf("Telemetry: Metrics server failed: %v", err)
		}
	}()

	return &Telemetry{
		meter:       meter,
		msgCounter:  msgCounter,
		errCounter:  errCounter,
		pluginGauge: pluginGauge,
	}, nil
}

func (t *Telemetry) RecordMessage(ctx context.Context, sender, target, method string) {
	if t == nil { return }
	t.msgCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("alloy.msg.sender", sender),
		attribute.String("alloy.msg.target", target),
		attribute.String("alloy.msg.method", method),
	))
}

func (t *Telemetry) RecordError(ctx context.Context, target, errorType string) {
	if t == nil { return }
	t.errCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("alloy.target", target),
		attribute.String("alloy.err_type", errorType),
	))
}

func (t *Telemetry) PluginCountChange(ctx context.Context, delta int64) {
	if t == nil { return }
	t.pluginGauge.Add(ctx, delta)
}
