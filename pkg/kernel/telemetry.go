package kernel

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

type Telemetry struct {
	meter metric.Meter
	tp    *sdktrace.TracerProvider
	srv   *http.Server

	msgCounter  metric.Int64Counter
	errCounter  metric.Int64Counter
	pluginGauge metric.Int64UpDownCounter
}

func initTelemetry(metricsAddr string) (*Telemetry, error) {
	// 1. Setup Metrics (Prometheus)
	metricsExporter, err := prometheus.New()
	if err != nil {
		return nil, err
	}

	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(metricsExporter))
	otel.SetMeterProvider(meterProvider)

	meter := otel.Meter("alloy-kernel")

	msgCounter, _ := meter.Int64Counter("alloy_messages_total",
		metric.WithDescription("Total number of messages routed by kernel"))

	errCounter, _ := meter.Int64Counter("alloy_errors_total",
		metric.WithDescription("Total number of errors encountered during routing"))

	pluginGauge, _ := meter.Int64UpDownCounter("alloy_plugins_active",
		metric.WithDescription("Number of active plugins"))

	// 2. Setup Tracing (OpenTelemetry)
	var tp *sdktrace.TracerProvider
	if os.Getenv("ALLOY_TELEMETRY_SILENT") != "true" {
		traceExporter, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
		if err == nil {
			tp = sdktrace.NewTracerProvider(
				sdktrace.WithBatcher(traceExporter),
				sdktrace.WithResource(resource.NewWithAttributes(
					semconv.SchemaURL,
					semconv.ServiceNameKey.String("alloy-core"),
				)),
			)
			otel.SetTracerProvider(tp)
		}
	}

	// Expose prometheus metrics
	var srv *http.Server
	if metricsAddr != "" {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		srv = &http.Server{Addr: metricsAddr, Handler: mux}
		go func() {
			fmt.Printf("Telemetry: Metrics server listening on %s/metrics\n", metricsAddr)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("Telemetry: Metrics server failed: %v", err)
			}
		}()
	}

	return &Telemetry{
		meter:       meter,
		tp:          tp,
		srv:         srv,
		msgCounter:  msgCounter,
		errCounter:  errCounter,
		pluginGauge: pluginGauge,
	}, nil
}

func (t *Telemetry) Shutdown(ctx context.Context) error {
	if t == nil {
		return nil
	}
	var errs []error
	if t.srv != nil {
		if err := t.srv.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if t.tp != nil {
		if err := t.tp.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("telemetry shutdown errors: %v", errs)
	}
	return nil
}

func (t *Telemetry) RecordMessage(ctx context.Context, sender, target, method string) {
	if t == nil {
		return
	}
	t.msgCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("alloy.msg.sender", sender),
		attribute.String("alloy.msg.target", target),
		attribute.String("alloy.msg.method", method),
	))
}

func (t *Telemetry) RecordError(ctx context.Context, target, errorType string) {
	if t == nil {
		return
	}
	t.errCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("alloy.target", target),
		attribute.String("alloy.err_type", errorType),
	))
}

func (t *Telemetry) PluginCountChange(ctx context.Context, delta int64) {
	if t == nil {
		return
	}
	t.pluginGauge.Add(ctx, delta)
}
