# Changelog

Notable user-facing changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.14.6] - 2026-08-23

### Fixed

- Back off automatic refreshes after failures instead of retrying every five minutes.
  Reported by [@syamichin](https://github.com/syamichin) in
  [#4](https://github.com/orangain/gh-pr-graph/issues/4) and fixed in
  [#6](https://github.com/orangain/gh-pr-graph/pull/6).
- Return empty arrays instead of `null` when no pull requests match a search.
  Reported by [@syamichin](https://github.com/syamichin) in
  [#3](https://github.com/orangain/gh-pr-graph/issues/3) and fixed in
  [#5](https://github.com/orangain/gh-pr-graph/pull/5).

## [0.14.5] - 2026-08-14

### Changed

- Unified the colors of the pending-review and re-review attention icons.

[Unreleased]: https://github.com/orangain/gh-pr-graph/compare/v0.14.6...HEAD
[0.14.6]: https://github.com/orangain/gh-pr-graph/compare/v0.14.5...v0.14.6
[0.14.5]: https://github.com/orangain/gh-pr-graph/compare/v0.14.4...v0.14.5
