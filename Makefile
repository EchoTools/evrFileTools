.PHONY: build test bench clean install fmt lint check

# Default target
all: build

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