#!/bin/bash
set -e

# Copyman CLI Installer
# Usage: curl -sSL https://raw.githubusercontent.com/mathysin/copyman-cli/main/install.sh | bash

REPO="mathysin/copyman-cli"
BINARY_NAME="copyman"
INSTALL_DIR="/usr/local/bin"

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$OS" in
    linux)
        case "$ARCH" in
            x86_64) SUFFIX="linux-amd64" ;;
            aarch64|arm64) SUFFIX="linux-arm64" ;;
            *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
        esac
        ;;
    darwin)
        case "$ARCH" in
            x86_64) SUFFIX="darwin-amd64" ;;
            arm64) SUFFIX="darwin-arm64" ;;
            *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
        esac
        ;;
    *)
        echo "Unsupported OS: $OS"
        exit 1
        ;;
esac

# Get latest release
API_URL="https://api.github.com/repos/$REPO/releases/latest"
echo "Fetching latest release..."
DOWNLOAD_URL=$(curl -s "$API_URL" | grep -o '"browser_download_url": "[^"]*copyman-'"$SUFFIX"'"' | cut -d'"' -f4)

if [ -z "$DOWNLOAD_URL" ]; then
    echo "Error: Could not find binary for $SUFFIX"
    exit 1
fi

# Download
echo "Downloading copyman-$SUFFIX..."
TMP_DIR=$(mktemp -d)
curl -sSL "$DOWNLOAD_URL" -o "$TMP_DIR/$BINARY_NAME"

# Verify checksum
echo "Verifying checksum..."
curl -sSL "${DOWNLOAD_URL%/*}/checksums.txt" -o "$TMP_DIR/checksums.txt"
cd "$TMP_DIR"
if ! sha256sum -c checksums.txt --ignore-missing 2>/dev/null | grep -q "OK"; then
    echo "Warning: Checksum verification failed or not available"
fi

# Install
chmod +x "$TMP_DIR/$BINARY_NAME"
echo "Installing to $INSTALL_DIR (may require sudo)..."
if [ -w "$INSTALL_DIR" ]; then
    mv "$TMP_DIR/$BINARY_NAME" "$INSTALL_DIR/"
else
    sudo mv "$TMP_DIR/$BINARY_NAME" "$INSTALL_DIR/"
fi

# Cleanup
rm -rf "$TMP_DIR"

echo ""
echo "✅ Copyman CLI installed successfully!"
echo ""
echo "Run 'copyman --help' to get started"
echo "Visit https://copyman.fr to use the web interface"
