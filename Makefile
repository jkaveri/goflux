.PHONY: all test clean mocks

# Default target
all: test

# Run tests
test:
	go test -v -race ./...
.PHONY: test

# Clean generated files
clean:
	go clean
.PHONY: clean

# Generate mocks
mocks:
	./scripts/generate-mocks.sh
.PHONY: mocks

# Install development tools
tools:
	go mod download
	go mod tidy
.PHONY: tools

# Run linter
lint:
	./scripts/lint.sh
.PHONY: lint
# Build the project
build:
	go build -v ./...
.PHONY: build

# Update dependencies
deps:
	go mod tidy
	go mod verify
.PHONY: deps