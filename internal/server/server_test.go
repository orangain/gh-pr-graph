package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/orange/gh-pr-graph/internal/graph"
)

type fakeLoader struct{ query string }

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
