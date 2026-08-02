package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/orange/gh-pr-graph/internal/github"
	"github.com/orange/gh-pr-graph/internal/server"
)

func main() {
	var port int
	var noOpen bool
	var hostname string
	flag.IntVar(&port, "port", 0, "local server port (0 selects a free port)")
	flag.BoolVar(&noOpen, "no-open", false, "do not open the browser")
	flag.StringVar(&hostname, "hostname", "", "GitHub hostname (defaults to gh configuration)")
	flag.Parse()

	client := github.New(hostname)
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
}
