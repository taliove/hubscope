#!/bin/sh
# Claude Code PreToolUse hook (matcher: Bash).
# Blocks `git commit --no-verify` / `-n`, which would bypass the
# .githooks/pre-commit gate. Reads the tool-call JSON from stdin.
input=$(cat)
case "$input" in
  *--no-verify*)
    echo "Blocked: --no-verify bypasses the pre-commit gate (make test + secret scan). Commit without it." >&2
    exit 2
    ;;
esac
exit 0
