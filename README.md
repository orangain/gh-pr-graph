# gh pr-graph

`gh pr-graph` is a GitHub CLI extension that opens a local web UI and displays pull requests as a directed graph based on their base and head branches.

The current MVP provides:

- Default discovery of open PRs authored by, assigned to, or requesting review from you
- GitHub PR search syntax from the UI
- Recursive discovery of open downstream stacked PRs
- Blue / green / gray nodes for your PRs, review requests, and other PRs
- Thick borders for ready PRs and thin borders plus a badge for drafts
- Aggregated review, CI, and conflict status
- Collapsible discovery of merged PRs detected from a PR's commit messages
- Early graph rendering followed by automatic batched Included PR hydration
- A visible loading progress indicator during GitHub API requests
- Repository lanes that keep repository-to-PR edges short
- Five-minute auto refresh that pauses in hidden or offline tabs
- A single Go binary with an embedded web UI; GitHub credentials never enter the browser

## Requirements

- `gh`, authenticated with `gh auth login`
- Go 1.23 or newer only when building from source

## Install

After this repository has been published and its first release has been created:

```sh
gh extension install orangain/gh-pr-graph
gh pr-graph
```

Users do not need Go; `gh` downloads the release binary matching their OS and architecture.

Upgrade later with:

```sh
gh extension upgrade pr-graph
```

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
--trace-otel[=URL] send gh command traces via OTLP/HTTP
```

`--trace-otel` exports traces to `http://localhost:4318/v1/traces` by default.
Use an explicit collector URL when needed, for example
`--trace-otel http://localhost:4318` or `--trace-otel=http://localhost:4318`
(the `/v1/traces` path is added when the
URL has no path). The graph and Included PR API requests are root spans with
child spans for PR search, stacked PR discovery, commit inspection, and every
`gh api graphql` command. Command spans include the executed arguments, exit
code, duration, and error status. Trace delivery is asynchronous, batched, and
best-effort, so an unavailable collector does not interrupt the application.

### Inspect traces locally

Start the included Jaeger collector and UI:

```sh
docker compose up -d
```

Run the extension with tracing enabled, then load or refresh the PR graph:

```sh
gh pr-graph --trace-otel
```

Open [http://localhost:16686](http://localhost:16686), select the
`gh-pr-graph` service, and click **Find Traces**. `GET /api/v1/graph` covers PR
search, stacked PR discovery, and commit inspection for Included PR candidates;
the subsequent `POST /api/v1/included` covers the batched candidate detail query.

Stop the local collector when finished:

```sh
docker compose down
```

For local extension development:

```sh
make build
gh extension install .
gh pr-graph
```

The server only listens on `127.0.0.1`. Press Ctrl-C in the terminal to stop it.

## Releasing

CI runs tests, vet, JavaScript syntax checking, and a build on pushes to `main` and pull requests. To publish a version, push a semantic version tag:

```sh
git tag v0.1.0
git push origin v0.1.0
```

The release workflow cross-compiles supported binaries, creates the GitHub Release, uploads correctly named assets, and generates build provenance attestations. The workflow needs the repository's default `GITHUB_TOKEN` with the permissions declared in the workflow; no additional secret is required.

For discoverability, set the repository topic `gh-extension` after publishing it.

## License

MIT. See [LICENSE](LICENSE).

## Status

This is an initial implementation based on [DESIGN.md](DESIGN.md). Search pagination beyond the first 100 results and richer team-review aggregation remain to be implemented.
