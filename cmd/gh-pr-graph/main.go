package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/orangain/gh-pr-graph/internal/github"
	"github.com/orangain/gh-pr-graph/internal/oteltrace"
	"github.com/orangain/gh-pr-graph/internal/server"
)

func main() {
	var port int
	var noOpen bool
	var hostname string
	var traceOTEL optionalValue
	flag.IntVar(&port, "port", 0, "local server port (0 selects a free port)")
	flag.BoolVar(&noOpen, "no-open", false, "do not open the browser")
	flag.StringVar(&hostname, "hostname", "", "GitHub hostname (defaults to gh configuration)")
	flag.Var(&traceOTEL, "trace-otel", "send gh command traces to OTLP/HTTP (optional endpoint; use --trace-otel=URL)")
	flag.Parse()
	if traceOTEL.set && traceOTEL.value == "true" && flag.NArg() == 1 {
		traceOTEL.value = flag.Arg(0)
	} else if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "gh pr-graph: unexpected arguments:", flag.Args())
		os.Exit(2)
	}

	client := github.New(hostname)
	var exporter *oteltrace.Exporter
	if traceOTEL.set {
		var err error
		exporter, err = oteltrace.New(traceOTEL.value)
		if err != nil {
			fmt.Fprintln(os.Stderr, "gh pr-graph: invalid --trace-otel:", err)
			os.Exit(2)
		}
		client.Tracer = exporter
	}
	app := server.New(client)
	address, err := app.Start(port)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gh pr-graph:", err)
		os.Exit(1)
	}
	fmt.Println("Opened", address)
	fmt.Println("Press Ctrl-C to stop.")
	if !noOpen {
		if err := server.OpenBrowser(address); err != nil {
			fmt.Fprintln(os.Stderr, "Could not open browser:", err)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = app.Shutdown(shutdown)
	if exporter != nil {
		_ = exporter.Close(shutdown)
	}
}

type optionalValue struct {
	set   bool
	value string
}

func (v *optionalValue) String() string         { return v.value }
func (v *optionalValue) Set(value string) error { v.set = true; v.value = value; return nil }
func (v *optionalValue) IsBoolFlag() bool       { return true }
