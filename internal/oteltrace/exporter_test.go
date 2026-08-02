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

func TestExporterSendsCommandArguments(t *testing.T) {
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
	exporter.Command("gh api graphql", []string{"api", "graphql", "-F", "owner=orangain"}, time.Now(), 1234, 0, nil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := exporter.Close(ctx); err != nil {
		t.Fatal(err)
	}

	select {
	case body := <-received:
		encoded, _ := json.Marshal(body)
		if string(encoded) == "" || !strings.Contains(string(encoded), "owner=orangain") {
			t.Errorf("payload does not contain command arguments: %s", encoded)
		}
	default:
		t.Fatal("collector did not receive a trace")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
