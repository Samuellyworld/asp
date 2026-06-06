#!/usr/bin/env bash
set -euo pipefail

# Runs the Go lint job locally with the same golangci-lint version used by CI.
# By default it reports only issues introduced since origin/main, matching the
# intent of golangci-lint-action's only-new-issues mode.

PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
BASE_REF="${BASE_REF:-origin/main}"
GOLANGCI_LINT_VERSION="${GOLANGCI_LINT_VERSION:-v2.12.2}"

cd "$PROJECT_DIR"

NEW_ISSUES_ARGS=()
if git rev-parse --verify --quiet "$BASE_REF" >/dev/null; then
  NEW_ISSUES_ARGS=(--new-from-rev="$BASE_REF")
else
  echo "warning: $BASE_REF not found; running full lint instead" >&2
fi

cd go-bot
go run "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${GOLANGCI_LINT_VERSION}" \
  run \
  --timeout=5m \
  "${NEW_ISSUES_ARGS[@]}"
