// otel.go provides the OTel tracer / meter initialization that the rest of
// the server uses (HTTP middleware, SSE broadcaster, adapters, etc.).
//
// Phase 1 wires the global TracerProvider / MeterProvider according to the
// configured exporter:
//
//   - "stdout": uses go.opentelemetry.io/otel/exporters/stdout/stdouttrace
//     plus a stdout periodic metric reader.
//   - "otlp":    uses otlptracegrpc against HNSX_OTEL_OTLP_ENDPOINT (default
//     127.0.0.1:4317).
//   - "none":    leaves the global providers unset; Shutdown is a no-op.
package telemetry

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// OTelOptions configures the global TracerProvider / MeterProvider.
type OTelOptions struct {
	// ServiceName is set as the service.name resource attribute.
	ServiceName string
	// Exporter is "stdout", "otlp", or "none".
	Exporter string
	// OTLPEndpoint is the gRPC target for the otlp exporter (host:port).
	OTLPEndpoint string
}

// Provider bundles the OTel providers so the caller can shut them down
// cleanly at server exit.
type Provider struct {
	Tracer *sdktrace.TracerProvider
	Meter  *metric.MeterProvider
}

// Shutdown flushes + stops both providers.
func (p *Provider) Shutdown(ctx context.Context) error {
	if p == nil {
		return nil
	}
	var errs []error
	if p.Tracer != nil {
		if err := p.Tracer.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("tracer shutdown: %w", err))
		}
	}
	if p.Meter != nil {
		if err := p.Meter.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("meter shutdown: %w", err))
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}

// Init sets the global OTel TracerProvider + MeterProvider per opts. The
// returned Provider retains references so the caller can call Shutdown on
// shutdown.
func Init(ctx context.Context, opts OTelOptions) (*Provider, error) {
	if opts.ServiceName == "" {
		opts.ServiceName = "hnsx-server"
	}
	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(opts.ServiceName)),
		resource.WithTelemetrySDK(),
	)
	if err != nil {
		return nil, fmt.Errorf("otel: build resource: %w", err)
	}

	p := &Provider{}

	switch opts.Exporter {
	case "", "none":
		// Global providers remain unset; nothing to wire.
		return p, nil

	case "stdout":
		traceExp, err := stdouttrace.New(
			stdouttrace.WithPrettyPrint(),
		)
		if err != nil {
			return nil, fmt.Errorf("otel: stdouttrace: %w", err)
		}
		p.Tracer = sdktrace.NewTracerProvider(
			sdktrace.WithBatcher(traceExp, sdktrace.WithBatchTimeout(5*time.Second)),
			sdktrace.WithResource(res),
			sdktrace.WithSampler(sdktrace.AlwaysSample()),
		)
		p.Meter = metric.NewMeterProvider(
			metric.WithResource(res),
			// No periodic reader in stdout mode — metrics still measurable
			// in tests via manual reader. (Future PR can add periodic.)
		)

	case "otlp":
		endpoint := opts.OTLPEndpoint
		if endpoint == "" {
			endpoint = "127.0.0.1:4317"
		}
		traceExp, err := otlptrace.New(ctx,
			otlptracegrpc.NewClient(
				otlptracegrpc.WithEndpoint(endpoint),
				otlptracegrpc.WithInsecure(),
			),
		)
		if err != nil {
			return nil, fmt.Errorf("otel: otlp: %w", err)
		}
		p.Tracer = sdktrace.NewTracerProvider(
			sdktrace.WithBatcher(traceExp, sdktrace.WithBatchTimeout(5*time.Second)),
			sdktrace.WithResource(res),
			sdktrace.WithSampler(sdktrace.AlwaysSample()),
		)
		p.Meter = metric.NewMeterProvider(metric.WithResource(res))

	default:
		return nil, fmt.Errorf("otel: unknown exporter %q", opts.Exporter)
	}

	otel.SetTracerProvider(p.Tracer)
	otel.SetMeterProvider(p.Meter)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return p, nil
}
