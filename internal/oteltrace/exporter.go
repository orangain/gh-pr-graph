package oteltrace

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const DefaultEndpoint = "http://localhost:4318/v1/traces"

type SpanKind int

const (
	SpanInternal SpanKind = 1
	SpanServer   SpanKind = 2
	SpanClient   SpanKind = 3
)

type Attributes map[string]any

type Span interface {
	End(error, Attributes)
}

type Tracer interface {
	Start(context.Context, string, SpanKind, Attributes) (context.Context, Span)
}

type Exporter struct {
	endpoint string
	client   *http.Client
	spans    chan spanData
	done     chan struct{}
	once     sync.Once
}

type spanContext struct{ traceID, spanID string }
type contextKey struct{}

type spanData struct {
	TraceID, SpanID, ParentSpanID string
	Name                          string
	Kind                          SpanKind
	Start, End                    time.Time
	Attributes                    Attributes
	Error                         string
}

type recordingSpan struct {
	exporter *Exporter
	data     spanData
	once     sync.Once
}

func New(rawEndpoint string) (*Exporter, error) {
	endpoint, err := normalizeEndpoint(rawEndpoint)
	if err != nil {
		return nil, err
	}
	e := &Exporter{endpoint: endpoint, client: &http.Client{Timeout: 5 * time.Second}, spans: make(chan spanData, 256), done: make(chan struct{})}
	go e.run()
	return e, nil
}

func normalizeEndpoint(raw string) (string, error) {
	if raw == "" || raw == "true" {
		return DefaultEndpoint, nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", &url.Error{Op: "parse", URL: raw, Err: errInvalidEndpoint{}}
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = "/v1/traces"
	}
	return u.String(), nil
}

type errInvalidEndpoint struct{}

func (errInvalidEndpoint) Error() string { return "endpoint must be an absolute URL" }

func (e *Exporter) Start(ctx context.Context, name string, kind SpanKind, attributes Attributes) (context.Context, Span) {
	traceID := randomHex(16)
	parentSpanID := ""
	if parent, ok := ctx.Value(contextKey{}).(spanContext); ok {
		traceID, parentSpanID = parent.traceID, parent.spanID
	}
	spanID := randomHex(8)
	s := &recordingSpan{exporter: e, data: spanData{TraceID: traceID, SpanID: spanID, ParentSpanID: parentSpanID, Name: name, Kind: kind, Start: time.Now(), Attributes: cloneAttributes(attributes)}}
	return context.WithValue(ctx, contextKey{}, spanContext{traceID: traceID, spanID: spanID}), s
}

func (s *recordingSpan) End(spanErr error, attributes Attributes) {
	s.once.Do(func() {
		s.data.End = time.Now()
		for key, value := range attributes {
			s.data.Attributes[key] = value
		}
		if spanErr != nil {
			s.data.Error = spanErr.Error()
			s.data.Attributes["error.type"] = "_OTHER"
		}
		select {
		case s.exporter.spans <- s.data:
		default:
		}
	})
}

func cloneAttributes(attributes Attributes) Attributes {
	result := make(Attributes, len(attributes))
	for key, value := range attributes {
		result[key] = value
	}
	return result
}

func (e *Exporter) Close(ctx context.Context) error {
	e.once.Do(func() { close(e.spans) })
	select {
	case <-e.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (e *Exporter) run() {
	defer close(e.done)
	for {
		first, ok := <-e.spans
		if !ok {
			return
		}
		batch := []spanData{first}
		timer := time.NewTimer(50 * time.Millisecond)
	collect:
		for len(batch) < 64 {
			select {
			case s, ok := <-e.spans:
				if !ok {
					if !timer.Stop() {
						<-timer.C
					}
					e.export(batch)
					return
				}
				batch = append(batch, s)
			case <-timer.C:
				break collect
			}
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		e.export(batch)
	}
}

func (e *Exporter) export(spans []spanData) {
	encodedSpans := make([]any, 0, len(spans))
	for _, s := range spans {
		status := map[string]any{"code": 1}
		if s.Error != "" {
			status = map[string]any{"code": 2, "message": s.Error}
		}
		encodedSpan := map[string]any{
			"traceId": s.TraceID, "spanId": s.SpanID, "name": s.Name, "kind": int(s.Kind),
			"startTimeUnixNano": strconv.FormatInt(s.Start.UnixNano(), 10), "endTimeUnixNano": strconv.FormatInt(s.End.UnixNano(), 10),
			"attributes": encodeAttributes(s.Attributes), "status": status,
		}
		if s.ParentSpanID != "" {
			encodedSpan["parentSpanId"] = s.ParentSpanID
		}
		encodedSpans = append(encodedSpans, encodedSpan)
	}
	payload := map[string]any{"resourceSpans": []any{map[string]any{
		"resource":   map[string]any{"attributes": encodeAttributes(Attributes{"service.name": "gh-pr-graph"})},
		"scopeSpans": []any{map[string]any{"scope": map[string]any{"name": "github.com/orangain/gh-pr-graph"}, "spans": encodedSpans}},
	}}}
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	req, err := http.NewRequest(http.MethodPost, e.endpoint, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.client.Do(req)
	if err == nil {
		_ = resp.Body.Close()
	}
}

func encodeAttributes(attributes Attributes) []any {
	result := make([]any, 0, len(attributes))
	for key, value := range attributes {
		encoded := map[string]any{}
		switch value := value.(type) {
		case string:
			encoded["stringValue"] = value
		case int:
			encoded["intValue"] = strconv.Itoa(value)
		case int64:
			encoded["intValue"] = strconv.FormatInt(value, 10)
		case bool:
			encoded["boolValue"] = value
		case []string:
			values := make([]any, 0, len(value))
			for _, item := range value {
				values = append(values, map[string]any{"stringValue": item})
			}
			encoded["arrayValue"] = map[string]any{"values": values}
		default:
			continue
		}
		result = append(result, map[string]any{"key": key, "value": encoded})
	}
	return result
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return strings.Repeat("0", n*2)
	}
	return hex.EncodeToString(b)
}
