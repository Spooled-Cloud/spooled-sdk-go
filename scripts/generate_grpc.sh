#!/bin/bash
# Generate Go gRPC stubs from spooled.proto
# Requires: go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
#           go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
#           protoc installed

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SDK_ROOT="$(dirname "$SCRIPT_DIR")"
PROTO_FILE="${SDK_ROOT}/../spooled-backend/proto/spooled.proto"
OUTPUT_DIR="${SDK_ROOT}/spooled/grpc/pb"

if [ ! -f "$PROTO_FILE" ]; then
    echo "Error: Proto file not found at $PROTO_FILE"
    exit 1
fi

# Ensure output directory exists
mkdir -p "$OUTPUT_DIR"

echo "Generating gRPC stubs from proto..."

# Copy proto file to local directory and add go_package option
{
    echo 'syntax = "proto3";'
    echo ''
    echo 'option go_package = "github.com/spooled-cloud/spooled-sdk-go/spooled/grpc/pb";'
    echo ''
    # Skip the original syntax line and add the rest
    tail -n +2 "$PROTO_FILE"
} > "$OUTPUT_DIR/spooled.proto"

# Generate Go code
protoc \
    --go_out="$OUTPUT_DIR" \
    --go_opt=paths=source_relative \
    --go-grpc_out="$OUTPUT_DIR" \
    --go-grpc_opt=paths=source_relative \
    -I "$OUTPUT_DIR" \
    "$OUTPUT_DIR/spooled.proto"

echo "Generated gRPC stubs in $OUTPUT_DIR"
echo "Done!"
