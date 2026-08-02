.PHONY: build test run

build:
	go build -o gh-pr-graph ./cmd/gh-pr-graph

test:
	go test ./...

run:
	go run ./cmd/gh-pr-graph

