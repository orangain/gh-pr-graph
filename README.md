# gh pr-graph

`gh pr-graph` turns the pull requests that need your attention into one visual workspace. Run one command to see every open PR you authored, were assigned to, or were asked to review—across repositories—without assembling filters or checking separate GitHub pages.

As AI makes it easier to develop multiple changes in parallel, stacked pull requests are becoming larger and more common. A flat PR list hides which change depends on which. `gh pr-graph` follows base and head branches recursively and draws those relationships as a directed graph, making the review order, downstream work, and the path back to each repository immediately visible.

Color-coded ownership and compact review, CI, conflict, and draft signals help you decide what needs action next. Merged PRs included in a change remain available in collapsible context, while automatic refresh keeps the workspace current. Everything runs locally through your authenticated `gh` CLI; GitHub credentials are never passed to the browser.

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
--port 8080        override the preferred local port (8787)
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
search and stacked PR discovery, `POST /api/v1/inspect` covers commit inspection
for one containing PR, and `POST /api/v1/included` covers its candidate detail query.

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
