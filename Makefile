.PHONY: build build-legacy test bench clean install

# Default target
all: build

# Build the new CLI tool
build:
	go build -o bin/evrtools ./cmd/evrtools

# Build legacy CLI (deprecated)
build-legacy:
	go build -o bin/evrFileTools ./main.go

# Run all tests
test:
	go test -v ./pkg/...

# Run benchmarks
bench:
	go test -bench=. -benchmem -benchtime=1s ./pkg/... | tee benchmark_results.log

# Run benchmarks with comparison
bench-compare:
	go test -bench=. -benchmem -count=5 ./pkg/... | tee benchmark_new.log

# Clean build artifacts
clean:
	rm -rf bin/
	rm -f benchmark_results.log benchmark_new.log

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