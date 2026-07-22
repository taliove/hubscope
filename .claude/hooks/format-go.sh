#!/bin/sh
# PostToolUse hook — auto-format Go files after Edit/Write so `make lint`
# (gofmt check in the pre-commit gate) never fails on style alone.
# Reads the tool-use payload from stdin; formats only files inside the repo.
set -u

file=$(sed -n 's/.*"file_path"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)
project_dir=${CLAUDE_PROJECT_DIR:-}
case "$file" in
  "$project_dir"/*.go)
    [ -n "$project_dir" ] && gofmt -w "$file" || true
    ;;
esac
exit 0
