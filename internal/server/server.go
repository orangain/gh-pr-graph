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
	"time"

	"github.com/orangain/gh-pr-graph/internal/graph"
)

//go:embed web/*
var assets embed.FS

type Loader interface {
	Load(context.Context, string) (graph.Result, error)
}

type progressiveLoader interface {
	LoadProgress(context.Context, string, func(current, total int, phase string)) (graph.Result, error)
}

type Server struct {
	loader Loader
	http   *http.Server
	ln     net.Listener
	mu     sync.Mutex
}

func New(loader Loader) *Server { return &Server{loader: loader} }

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
	mux.HandleFunc("GET /api/v1/graph", s.graph)
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

func (s *Server) Shutdown(ctx context.Context) error {
	if s.http == nil {
		return nil
	}
	return s.http.Shutdown(ctx)
}

func (s *Server) graph(w http.ResponseWriter, r *http.Request) {
	// Serialize refreshes so polling and manual refresh cannot multiply API cost.
	s.mu.Lock()
	defer s.mu.Unlock()
	if loader, ok := s.loader.(progressiveLoader); ok {
		s.graphStream(w, r, loader)
		return
	}
	result, err := s.loader.Load(r.Context(), r.URL.Query().Get("q"))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(result)
}

func (s *Server) graphStream(w http.ResponseWriter, r *http.Request, loader progressiveLoader) {
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	flusher, _ := w.(http.Flusher)
	encoder := json.NewEncoder(w)
	lastPercent := 0
	report := func(current, total int, phase string) {
		percent := progressPercent(current, total, phase)
		if percent < lastPercent {
			percent = lastPercent
		} else {
			lastPercent = percent
		}
		_ = encoder.Encode(map[string]any{"type": "progress", "current": current, "total": total, "phase": phase, "percent": percent})
		if flusher != nil {
			flusher.Flush()
		}
	}
	result, err := loader.LoadProgress(r.Context(), r.URL.Query().Get("q"), report)
	if err != nil {
		_ = encoder.Encode(map[string]any{"type": "error", "error": err.Error()})
		return
	}
	_ = encoder.Encode(map[string]any{"type": "progress", "current": 1, "total": 1, "phase": "Complete", "percent": 100})
	_ = encoder.Encode(map[string]any{"type": "result", "result": result})
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
		return 20 + int(ratio*30)
	case "Inspecting included pull requests":
		return 50 + int(ratio*50)
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
