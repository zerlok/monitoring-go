package scraper

import (
	"context"
	"fmt"
	"github.com/zerlok/monitoring-go"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"os"
	"sync"
)

type OtelTracingFactory struct {
	Options        *OtelTracingOptions
	registerOnce   sync.Once
	unregisterOnce sync.Once
	provider       *sdktrace.TracerProvider
	tracer         trace.Tracer
}

type OtelTracingOptions struct {
	ApplicationName string
	TraceProvider   *sdktrace.TracerProvider
	Exporter        sdktrace.SpanExporter
}

func (f *OtelTracingFactory) Register(ctx context.Context, _ *Config) (ok bool, err error) {
	f.registerOnce.Do(func() {
		if f.Options == nil {
			f.Options = &OtelTracingOptions{}
		}

		provider := f.Options.TraceProvider
		if provider == nil {
			provider, err = NewOtelTraceProvider(f.Options)
			if err != nil {
				err = fmt.Errorf("failed to setup otel tracing: %w", err)
				return
			}
		}

		tracer := provider.Tracer(f.Options.ApplicationName)

		f.provider = provider
		f.tracer = tracer
		ok = true
	})

	return
}

func (f *OtelTracingFactory) Create(ctx context.Context, op monitoring.OperationContext) (Scraper, error) {
	ctx, span := f.tracer.Start(ctx, op.Name())

	return &otelTrace{ctx, op, span}, nil
}

func (f *OtelTracingFactory) Unregister(ctx context.Context) {
	f.unregisterOnce.Do(func() {
		tp := f.provider
		f.provider = nil
		f.tracer = nil

		_ = tp.ForceFlush(ctx)
		_ = tp.Shutdown(ctx)
	})
}

type otelTrace struct {
	ctx  context.Context
	op   monitoring.OperationContext
	span trace.Span
}

var _ Scraper = (*otelTrace)(nil)

func (t *otelTrace) Context() context.Context {
	return t.ctx
}

func (t *otelTrace) Operation() monitoring.OperationContext {
	return t.op
}

func (t *otelTrace) AddEvent(name string) {
	t.span.AddEvent(name)
}

func (t *otelTrace) AddError(err error) {
	t.span.RecordError(err)
}

func (t *otelTrace) End() {
	t.op.Finish(nil)

	err := t.op.Err()
	t.AddError(err)

	if err != nil {
		t.span.SetStatus(codes.Error, err.Error())
	} else {
		t.span.SetStatus(codes.Ok, "")
	}

	t.span.End(trace.WithTimestamp(*t.op.FinishedAt()), trace.WithStackTrace(err != nil))
}

func (t *otelTrace) EndError(err error) {
	t.op.Finish(err)

	t.End()
}

func NewOtelTraceProvider(options *OtelTracingOptions) (tr *sdktrace.TracerProvider, err error) {
	exp := options.Exporter
	if exp == nil {
		exp, err = NewOtelSpanExporter(os.Getenv("OTEL_EXPORTER_TYPE"))
		if err != nil {
			return
		}
	}

	tr = sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(resource.Default()),
	)

	return
}

func NewOtelSpanExporter(exporterType string) (exp sdktrace.SpanExporter, err error) {
	switch exporterType {
	case "jaeger":
		exp, err = jaeger.New(jaeger.WithCollectorEndpoint())
	case "stdout":
	default:
		exp, err = stdouttrace.New(
			stdouttrace.WithPrettyPrint(),
		)
	}

	return
}
