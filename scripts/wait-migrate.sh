#!/usr/bin/env bash
# Waits for the dev stack's one-shot `migrate` service and exits with its exit
# code. `docker compose wait migrate` alone fails with "no containers" when the
# migrations finished before it was called; `docker wait` does not.
set -euo pipefail
cd "$(dirname "$0")/.."

id=$(docker compose -f deploy/docker-compose.yml ps -aq migrate)
[ -n "$id" ] || { echo "no migrate container: start the dev stack first (make docker-up)" >&2; exit 1; }
code=$(docker wait "$id")
if [ "$code" != 0 ]; then
  docker compose -f deploy/docker-compose.yml logs migrate >&2
  exit "$code"
fi
