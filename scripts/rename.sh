#!/usr/bin/env bash
# Renames the Go module and the project name in every tracked text file:
# imports, Docker names, database roles, cron jobs and titles.
set -euo pipefail

usage="usage: scripts/rename.sh <go module path> <name>  (or make rename in backend/)"
module=${1:?$usage}
name=${2:?$usage}
old_name=dataset

# The name becomes Postgres roles (<name>_app), cron jobs and Docker names.
if [[ ! $name =~ ^[a-z][a-z0-9_]{1,30}$ ]]; then
  echo "name must match ^[a-z][a-z0-9_]{1,30}\$" >&2
  exit 1
fi
if [[ ! $module =~ ^[A-Za-z0-9._~/-]+$ ]]; then
  echo "module path has characters Go does not accept" >&2
  exit 1
fi
cd "$(git rev-parse --show-toplevel)"
if [[ -n $(git status --porcelain) ]]; then
  echo "commit or stash your changes first: the rename touches most files" >&2
  exit 1
fi

old_module=$(awk '$1 == "module" { print $2 }' backend/go.mod)
quote() { printf '%s' "$1" | sed 's/[][\.*^$#/]/\\&/g'; }

# The module path goes first: it contains the old name.
git ls-files -z | xargs -0 grep -lIZ -i -e "$old_name" -e "$old_module" |
  xargs -0 -r sed -i \
    -e "s#$(quote "$old_module")#$(quote "$module")#g" \
    -e "s#$old_name#$name#g" \
    -e "s#${old_name^}#${name^}#g"

# A new module path can sort differently among the imports.
git ls-files -z '*.go' | xargs -0 gofmt -w

cat <<EOF
Renamed $old_module ($old_name) to $module ($name). Next:
  cd backend
  make init        # config/.env still names the old roles; regenerate it
  make swagger     # regenerates the Swagger docs
  go build ./... && go test ./...
EOF
