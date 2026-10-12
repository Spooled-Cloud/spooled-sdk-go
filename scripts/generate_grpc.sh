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

# The one deliberate difference from the backend proto. The backend declares
# FailRequest.retry `optional bool`; generated Go would make
# pb.FailRequest.Retry a *bool and break code that sets or reads it as a bool.
# The SDK keeps a plain bool, and grpc.SetNoRetry (used by Client.Fail for
# NoRetry) sends an explicit false as an unknown field 4 = 0, which the backend
# reads as "dead-letter now". TestProto_MatchesBackendProto knows this line.
BACKEND_RETRY='    optional bool retry = 4;'
SDK_RETRY='    bool retry = 4; // SDK copy: a plain bool, so false is not sent and means retry; grpc.SetNoRetry sends false.'

# Copy proto file to local directory and add go_package option
{
    echo 'syntax = "proto3";'
    echo ''
    echo 'option go_package = "github.com/spooled-cloud/spooled-sdk-go/spooled/grpc/pb";'
    echo ''
    # Skip the original syntax line and add the rest
    tail -n +2 "$PROTO_FILE" | awk -v from="$BACKEND_RETRY" -v to="$SDK_RETRY" '$0 == from { print to; next } { print }'
} > "$OUTPUT_DIR/spooled.proto"

if [ "$(grep -cxF -- "$SDK_RETRY" "$OUTPUT_DIR/spooled.proto")" != "1" ]; then
    echo "Error: FailRequest.retry in $PROTO_FILE is no longer '$BACKEND_RETRY'; review the SDK override above" >&2
    exit 1
fi

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
