#!/bin/bash
set -e

# Check if swag is installed, install if not
if ! command -v swag &> /dev/null; then
    echo "swag not found, installing..."
    go install github.com/swaggo/swag/cmd/swag@latest
fi

echo "Generating Swagger documentation..."
swag init -g server/server.go -o docs

echo "✓ Swagger documentation generated successfully"
echo "  - docs/swagger.json"
echo "  - docs/swagger.yaml"
echo "  - docs/docs.go"
