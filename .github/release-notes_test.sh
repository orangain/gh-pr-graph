#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

cat >"$tmp_dir/CHANGELOG.md" <<'EOF'
# Changelog

## [Unreleased]

- Next change.

## [1.2.3] - 2026-08-23

### Fixed

- Released fix.

## [1.2.2] - 2026-08-22

- Previous change.

[Unreleased]: https://example.com/compare/1.2.3...HEAD
[1.2.3]: https://example.com/compare/1.2.2...1.2.3
EOF

bash "$repo_root/.github/release-notes.sh" 1.2.3 "$tmp_dir/CHANGELOG.md" >"$tmp_dir/actual"

cat >"$tmp_dir/expected" <<'EOF'

### Fixed

- Released fix.

EOF

cmp "$tmp_dir/expected" "$tmp_dir/actual"

bash "$repo_root/.github/release-notes.sh" 1.2.2 "$tmp_dir/CHANGELOG.md" >"$tmp_dir/previous-actual"

cat >"$tmp_dir/previous-expected" <<'EOF'

- Previous change.

EOF

cmp "$tmp_dir/previous-expected" "$tmp_dir/previous-actual"

if bash "$repo_root/.github/release-notes.sh" 9.9.9 "$tmp_dir/CHANGELOG.md" >"$tmp_dir/missing" 2>/dev/null; then
  echo "missing release notes unexpectedly succeeded" >&2
  exit 1
fi
