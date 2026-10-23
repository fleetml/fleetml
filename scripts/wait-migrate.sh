#!/usr/bin/env bash
# Waits for the dev stack's one-shot `migrate` service and exits with its exit
# code. `docker compose wait migrate` alone fails with "no containers" when the
# migrations finished before it was called; `docker wait` does not. The
# project is the dev stack's (`fleetml`) unless COMPOSE_PROJECT_NAME names
# another, as `make docker-up` does.
set -euo pipefail
cd "$(dirname "$0")/.."
PROJECT=${COMPOSE_PROJECT_NAME:-fleetml}

id=$(docker compose -p "$PROJECT" -f deploy/docker-compose.yml ps -aq migrate)
[ -n "$id" ] || { echo "no migrate container: start the dev stack first (make docker-up)" >&2; exit 1; }
code=$(docker wait "$id")
if [ "$code" != 0 ]; then
  docker compose -p "$PROJECT" -f deploy/docker-compose.yml logs migrate >&2
  exit "$code"
fi
