#!/usr/bin/env bash
set -euo pipefail

DATABASE_URL="${DATABASE_URL:-postgres://fleetml:fleetml_dev@localhost:15432/fleetml?sslmode=disable}"
MIGRATIONS_PATH="internal/db/migrations"

DIRECTION="${1:-up}"

migrate -database "${DATABASE_URL}" -path "${MIGRATIONS_PATH}" "${DIRECTION}"
