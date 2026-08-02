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

type Exporter struct {
	endpoint string
	client   *http.Client
	spans    chan span
	done     chan struct{}
	once     sync.Once
}

type span struct {
	TraceID   string
	SpanID    string
	Name      string
	Start     time.Time
	End       time.Time
	Args      []string
	ProcessID int
	ExitCode  int
	Error     string
}

func New(rawEndpoint string) (*Exporter, error) {
	endpoint, err := normalizeEndpoint(rawEndpoint)
	if err != nil {
		return nil, err
	}
	e := &Exporter{
		endpoint: endpoint,
		client:   &http.Client{Timeout: 5 * time.Second},
		spans:    make(chan span, 256),
		done:     make(chan struct{}),
	}
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

// Command records one external command as an OTEL span. Export failures never
// affect the command because tracing is diagnostic and deliberately best-effort.
func (e *Exporter) Command(name string, args []string, start time.Time, processID, exitCode int, commandErr error) {
	s := span{TraceID: randomHex(16), SpanID: randomHex(8), Name: name, Start: start, End: time.Now(), Args: append([]string(nil), args...), ProcessID: processID, ExitCode: exitCode}
	if commandErr != nil {
		s.Error = commandErr.Error()
	}
	select {
	case e.spans <- s:
	default:
	}
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
	for s := range e.spans {
		e.export(s)
	}
}

func (e *Exporter) export(s span) {
	statusCode := 1
	status := map[string]any{"code": statusCode}
	if s.Error != "" {
		status = map[string]any{"code": 2, "message": s.Error}
	}
	attrs := []any{
		map[string]any{"key": "process.executable.name", "value": map[string]any{"stringValue": "gh"}},
		map[string]any{"key": "process.command_args", "value": map[string]any{"arrayValue": map[string]any{"values": stringValues(append([]string{"gh"}, s.Args...))}}},
		map[string]any{"key": "process.pid", "value": map[string]any{"intValue": strconv.Itoa(s.ProcessID)}},
		map[string]any{"key": "process.exit.code", "value": map[string]any{"intValue": strconv.Itoa(s.ExitCode)}},
	}
	if s.Error != "" {
		attrs = append(attrs, map[string]any{"key": "error.type", "value": map[string]any{"stringValue": "_OTHER"}})
	}
	payload := map[string]any{"resourceSpans": []any{map[string]any{
		"resource": map[string]any{"attributes": []any{map[string]any{"key": "service.name", "value": map[string]any{"stringValue": "gh-pr-graph"}}}},
		"scopeSpans": []any{map[string]any{
			"scope": map[string]any{"name": "github.com/orangain/gh-pr-graph"},
			"spans": []any{map[string]any{"traceId": s.TraceID, "spanId": s.SpanID, "name": s.Name, "kind": 3, "startTimeUnixNano": strconv.FormatInt(s.Start.UnixNano(), 10), "endTimeUnixNano": strconv.FormatInt(s.End.UnixNano(), 10), "attributes": attrs, "status": status}},
		}},
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

func stringValues(values []string) []any {
	result := make([]any, 0, len(values))
	for _, value := range values {
		result = append(result, map[string]any{"stringValue": value})
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
