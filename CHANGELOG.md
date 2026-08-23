# Changelog

Notable user-facing changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- Back off automatic refreshes after failures instead of retrying every five minutes.
- Return empty arrays instead of `null` when no pull requests match a search.

## [0.14.5] - 2026-08-14

### Changed

- Unified the colors of the pending-review and re-review attention icons.

[Unreleased]: https://github.com/orangain/gh-pr-graph/compare/v0.14.5...HEAD
[0.14.5]: https://github.com/orangain/gh-pr-graph/compare/v0.14.4...v0.14.5
