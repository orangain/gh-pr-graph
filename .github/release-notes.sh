#!/usr/bin/env bash
set -euo pipefail

version="${1:?release version is required}"
changelog="${2:-CHANGELOG.md}"

awk -v version="$version" '
  $0 == "## [" version "]" || index($0, "## [" version "] - ") == 1 {
    found = 1
    next
  }
  found && /^## \[/ {
    exit
  }
  found && /^\[[^]]+\]:[[:space:]]/ {
    exit
  }
  found {
    notes = notes $0 ORS
    if ($0 !~ /^[[:space:]]*$/) {
      has_content = 1
    }
  }
  END {
    if (!found || !has_content) {
      print "No release notes found for " version > "/dev/stderr"
      exit 1
    }
    printf "%s", notes
  }
' "$changelog"
