#!/usr/bin/env bash
set -euo pipefail

PROTO_DIR="proto"
GEN_DIR="gen"

rm -rf "${GEN_DIR}"
mkdir -p "${GEN_DIR}"

protoc \
  --proto_path="${PROTO_DIR}" \
  --go_out="${GEN_DIR}" \
  --go_opt=paths=source_relative \
  --go-grpc_out="${GEN_DIR}" \
  --go-grpc_opt=paths=source_relative \
  "${PROTO_DIR}"/fleetml/v1/*.proto

echo "Protobuf generation complete."
