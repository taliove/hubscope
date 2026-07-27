#!/bin/sh
# PostToolUse hook — auto-format web/src files after Edit/Write, mirroring
# format-go.sh. Formats only if prettier is installed in web/node_modules;
# otherwise silent skip. web/ does not currently ship prettier as a
# devDependency — install it (and add a format script) to enable auto-format.
# Reads the tool-use payload from stdin; formats only files inside the repo.
set -u

file=$(sed -n 's/.*"file_path"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)
project_dir=${CLAUDE_PROJECT_DIR:-}
case "$file" in
  "$project_dir"/web/src/*.ts|"$project_dir"/web/src/*.tsx|"$project_dir"/web/src/*.vue|"$project_dir"/web/src/*.js|"$project_dir"/web/src/*.jsx|"$project_dir"/web/src/*.css|"$project_dir"/web/src/*.scss)
    prettier="$project_dir/web/node_modules/.bin/prettier"
    [ -n "$project_dir" ] && [ -x "$prettier" ] && "$prettier" --write "$file" >/dev/null 2>&1 || true
    ;;
esac
exit 0
