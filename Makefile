.PHONY: tools docs build test

# Install development tools
tools:
	@echo "Installing development tools..."
	@go install github.com/swaggo/swag/cmd/swag@latest
	@echo "✓ Tools installed"

# Generate API documentation
docs:
	@./build-docs.sh

# Build the server
build:
	@./build.sh

# Run tests
test:
	@go test ./...
