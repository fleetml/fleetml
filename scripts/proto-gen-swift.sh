#!/usr/bin/env bash
# Regenerates the iOS SDK's protobuf and gRPC Swift code from proto/.
# The protoc plugins are built from the exact grpc-swift / swift-protobuf
# versions the package resolves, so generated code always matches the runtime.
# Needs protoc and a Swift toolchain (Xcode).
set -euo pipefail
cd "$(dirname "$0")/.." # the package manifest is at the repository root

swift package resolve >/dev/null
for pkg in swift-protobuf:protoc-gen-swift grpc-swift:protoc-gen-grpc-swift; do
  swift build -c release --package-path ".build/checkouts/${pkg%%:*}" --product "${pkg##*:}" >/dev/null
done
PLUGINS=$(pwd)/.build/checkouts/swift-protobuf/.build/release
GRPC_PLUGIN=$(pwd)/.build/checkouts/grpc-swift/.build/release/protoc-gen-grpc-swift

OUT=ios/FleetMLSDK/Sources/FleetMLSDK/Generated
rm -rf "$OUT" && mkdir -p "$OUT"
protoc --proto_path=proto \
  --plugin=protoc-gen-swift="$PLUGINS/protoc-gen-swift" --swift_out="$OUT" --swift_opt=Visibility=Internal \
  --plugin=protoc-gen-grpc-swift="$GRPC_PLUGIN" --grpc-swift_out="$OUT" \
  --grpc-swift_opt=Visibility=Internal,Server=false,Client=true \
  proto/fleetml/v1/device.proto proto/fleetml/v1/telemetry.proto
echo "Swift protobuf generation complete."
