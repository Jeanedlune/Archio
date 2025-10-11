.PHONY: build test clean run fmt vet mod-tidy

# Build the application
build:
	go build -o bin/archio ./cmd/server

# Run tests
test:
	go test -v -race -cover ./...

# Run tests with coverage report
test-cover:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Format code
fmt:
	go fmt ./...

# Vet code
vet:
	go vet ./...

# Tidy modules
mod-tidy:
	go mod tidy

# Clean build artifacts
clean:
	rm -rf bin/
	rm -f coverage.out coverage.html

# Run the application
run:
	go run ./cmd/server/main.go

# Run with config
run-config:
	go run ./cmd/server/main.go -config config.yaml

# Development setup
dev-setup: mod-tidy fmt vet test build

# CI checks (same as GitHub Actions)
ci: mod-tidy fmt vet test build

# Install development tools
install-tools:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Lint code
lint:
	golangci-lint run

# Create example config
config:
	cp config.example.yaml config.yaml

# Show help
help:
	@echo "Available targets:"
	@echo "  build      - Build the application"
	@echo "  test       - Run tests"
	@echo "  test-cover - Run tests with coverage report"
	@echo "  fmt        - Format code"
	@echo "  vet        - Vet code"
	@echo "  mod-tidy   - Tidy modules"
	@echo "  clean      - Clean build artifacts"
	@echo "  run        - Run the application"
	@echo "  run-config - Run with config file"
	@echo "  dev-setup  - Development setup (tidy, fmt, vet, test, build)"
	@echo "  ci         - CI checks"
	@echo "  install-tools - Install development tools"
	@echo "  lint       - Lint code with golangci-lint"
	@echo "  config     - Create config.yaml from example"
	@echo "  help       - Show this help"