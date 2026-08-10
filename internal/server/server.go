package server

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/orangain/gh-pr-graph/internal/graph"
	"github.com/orangain/gh-pr-graph/internal/oteltrace"
)

//go:embed web/*
var assets embed.FS

const DefaultPort = 8787

type Loader interface {
	Load(context.Context, graph.SearchOptions) (graph.Result, error)
}

type progressiveLoader interface {
	LoadProgress(context.Context, graph.SearchOptions, func(current, total int, phase string, collected int)) (graph.Result, error)
}

type includedLoader interface {
	LoadIncluded(context.Context, []*graph.PullRequest, func(current, total int, phase string)) ([]graph.IncludedUpdate, error)
}

type inspectLoader interface {
	InspectPullRequest(context.Context, *graph.PullRequest) (graph.IncludedUpdate, error)
}

type Server struct {
	loader  Loader
	http    *http.Server
	ln      net.Listener
	mu      sync.Mutex
	tracer  oteltrace.Tracer
	version string
}

func New(loader Loader) *Server { return &Server{loader: loader, version: "dev"} }

func (s *Server) SetTracer(tracer oteltrace.Tracer) { s.tracer = tracer }

func (s *Server) SetVersion(version string) {
	if version != "" {
		s.version = version
	}
}

func (s *Server) Start(port int) (string, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return "", err
	}
	s.ln = ln
	web, err := fs.Sub(assets, "web")
	if err != nil {
		return "", err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/meta", s.meta)
	mux.HandleFunc("GET /api/v1/graph", s.graph)
	mux.HandleFunc("POST /api/v1/inspect", s.inspect)
	mux.HandleFunc("POST /api/v1/included", s.included)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.Handle("/", http.FileServer(http.FS(web)))
	s.http = &http.Server{Handler: securityHeaders(mux), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := s.http.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("server: %v", err)
		}
	}()
	return "http://" + ln.Addr().String(), nil
}

func (s *Server) meta(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]string{"version": s.version})
}

func (s *Server) StartPreferred(port int) (string, error) {
	address, err := s.Start(port)
	if err == nil || !shouldFallbackPort(err) {
		return address, err
	}
	return s.Start(0)
}

func shouldFallbackPort(err error) bool { return errors.Is(err, syscall.EADDRINUSE) }

func (s *Server) Shutdown(ctx context.Context) error {
	if s.http == nil {
		return nil
	}
	return s.http.Shutdown(ctx)
}

func (s *Server) graph(w http.ResponseWriter, r *http.Request) {
	options := searchOptions(r)
	var requestSpan oteltrace.Span
	if s.tracer != nil {
		ctx, span := s.tracer.Start(r.Context(), "GET /api/v1/graph", oteltrace.SpanServer, oteltrace.Attributes{
			"http.request.method": "GET",
			"http.route":          "/api/v1/graph",
			"pr.search_query":     options.Query,
		})
		r = r.WithContext(ctx)
		requestSpan = span
	}
	var requestErr error
	statusCode := http.StatusOK
	defer func() {
		if requestSpan != nil {
			requestSpan.End(requestErr, oteltrace.Attributes{"http.response.status_code": statusCode})
		}
	}()
	// Serialize refreshes so polling and manual refresh cannot multiply API cost.
	s.mu.Lock()
	defer s.mu.Unlock()
	if loader, ok := s.loader.(progressiveLoader); ok {
		requestErr = s.graphStream(w, r, loader, options)
		return
	}
	result, err := s.loader.Load(r.Context(), options)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err != nil {
		requestErr = err
		statusCode = http.StatusBadGateway
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(result)
}

func (s *Server) graphStream(w http.ResponseWriter, r *http.Request, loader progressiveLoader, options graph.SearchOptions) error {
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	flusher, _ := w.(http.Flusher)
	encoder := json.NewEncoder(w)
	lastPercent := 0
	report := func(current, total int, phase string, collected int) {
		percent := progressPercent(current, total, phase)
		if percent < lastPercent {
			percent = lastPercent
		} else {
			lastPercent = percent
		}
		_ = encoder.Encode(map[string]any{"type": "progress", "current": current, "total": total, "phase": phase, "percent": percent, "collected": collected})
		if flusher != nil {
			flusher.Flush()
		}
	}
	result, err := loader.LoadProgress(r.Context(), options, report)
	if err != nil {
		_ = encoder.Encode(map[string]any{"type": "error", "error": err.Error()})
		return err
	}
	collected := 0
	for _, node := range result.Nodes {
		if node.Kind == "pullRequest" {
			collected++
		}
	}
	_ = encoder.Encode(map[string]any{"type": "progress", "current": 0, "total": 1, "phase": "Inspecting pull request commits", "percent": 65, "collected": collected})
	_ = encoder.Encode(map[string]any{"type": "result", "result": result})
	return nil
}

func searchOptions(r *http.Request) graph.SearchOptions {
	query := r.URL.Query()
	explicitScopes := query.Has("authored") || query.Has("assigned") || query.Has("reviewRequested")
	options := graph.SearchOptions{Query: query.Get("q"), Authored: true, Assigned: true, ReviewRequested: true}
	if explicitScopes {
		options.Authored = query.Get("authored") == "1"
		options.Assigned = query.Get("assigned") == "1"
		options.ReviewRequested = query.Get("reviewRequested") == "1"
	}
	return options
}

func (s *Server) inspect(w http.ResponseWriter, r *http.Request) {
	loader, ok := s.loader.(inspectLoader)
	if !ok {
		http.Error(w, "pull request inspection is not supported", http.StatusNotImplemented)
		return
	}
	var pr graph.PullRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&pr); err != nil || pr.ID == "" {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	var span oteltrace.Span
	if s.tracer != nil {
		ctx, span = s.tracer.Start(ctx, "POST /api/v1/inspect", oteltrace.SpanServer, oteltrace.Attributes{"http.request.method": "POST", "http.route": "/api/v1/inspect", "pr.id": pr.ID})
	}
	update, err := loader.InspectPullRequest(ctx, &pr)
	if span != nil {
		span.End(err, oteltrace.Attributes{"pr.included_count": len(update.IncludedPullRequests)})
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(update)
}

func (s *Server) included(w http.ResponseWriter, r *http.Request) {
	loader, ok := s.loader.(includedLoader)
	if !ok {
		http.Error(w, "included pull requests are not supported", http.StatusNotImplemented)
		return
	}
	var request struct {
		PullRequests []*graph.PullRequest `json:"pullRequests"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 10<<20))
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if len(request.PullRequests) != 1 {
		http.Error(w, "exactly one containing pull request is required", http.StatusBadRequest)
		return
	}
	var requestSpan oteltrace.Span
	if s.tracer != nil {
		ctx, span := s.tracer.Start(r.Context(), "POST /api/v1/included", oteltrace.SpanServer, oteltrace.Attributes{
			"http.request.method": "POST",
			"http.route":          "/api/v1/included",
			"pr.count":            len(request.PullRequests),
			"pr.id":               request.PullRequests[0].ID,
			"pr.included_count":   len(request.PullRequests[0].IncludedPRs),
		})
		r = r.WithContext(ctx)
		requestSpan = span
	}
	var requestErr error
	defer func() {
		if requestSpan != nil {
			requestSpan.End(requestErr, oteltrace.Attributes{"http.response.status_code": http.StatusOK})
		}
	}()
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	flusher, _ := w.(http.Flusher)
	encoder := json.NewEncoder(w)
	report := func(current, total int, phase string) {
		percent := progressPercent(current, total, phase)
		_ = encoder.Encode(map[string]any{"type": "progress", "current": current, "total": total, "phase": phase, "percent": percent})
		if flusher != nil {
			flusher.Flush()
		}
	}
	updates, err := loader.LoadIncluded(r.Context(), request.PullRequests, report)
	if err != nil {
		requestErr = err
		_ = encoder.Encode(map[string]any{"type": "error", "error": err.Error()})
		return
	}
	_ = encoder.Encode(map[string]any{"type": "progress", "current": 1, "total": 1, "phase": "Complete", "percent": 100})
	_ = encoder.Encode(map[string]any{"type": "result", "result": updates})
}

func progressPercent(current, total int, phase string) int {
	if total <= 0 {
		return 0
	}
	ratio := float64(current) / float64(total)
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	switch phase {
	case "Searching pull requests":
		return int(ratio * 20)
	case "Discovering stacked pull requests":
		return 20 + int(ratio*45)
	case "Inspecting included pull requests":
		return 65 + int(ratio*35)
	case "Fetching included pull requests":
		return 90 + int(ratio*10)
	default:
		return int(ratio * 100)
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' https: data:; style-src 'self'; script-src 'self'; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

func OpenBrowser(rawURL string) error {
	if _, err := url.ParseRequestURI(rawURL); err != nil {
		return err
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	default:
		cmd = exec.Command("xdg-open", rawURL)
	}
	return cmd.Start()
}
