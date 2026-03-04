.PHONY: build build-all clean test install

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BINARY_NAME = copyman
BUILD_DIR = bin
SRC_DIR = src

# Default build for current platform
build:
	mkdir -p $(BUILD_DIR)
	cd $(SRC_DIR) && go build -ldflags="-s -w -X main.version=$(VERSION)" -o ../$(BUILD_DIR)/$(BINARY_NAME) .

# Build for all platforms
build-all:
	mkdir -p $(BUILD_DIR)
	# Linux AMD64
	cd $(SRC_DIR) && GOOS=linux GOARCH=amd64 go build -ldflags="-s -w -X main.version=$(VERSION)" -o ../$(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 .
	# Linux ARM64
	cd $(SRC_DIR) && GOOS=linux GOARCH=arm64 go build -ldflags="-s -w -X main.version=$(VERSION)" -o ../$(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 .
	# macOS AMD64
	cd $(SRC_DIR) && GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w -X main.version=$(VERSION)" -o ../$(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 .
	# macOS ARM64 (M1/M2)
	cd $(SRC_DIR) && GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w -X main.version=$(VERSION)" -o ../$(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 .
	# Windows AMD64
	cd $(SRC_DIR) && GOOS=windows GOARCH=amd64 go build -ldflags="-s -w -X main.version=$(VERSION)" -o ../$(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe .
	@echo "Build complete! Binaries in $(BUILD_DIR)/"
	@ls -lh $(BUILD_DIR)/

# Clean build artifacts
clean:
	rm -rf $(BUILD_DIR)

# Run tests
test:
	cd $(SRC_DIR) && go test -v ./...

# Install locally (requires Go)
install:
	cd $(SRC_DIR) && go install -ldflags="-s -w -X main.version=$(VERSION)" .

# Development build with debug info
dev:
	mkdir -p $(BUILD_DIR)
	cd $(SRC_DIR) && go build -o ../$(BUILD_DIR)/$(BINARY_NAME) .

# Check for updates to dependencies
update:
	cd $(SRC_DIR) && go get -u ./... && go mod tidy

# Show help
help:
	@echo "Available targets:"
	@echo "  build      - Build for current platform"
	@echo "  build-all  - Build for all platforms (Linux, macOS, Windows)"
	@echo "  clean      - Remove build artifacts"
	@echo "  test       - Run tests"
	@echo "  install    - Install to \$$GOPATH/bin"
	@echo "  dev        - Development build (no optimizations)"
	@echo "  update     - Update Go dependencies"
