package tracing

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestInit(t *testing.T) {
	provider, err := Init("gateway")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if provider == nil {
		t.Fatal("expected tracer provider, got nil")
	}

	exporter := tracetest.NewInMemoryExporter()
	provider.RegisterSpanProcessor(sdktrace.NewSimpleSpanProcessor(exporter))

	tracer := provider.Tracer("test")
	_, span := tracer.Start(context.Background(), "test-operation")
	span.End()

	spans := exporter.GetSpans()

	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	var serviceName string

	for _, attr := range spans[0].Resource.Attributes() {
		if attr.Key == attribute.Key("service.name") {
			serviceName = attr.Value.AsString()
			break
		}
	}

	if serviceName != "gateway" {
		t.Fatalf("expected service.name gateway, got %q", serviceName)
	}

	if err := Shutdown(context.Background(), provider); err != nil {
		t.Fatalf("failed to shut down provider: %v", err)
	}
}

func TestInitRejectsEmptyServiceName(t *testing.T) {
	provider, err := Init("")

	if err == nil {
		t.Fatal("expected an error for empty service name")
	}

	if provider != nil {
		t.Fatal("expected nil provider for empty service name")
	}
}

func TestTraceContextPropagation(t *testing.T) {
	provider, err := Init("gateway")
	if err != nil {
		t.Fatalf("failed to initialize tracing: %v", err)
	}

	traceparent := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"

	ctx := otel.GetTextMapPropagator().Extract(
		context.Background(),
		propagation.MapCarrier{
			"traceparent": traceparent,
		},
	)

	spanContext := trace.SpanContextFromContext(ctx)

	if !spanContext.IsValid() {
		t.Fatal("expected valid span context")
	}

	if spanContext.TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("unexpected trace ID: %s", spanContext.TraceID())
	}

	if !spanContext.IsRemote() {
		t.Fatal("expected extracted span context to be remote")
	}

	carrier := propagation.MapCarrier{}

	otel.GetTextMapPropagator().Inject(ctx, carrier)

	if carrier.Get("traceparent") != traceparent {
		t.Fatalf("expected traceparent %q, got %q", traceparent, carrier.Get("traceparent"))
	}

	if err := Shutdown(context.Background(), provider); err != nil {
		t.Fatalf("failed to shut down tracing: %v", err)
	}
}

func TestShutdownRejectsNilProvider(t *testing.T) {
	err := Shutdown(context.Background(), nil)

	if err == nil {
		t.Fatal("expected an error when shutting down nil provider")
	}
}
