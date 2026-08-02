# gh pr-graph

`gh pr-graph` is a GitHub CLI extension that opens a local web UI and displays pull requests as a directed graph based on their base and head branches.

The current MVP provides:

- Default discovery of open PRs authored by, assigned to, or requesting review from you
- GitHub PR search syntax from the UI
- Recursive discovery of open downstream stacked PRs
- Blue / green / gray nodes for your PRs, review requests, and other PRs
- Thick borders for ready PRs and thin borders plus a badge for drafts
- Aggregated review, CI, and conflict status
- Five-minute auto refresh that pauses in hidden or offline tabs
- A single Go binary with an embedded web UI; GitHub credentials never enter the browser

## Requirements

- `gh`, authenticated with `gh auth login`
- Go 1.23 or newer to build from source

## Build and run

```sh
make test
make build
./gh-pr-graph
```

Useful options:

```text
--no-open          print the URL without opening a browser
--port 8080        use a fixed local port
--hostname HOST    use a GitHub Enterprise hostname
```

For local extension development:

```sh
make build
gh extension install .
gh pr-graph
```

The server only listens on `127.0.0.1`. Press Ctrl-C in the terminal to stop it.

## Status

This is an initial implementation of Phase 1 in [DESIGN.md](DESIGN.md). Included/merged PR folding, search pagination beyond the first 100 results, richer team-review aggregation, and release automation remain to be implemented.

