// OpenTelemetry provider initialization and configuration.
package telemetry

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// ProviderConfig configures the OpenTelemetry provider.
type ProviderConfig struct {
	// ServiceName is the name of the service (required).
	ServiceName string

	// ServiceVersion is the version of the service.
	ServiceVersion string

	// Endpoint is the OTLP endpoint (e.g., "localhost:4317").
	// If empty, uses OTEL_EXPORTER_OTLP_ENDPOINT env var.
	Endpoint string

	// Protocol is "grpc" or "http". Default is "grpc".
	Protocol string

	// Insecure disables TLS. Default is false.
	Insecure bool

	// Debug enables content in span attributes.
	Debug bool

	// Headers are additional headers to send with requests.
	Headers map[string]string

	// BatchTimeout is the maximum time to wait before sending a batch.
	BatchTimeout time.Duration

	// ExportTimeout is the timeout for exporting spans.
	ExportTimeout time.Duration
}

// Provider wraps the OpenTelemetry TracerProvider with cleanup.
type Provider struct {
	tp     *sdktrace.TracerProvider
	tracer *Tracer
}

// InitProvider initializes OpenTelemetry with the given configuration.
// Returns a Provider that must be shut down when done.
func InitProvider(ctx context.Context, cfg ProviderConfig) (*Provider, error) {
	// Resolve endpoint from config or env
	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	}
	if endpoint == "" {
		return nil, fmt.Errorf("telemetry endpoint not configured (set endpoint or OTEL_EXPORTER_OTLP_ENDPOINT)")
	}

	// Strip protocol prefix if present
	endpoint = strings.TrimPrefix(endpoint, "http://")
	endpoint = strings.TrimPrefix(endpoint, "https://")

	// Resolve service name
	serviceName := cfg.ServiceName
	if serviceName == "" {
		serviceName = os.Getenv("OTEL_SERVICE_NAME")
	}
	if serviceName == "" {
		serviceName = "agent"
	}

	// Create resource
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("creating resource: %w", err)
	}

	// Create exporter
	var exporter sdktrace.SpanExporter
	protocol := cfg.Protocol
	if protocol == "" {
		protocol = "grpc"
	}

	switch protocol {
	case "grpc":
		opts := []otlptracegrpc.Option{
			otlptracegrpc.WithEndpoint(endpoint),
		}
		if cfg.Insecure {
			opts = append(opts, otlptracegrpc.WithInsecure())
		}
		if len(cfg.Headers) > 0 {
			opts = append(opts, otlptracegrpc.WithHeaders(cfg.Headers))
		}
		if cfg.ExportTimeout > 0 {
			opts = append(opts, otlptracegrpc.WithTimeout(cfg.ExportTimeout))
		}
		exporter, err = otlptracegrpc.New(ctx, opts...)

	case "http":
		opts := []otlptracehttp.Option{
			otlptracehttp.WithEndpoint(endpoint),
		}
		if cfg.Insecure {
			opts = append(opts, otlptracehttp.WithInsecure())
		}
		if len(cfg.Headers) > 0 {
			opts = append(opts, otlptracehttp.WithHeaders(cfg.Headers))
		}
		if cfg.ExportTimeout > 0 {
			opts = append(opts, otlptracehttp.WithTimeout(cfg.ExportTimeout))
		}
		exporter, err = otlptracehttp.New(ctx, opts...)

	default:
		return nil, fmt.Errorf("unknown protocol: %s (use 'grpc' or 'http')", protocol)
	}

	if err != nil {
		return nil, fmt.Errorf("creating exporter: %w", err)
	}

	// Configure batch processor
	batchOpts := []sdktrace.BatchSpanProcessorOption{}
	if cfg.BatchTimeout > 0 {
		batchOpts = append(batchOpts, sdktrace.WithBatchTimeout(cfg.BatchTimeout))
	}

	// Create tracer provider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter, batchOpts...),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	// Set as global provider
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Create and set global tracer
	tracer := NewTracer(serviceName, cfg.Debug)
	SetGlobalTracer(tracer)

	return &Provider{
		tp:     tp,
		tracer: tracer,
	}, nil
}

// Tracer returns the tracer for this provider.
func (p *Provider) Tracer() *Tracer {
	return p.tracer
}

// SetDebug enables or disables debug mode.
func (p *Provider) SetDebug(debug bool) {
	p.tracer.SetDebug(debug)
}

// Shutdown gracefully shuts down the provider.
func (p *Provider) Shutdown(ctx context.Context) error {
	return p.tp.Shutdown(ctx)
}

// ForceFlush forces a flush of all pending spans.
func (p *Provider) ForceFlush(ctx context.Context) error {
	return p.tp.ForceFlush(ctx)
}
