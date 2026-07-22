#!/usr/bin/env bash
set -Eeuo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$repo_root"
source ci/gitlab/bootstrap-toolchain.sh

(
  cd web
  bun install --frozen-lockfile
)
(
  cd web/default
  bun run build:check
)

install -d web/classic/dist
printf '<!doctype html><title>test fixture</title>\n' > web/classic/dist/index.html
node --test scripts/glimo-b2b-policy-audit.test.mjs
go test ./...
