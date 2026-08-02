package oteltrace

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestNormalizeEndpoint(t *testing.T) {
	tests := map[string]string{
		"":                        DefaultEndpoint,
		"true":                    DefaultEndpoint,
		"http://localhost:4318":   "http://localhost:4318/v1/traces",
		"http://collector/custom": "http://collector/custom",
	}
	for input, want := range tests {
		got, err := normalizeEndpoint(input)
		if err != nil {
			t.Fatalf("normalizeEndpoint(%q): %v", input, err)
		}
		if got != want {
			t.Errorf("normalizeEndpoint(%q) = %q, want %q", input, got, want)
		}
	}
	if _, err := normalizeEndpoint("localhost:4318"); err == nil {
		t.Error("normalizeEndpoint accepted a URL without a scheme")
	}
}

func TestExporterPreservesParentChildRelationship(t *testing.T) {
	received := make(chan map[string]any, 1)
	exporter, err := New("http://collector.test")
	if err != nil {
		t.Fatal(err)
	}
	exporter.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		received <- body
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	ctx, root := exporter.Start(context.Background(), "GET /api/v1/graph", SpanServer, nil)
	_, child := exporter.Start(ctx, "gh api graphql: search", SpanClient, Attributes{"process.command_args": []string{"gh", "api", "graphql", "-F", "owner=orangain"}})
	child.End(nil, Attributes{"process.exit.code": 0})
	root.End(nil, nil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := exporter.Close(ctx); err != nil {
		t.Fatal(err)
	}

	payload := <-received
	spans := payloadSpans(t, payload)
	if len(spans) != 2 {
		t.Fatalf("exported spans = %d, want 2", len(spans))
	}
	childSpan, rootSpan := spans[0], spans[1]
	if childSpan["traceId"] != rootSpan["traceId"] {
		t.Errorf("trace IDs differ: child %v, root %v", childSpan["traceId"], rootSpan["traceId"])
	}
	if childSpan["parentSpanId"] != rootSpan["spanId"] {
		t.Errorf("child parentSpanId = %v, root spanId = %v", childSpan["parentSpanId"], rootSpan["spanId"])
	}
	encoded, _ := json.Marshal(payload)
	if !strings.Contains(string(encoded), "owner=orangain") {
		t.Errorf("payload does not contain command arguments: %s", encoded)
	}
}

func payloadSpans(t *testing.T, payload map[string]any) []map[string]any {
	t.Helper()
	resources := payload["resourceSpans"].([]any)
	scopes := resources[0].(map[string]any)["scopeSpans"].([]any)
	rawSpans := scopes[0].(map[string]any)["spans"].([]any)
	spans := make([]map[string]any, 0, len(rawSpans))
	for _, span := range rawSpans {
		spans = append(spans, span.(map[string]any))
	}
	return spans
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
