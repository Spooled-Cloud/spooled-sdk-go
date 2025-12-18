#!/bin/bash
# Generate Go types and client from OpenAPI spec
# Requires: go install github.com/deepmap/oapi-codegen/v2/cmd/oapi-codegen@latest

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SDK_ROOT="$(dirname "$SCRIPT_DIR")"
OPENAPI_SPEC="${SDK_ROOT}/../spooled-backend/docs/openapi.yaml"
OUTPUT_DIR="${SDK_ROOT}/internal/openapi"

if [ ! -f "$OPENAPI_SPEC" ]; then
    echo "Error: OpenAPI spec not found at $OPENAPI_SPEC"
    exit 1
fi

# Ensure output directory exists
mkdir -p "$OUTPUT_DIR"

echo "Generating Go types from OpenAPI spec..."

# Generate types only (we'll write our own client wrapper)
go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest \
    -generate types \
    -package openapi \
    -o "${OUTPUT_DIR}/types.gen.go" \
    "$OPENAPI_SPEC"

# Fix duplicate BearerAuthScopes declaration (caused by both BearerAuth and bearerAuth in spec)
if grep -q 'BearerAuthScopes.*bearerAuth' "${OUTPUT_DIR}/types.gen.go"; then
    echo "Fixing duplicate BearerAuthScopes declaration..."
    if [[ "$OSTYPE" == "darwin"* ]]; then
        sed -i '' '/BearerAuthScopes.*bearerAuth/d' "${OUTPUT_DIR}/types.gen.go"
    else
        sed -i '/BearerAuthScopes.*bearerAuth/d' "${OUTPUT_DIR}/types.gen.go"
    fi
fi

echo "Generated ${OUTPUT_DIR}/types.gen.go"
echo "Done!"
