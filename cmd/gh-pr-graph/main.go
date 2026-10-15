package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"syscall"
	"time"

	"github.com/orangain/gh-pr-graph/internal/demo"
	"github.com/orangain/gh-pr-graph/internal/github"
	"github.com/orangain/gh-pr-graph/internal/oteltrace"
	"github.com/orangain/gh-pr-graph/internal/server"
)

const projectURL = "https://github.com/orangain/gh-pr-graph"

var version = "dev"

// ownerPattern and repoPattern accept GitHub names only so the values cannot inject extra search qualifiers.
var (
	ownerPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?$`)
	repoPattern  = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?/[A-Za-z0-9._-]+$`)
)

func main() {
	var port int
	var noOpen bool
	var hostname string
	var owner string
	var repo string
	flag.IntVar(&port, "port", server.DefaultPort, "local server port (0 selects a free port)")
	flag.BoolVar(&noOpen, "no-open", false, "do not open the browser")
	flag.StringVar(&hostname, "hostname", "", "GitHub hostname (defaults to gh configuration)")
	flag.StringVar(&owner, "owner", "", "only show pull requests in repositories owned by this user or organization")
	flag.StringVar(&repo, "repo", "", "only show pull requests in this OWNER/REPO repository")
	flag.Parse()
	portExplicit := false
	flag.Visit(func(current *flag.Flag) {
		if current.Name == "port" {
			portExplicit = true
		}
	})
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "gh pr-graph: unexpected arguments:", flag.Args())
		os.Exit(2)
	}
	if owner != "" && !ownerPattern.MatchString(owner) {
		fmt.Fprintln(os.Stderr, "gh pr-graph: invalid --owner:", owner)
		os.Exit(2)
	}
	if repo != "" && !repoPattern.MatchString(repo) {
		fmt.Fprintln(os.Stderr, "gh pr-graph: invalid --repo (want OWNER/REPO):", repo)
		os.Exit(2)
	}
	if owner != "" && repo != "" {
		fmt.Fprintln(os.Stderr, "gh pr-graph: --owner and --repo cannot be used together")
		os.Exit(2)
	}
	fmt.Printf("gh pr-graph %s\n%s\n", version, projectURL)

	client := github.New(hostname)
	var loader server.Loader = client
	if os.Getenv("GH_PR_GRAPH_DEMO") == "1" {
		loader = demo.New()
		fmt.Fprintln(os.Stderr, "gh pr-graph: GH_PR_GRAPH_DEMO enabled; using demo data")
	}
	var exporter *oteltrace.Exporter
	if endpoint, enabled := os.LookupEnv("GH_PR_GRAPH_TRACE_OTEL"); enabled {
		if endpoint == "1" || endpoint == "true" {
			endpoint = ""
		}
		var err error
		exporter, err = oteltrace.New(endpoint)
		if err != nil {
			fmt.Fprintln(os.Stderr, "gh pr-graph: invalid GH_PR_GRAPH_TRACE_OTEL:", err)
			os.Exit(2)
		}
		client.Tracer = exporter
		fmt.Fprintf(os.Stderr, "gh pr-graph: GH_PR_GRAPH_TRACE_OTEL enabled; exporting traces to %s\n", exporter.Endpoint())
	}
	app := server.New(loader)
	app.SetVersion(version)
	app.SetOwner(owner)
	app.SetRepo(repo)
	if exporter != nil {
		app.SetTracer(exporter)
	}
	var address string
	var err error
	if portExplicit {
		address, err = app.Start(port)
	} else {
		address, err = app.StartPreferred(port)
	}
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
