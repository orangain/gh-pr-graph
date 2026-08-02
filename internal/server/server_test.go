package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/orangain/gh-pr-graph/internal/graph"
)

type fakeLoader struct{ query string }

type progressLoader struct{ fakeLoader }

func (f *progressLoader) LoadProgress(_ context.Context, _ string, progress func(int, int, string)) (graph.Result, error) {
	progress(1, 1, "Searching pull requests")
	progress(1, 1, "Discovering stacked pull requests")
	return graph.Result{UpdatedAt: time.Unix(1, 0)}, nil
}

func (f *progressLoader) LoadIncluded(_ context.Context, prs []*graph.PullRequest, progress func(int, int, string)) ([]graph.IncludedUpdate, error) {
	progress(len(prs), len(prs), "Inspecting included pull requests")
	return []graph.IncludedUpdate{{PullRequestID: "pr1", IncludedPullRequests: []graph.IncludedPullRequest{{ID: "included1", Number: 1}}}}, nil
}

func (f *fakeLoader) Load(_ context.Context, query string) (graph.Result, error) {
	f.query = query
	return graph.Result{UpdatedAt: time.Unix(1, 0)}, nil
}

func TestGraphHandler(t *testing.T) {
	loader := &fakeLoader{}
	s := New(loader)
	recorder := httptest.NewRecorder()
	s.graph(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/graph?q=is%3Aopen", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	if loader.query != "is:open" {
		t.Fatalf("query = %q", loader.query)
	}
	var result graph.Result
	if err := json.NewDecoder(recorder.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
}

func TestSecurityHeaders(t *testing.T) {
	handler := securityHeaders(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if got := recorder.Header().Get("Content-Security-Policy"); got == "" {
		t.Fatal("Content-Security-Policy is missing")
	}
}

func TestProgressPercent(t *testing.T) {
	tests := []struct {
		current, total int
		phase          string
		want           int
	}{
		{1, 2, "Searching pull requests", 12},
		{1, 2, "Discovering stacked pull requests", 45},
		{1, 2, "Inspecting included pull requests", 77},
		{1, 2, "Fetching included pull requests", 95},
		{1, 1, "Complete", 100},
	}
	for _, tt := range tests {
		if got := progressPercent(tt.current, tt.total, tt.phase); got != tt.want {
			t.Errorf("progressPercent(%d, %d, %q) = %d, want %d", tt.current, tt.total, tt.phase, got, tt.want)
		}
	}
}

func TestGraphStreamsProgressAndResult(t *testing.T) {
	s := New(&progressLoader{})
	recorder := httptest.NewRecorder()
	s.graph(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/graph", nil))
	body := recorder.Body.String()
	if !strings.Contains(body, `"percent":65`) || !strings.Contains(body, `"type":"result"`) {
		t.Fatalf("unexpected stream: %s", body)
	}
	if got := recorder.Header().Get("Content-Type"); !strings.Contains(got, "application/x-ndjson") {
		t.Fatalf("content type = %q", got)
	}
}

func TestIncludedStreamsProgressAndUpdates(t *testing.T) {
	s := New(&progressLoader{})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/included", strings.NewReader(`{"pullRequests":[{"id":"pr1"}]}`))
	s.included(recorder, request)
	body := recorder.Body.String()
	if !strings.Contains(body, `"percent":100`) || !strings.Contains(body, `"pullRequestId":"pr1"`) || !strings.Contains(body, `"type":"result"`) {
		t.Fatalf("unexpected stream: %s", body)
	}
}
