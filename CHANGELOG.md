# Changelog

Notable user-facing changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.15.1] - 2026-09-01

### Fixed

- Keep automatic refresh active for up to 30 minutes while the browser tab is hidden, then refresh stale data when the tab becomes visible again in [#9](https://github.com/orangain/gh-pr-graph/pull/9).

## [0.15.0] - 2026-08-28

### Added

- Copy a pull request's number, title, and URL as plain text from its card. Contributed by [@shogidemo](https://github.com/shogidemo) in [#2](https://github.com/orangain/gh-pr-graph/pull/2).

## [0.14.6] - 2026-08-23

### Fixed

- Back off automatic refreshes after failures instead of retrying every five minutes. Reported by [@syamichin](https://github.com/syamichin) in [#4](https://github.com/orangain/gh-pr-graph/issues/4) and fixed in [#6](https://github.com/orangain/gh-pr-graph/pull/6).
- Return empty arrays instead of `null` when no pull requests match a search. Reported by [@syamichin](https://github.com/syamichin) in [#3](https://github.com/orangain/gh-pr-graph/issues/3) and fixed in [#5](https://github.com/orangain/gh-pr-graph/pull/5).

## [0.14.5] - 2026-08-14

### Changed

- Unified the colors of the pending-review and re-review attention icons.

[Unreleased]: https://github.com/orangain/gh-pr-graph/compare/v0.15.1...HEAD
[0.15.1]: https://github.com/orangain/gh-pr-graph/compare/v0.15.0...v0.15.1
[0.15.0]: https://github.com/orangain/gh-pr-graph/compare/v0.14.6...v0.15.0
[0.14.6]: https://github.com/orangain/gh-pr-graph/compare/v0.14.5...v0.14.6
[0.14.5]: https://github.com/orangain/gh-pr-graph/compare/v0.14.4...v0.14.5
