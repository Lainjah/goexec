// Package observability provides OpenTelemetry integration and audit logging.
package observability

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// Telemetry provides observability features.
type Telemetry interface {
	// StartSpan starts a new trace span.
	StartSpan(ctx context.Context, name string, opts ...SpanOption) (context.Context, func())

	// RecordMetric records a metric value.
	RecordMetric(name string, value float64, labels map[string]string)

	// RecordDuration records a duration metric.
	RecordDuration(name string, duration float64, labels map[string]string)

	// RecordCounter increments a counter.
	RecordCounter(name string, labels map[string]string)

	// SetGauge sets a gauge value.
	SetGauge(name string, value float64, labels map[string]string)
}

// SpanOption configures span creation.
type SpanOption func(*spanConfig)

type spanConfig struct {
	attributes []attribute.KeyValue
	kind       trace.SpanKind
}

// WithAttribute adds an attribute to the span.
func WithAttribute(key string, value interface{}) SpanOption {
	return func(c *spanConfig) {
		switch v := value.(type) {
		case string:
			c.attributes = append(c.attributes, attribute.String(key, v))
		case int:
			c.attributes = append(c.attributes, attribute.Int(key, v))
		case int64:
			c.attributes = append(c.attributes, attribute.Int64(key, v))
		case float64:
			c.attributes = append(c.attributes, attribute.Float64(key, v))
		case bool:
			c.attributes = append(c.attributes, attribute.Bool(key, v))
		}
	}
}

// WithSpanKind sets the span kind.
func WithSpanKind(kind trace.SpanKind) SpanOption {
	return func(c *spanConfig) {
		c.kind = kind
	}
}

// TelemetryConfig configures telemetry.
type TelemetryConfig struct {
	// ServiceName is the service name for tracing.
	ServiceName string

	// ServiceVersion is the service version.
	ServiceVersion string

	// Environment is the deployment environment.
	Environment string

	// EnableTracing enables distributed tracing.
	EnableTracing bool

	// EnableMetrics enables metrics collection.
	EnableMetrics bool

	// MetricsPrefix is the prefix for all metrics.
	MetricsPrefix string
}

// DefaultTelemetryConfig returns default configuration.
func DefaultTelemetryConfig() TelemetryConfig {
	return TelemetryConfig{
		ServiceName:    "goexec",
		ServiceVersion: "1.0.0",
		Environment:    "development",
		EnableTracing:  true,
		EnableMetrics:  true,
		MetricsPrefix:  "goexec_",
	}
}

// telemetry implements Telemetry.
type telemetry struct {
	config TelemetryConfig
	tracer trace.Tracer
	meter  metric.Meter

	// Metrics
	executionCounter    metric.Int64Counter
	executionDuration   metric.Float64Histogram
	activeExecutions    metric.Int64UpDownCounter
	errorCounter        metric.Int64Counter
	policyDeniedCounter metric.Int64Counter
}

// NewTelemetry creates a new telemetry instance.
func NewTelemetry(config TelemetryConfig) (Telemetry, error) {
	t := &telemetry{
		config: config,
		tracer: otel.Tracer(config.ServiceName),
		meter:  otel.Meter(config.ServiceName),
	}

	// Initialize metrics
	var err error

	t.executionCounter, err = t.meter.Int64Counter(
		config.MetricsPrefix+"executions_total",
		metric.WithDescription("Total number of command executions"),
	)
	if err != nil {
		return nil, err
	}

	t.executionDuration, err = t.meter.Float64Histogram(
		config.MetricsPrefix+"execution_duration_seconds",
		metric.WithDescription("Duration of command executions"),
	)
	if err != nil {
		return nil, err
	}

	t.activeExecutions, err = t.meter.Int64UpDownCounter(
		config.MetricsPrefix+"active_executions",
		metric.WithDescription("Number of currently active executions"),
	)
	if err != nil {
		return nil, err
	}

	t.errorCounter, err = t.meter.Int64Counter(
		config.MetricsPrefix+"errors_total",
		metric.WithDescription("Total number of execution errors"),
	)
	if err != nil {
		return nil, err
	}

	t.policyDeniedCounter, err = t.meter.Int64Counter(
		config.MetricsPrefix+"policy_denied_total",
		metric.WithDescription("Total number of policy denials"),
	)
	if err != nil {
		return nil, err
	}

	return t, nil
}

// StartSpan implements Telemetry.StartSpan.
func (t *telemetry) StartSpan(ctx context.Context, name string, opts ...SpanOption) (context.Context, func()) {
	if !t.config.EnableTracing {
		return ctx, func() {}
	}

	cfg := &spanConfig{
		kind: trace.SpanKindInternal,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	ctx, span := t.tracer.Start(ctx, name,
		trace.WithAttributes(cfg.attributes...),
		trace.WithSpanKind(cfg.kind),
	)

	return ctx, func() {
		span.End()
	}
}

// RecordMetric implements Telemetry.RecordMetric.
func (t *telemetry) RecordMetric(name string, value float64, labels map[string]string) {
	if !t.config.EnableMetrics {
		return
	}

	attrs := labelsToAttributes(labels)
	t.executionDuration.Record(context.Background(), value, metric.WithAttributes(attrs...))
}

// RecordDuration implements Telemetry.RecordDuration.
func (t *telemetry) RecordDuration(name string, duration float64, labels map[string]string) {
	if !t.config.EnableMetrics {
		return
	}

	attrs := labelsToAttributes(labels)
	t.executionDuration.Record(context.Background(), duration, metric.WithAttributes(attrs...))
}

// RecordCounter implements Telemetry.RecordCounter.
func (t *telemetry) RecordCounter(name string, labels map[string]string) {
	if !t.config.EnableMetrics {
		return
	}

	attrs := labelsToAttributes(labels)
	t.executionCounter.Add(context.Background(), 1, metric.WithAttributes(attrs...))
}

// SetGauge implements Telemetry.SetGauge.
func (t *telemetry) SetGauge(name string, value float64, labels map[string]string) {
	if !t.config.EnableMetrics {
		return
	}

	attrs := labelsToAttributes(labels)
	t.activeExecutions.Add(context.Background(), int64(value), metric.WithAttributes(attrs...))
}

// labelsToAttributes converts labels to OTEL attributes.
func labelsToAttributes(labels map[string]string) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, len(labels))
	for k, v := range labels {
		attrs = append(attrs, attribute.String(k, v))
	}
	return attrs
}

// NoopTelemetry returns a no-op telemetry implementation.
func NoopTelemetry() Telemetry {
	return &noopTelemetry{}
}

type noopTelemetry struct{}

func (t *noopTelemetry) StartSpan(ctx context.Context, name string, opts ...SpanOption) (context.Context, func()) {
	return ctx, func() {}
}

func (t *noopTelemetry) RecordMetric(name string, value float64, labels map[string]string)      {}
func (t *noopTelemetry) RecordDuration(name string, duration float64, labels map[string]string) {}
func (t *noopTelemetry) RecordCounter(name string, labels map[string]string)                    {}
func (t *noopTelemetry) SetGauge(name string, value float64, labels map[string]string)          {}
