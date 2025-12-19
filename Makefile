.PHONY: help build test bench clean install fmt lint check

# Default target - show help
.DEFAULT_GOAL := help

# Show available targets
help:
	@echo "evrFileTools - EVR package/manifest tool"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  build         Build the CLI tool to bin/evrtools"
	@echo "  test          Run all tests"
	@echo "  bench         Run benchmarks"
	@echo "  bench-compare Run benchmarks with multiple iterations"
	@echo "  clean         Remove build artifacts"
	@echo "  install       Install CLI tool via go install"
	@echo "  fmt           Format code"
	@echo "  lint          Run go vet"
	@echo "  check         Run fmt, lint, and test"

# Build the CLI tool
build:
	go build -o bin/evrtools ./cmd/evrtools

# Run all tests
test:
	go test -v ./pkg/...

# Run benchmarks
bench:
	go test -bench=. -benchmem -benchtime=1s ./pkg/...

# Run benchmarks with comparison
bench-compare:
	go test -bench=. -benchmem -count=5 ./pkg/...

# Clean build artifacts
clean:
	rm -rf bin/

# Install the CLI tool
install:
	go install ./cmd/evrtools

# Format code
fmt:
	go fmt ./...

# Lint code
lint:
	go vet ./...

# Check for common issues
check: fmt lint test