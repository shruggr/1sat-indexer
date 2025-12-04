#!/bin/bash
set -e

echo "Generating Swagger documentation..."
swag init -g server/server.go -o docs

echo "✓ Swagger documentation generated successfully"
echo "  - docs/swagger.json"
echo "  - docs/swagger.yaml"
echo "  - docs/docs.go"
