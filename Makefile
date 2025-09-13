# AWS SLURM Burst Advisor Makefile

# Variables
BINARY_NAME=aws-slurm-burst-advisor
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT=$(shell git rev-parse HEAD 2>/dev/null || echo "unknown")

# Go build flags
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.GitCommit=$(GIT_COMMIT)"
GOFLAGS=-mod=readonly

# Directories
BUILD_DIR=build
DIST_DIR=dist
CONFIG_DIR=configs

# Default target
.PHONY: all
all: clean build

# Build the binary
.PHONY: build
build:
	@echo "Building $(BINARY_NAME) $(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	go build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/aws-slurm-burst-advisor

# Build for multiple platforms
.PHONY: build-all
build-all: clean
	@echo "Building for multiple platforms..."
	@mkdir -p $(DIST_DIR)

	# Linux AMD64
	GOOS=linux GOARCH=amd64 go build $(GOFLAGS) $(LDFLAGS) \
		-o $(DIST_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/aws-slurm-burst-advisor

	# Linux ARM64 (for Graviton instances)
	GOOS=linux GOARCH=arm64 go build $(GOFLAGS) $(LDFLAGS) \
		-o $(DIST_DIR)/$(BINARY_NAME)-linux-arm64 ./cmd/aws-slurm-burst-advisor

	# macOS AMD64 (for development)
	GOOS=darwin GOARCH=amd64 go build $(GOFLAGS) $(LDFLAGS) \
		-o $(DIST_DIR)/$(BINARY_NAME)-darwin-amd64 ./cmd/aws-slurm-burst-advisor

	# macOS ARM64 (Apple Silicon)
	GOOS=darwin GOARCH=arm64 go build $(GOFLAGS) $(LDFLAGS) \
		-o $(DIST_DIR)/$(BINARY_NAME)-darwin-arm64 ./cmd/aws-slurm-burst-advisor

# Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR) $(DIST_DIR)

# Run tests
.PHONY: test
test:
	@echo "Running tests..."
	go test -v ./...

# Run tests with coverage
.PHONY: test-coverage
test-coverage:
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Lint code
.PHONY: lint
lint:
	@echo "Running linter..."
	golangci-lint run ./...

# Format code
.PHONY: fmt
fmt:
	@echo "Formatting code..."
	gofumpt -l -w .
	goimports -w .

# Vet code
.PHONY: vet
vet:
	@echo "Running go vet..."
	go vet ./...

# Security check
.PHONY: security
security:
	@echo "Running security analysis..."
	gosec ./...

# Check code quality
.PHONY: check
check: fmt vet lint security test

# Quality gate for CI/CD
.PHONY: quality-gate
quality-gate: test-coverage lint vet security
	@echo "Running quality gate checks..."
	@if [ ! -f coverage.out ]; then echo "Coverage file not found"; exit 1; fi
	@go tool cover -func=coverage.out | grep total | awk '{print $$3}' | sed 's/%//' | \
		awk '{if ($$1 < 80) {print "Coverage " $$1 "% is below 80% threshold"; exit 1} else {print "Coverage " $$1 "% meets threshold"}}'

# Install binary to system
.PHONY: install
install: build
	@echo "Installing $(BINARY_NAME) to /usr/local/bin..."
	sudo cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/
	sudo chmod +x /usr/local/bin/$(BINARY_NAME)

# Install binary system-wide with configuration
.PHONY: install-system
install-system: build config
	@echo "Installing $(BINARY_NAME) system-wide..."
	sudo cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/
	sudo chmod +x /usr/local/bin/$(BINARY_NAME)
	sudo mkdir -p /etc/slurm-burst-advisor
	sudo cp $(CONFIG_DIR)/*.yaml /etc/slurm-burst-advisor/ 2>/dev/null || true

# Create example configuration files
.PHONY: config
config:
	@echo "Creating example configuration files..."
	@mkdir -p $(CONFIG_DIR)
	@cp configs/config.example.yaml $(CONFIG_DIR)/config.yaml 2>/dev/null || \
		echo "# Example configuration - customize for your environment" > $(CONFIG_DIR)/config.yaml
	@cp configs/local-costs.example.yaml $(CONFIG_DIR)/local-costs.yaml 2>/dev/null || \
		echo "# Local costs configuration - adjust for your cluster" > $(CONFIG_DIR)/local-costs.yaml

# Run development server with hot reload
.PHONY: dev
dev:
	@echo "Starting development mode..."
	go run ./cmd/aws-slurm-burst-advisor --verbose

# Set up development environment
.PHONY: dev-setup
dev-setup:
	@echo "Setting up development environment..."
	go mod tidy
	go mod download
	@echo "Installing development tools..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Validate configuration
.PHONY: validate-config
validate-config:
	@echo "Validating configuration..."
	$(BUILD_DIR)/$(BINARY_NAME) --config=$(CONFIG_DIR)/config.yaml --help >/dev/null

# Create release package
.PHONY: package
package: build-all
	@echo "Creating release packages..."
	@mkdir -p $(DIST_DIR)/packages

	# Linux AMD64
	tar -czf $(DIST_DIR)/packages/$(BINARY_NAME)-$(VERSION)-linux-amd64.tar.gz \
		-C $(DIST_DIR) $(BINARY_NAME)-linux-amd64 \
		-C .. configs README.md LICENSE

	# Linux ARM64
	tar -czf $(DIST_DIR)/packages/$(BINARY_NAME)-$(VERSION)-linux-arm64.tar.gz \
		-C $(DIST_DIR) $(BINARY_NAME)-linux-arm64 \
		-C .. configs README.md LICENSE

	# macOS AMD64
	tar -czf $(DIST_DIR)/packages/$(BINARY_NAME)-$(VERSION)-darwin-amd64.tar.gz \
		-C $(DIST_DIR) $(BINARY_NAME)-darwin-amd64 \
		-C .. configs README.md LICENSE

	# macOS ARM64
	tar -czf $(DIST_DIR)/packages/$(BINARY_NAME)-$(VERSION)-darwin-arm64.tar.gz \
		-C $(DIST_DIR) $(BINARY_NAME)-darwin-arm64 \
		-C .. configs README.md LICENSE

# Help target
.PHONY: help
help:
	@echo "AWS SLURM Burst Advisor Build System"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  build          Build the binary for current platform"
	@echo "  build-all      Build for all supported platforms"
	@echo "  clean          Clean build artifacts"
	@echo "  test           Run tests"
	@echo "  test-coverage  Run tests with coverage report"
	@echo "  lint           Run code linter"
	@echo "  fmt            Format code"
	@echo "  check          Run format, lint, and test"
	@echo "  install        Install binary to /usr/local/bin"
	@echo "  install-system Install system-wide with config"
	@echo "  config         Create example configuration files"
	@echo "  dev            Run in development mode"
	@echo "  dev-setup      Set up development environment"
	@echo "  validate-config Validate configuration files"
	@echo "  package        Create release packages"
	@echo "  help           Show this help message"