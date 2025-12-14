#!/bin/bash
set -e

SWAG="$(go env GOPATH)/bin/swag"

# Check if swag is installed, install if not
if [ ! -f "$SWAG" ]; then
    echo "swag not found, installing..."
    go install github.com/swaggo/swag/cmd/swag@latest
fi

echo "Generating Swagger documentation..."
$SWAG init -g server/server.go -o docs --parseDependencyLevel 3 --parseGoList

echo "✓ Swagger documentation generated successfully"
echo "  - docs/swagger.json"
echo "  - docs/swagger.yaml"
echo "  - docs/docs.go"
