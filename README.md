# Copyman CLI

A command-line interface for [copyman.fr](https://copyman.fr) - share notes and files across devices with optional end-to-end encryption.

## Features

- **Share notes** - Quick text sharing across devices
- **Share files** - Upload and download files
- **End-to-End Encryption** - Optional E2EE for sensitive content
- **Cross-platform** - Works on Linux, macOS, and Windows
- **Fast** - Lightweight Go binary
- **Interactive** - Arrow-key selection with `-i` flag

## Installation

### Quick Install (Recommended)

```bash
curl -sSL https://raw.githubusercontent.com/mathysIN/copyman-cli/main/install.sh | bash
```

This installs to `/usr/local/bin/copyman`. You may need to run with `sudo` depending on your system permissions.

### Option 2: Download Binary

Download the latest release for your platform from the [Releases page](https://github.com/mathysin/copyman-cli/releases).

```bash
# Linux/macOS
chmod +x copyman-linux-amd64
sudo mv copyman-linux-amd64 /usr/local/bin/copyman

# Windows
# Add copyman-windows-amd64.exe to your PATH
```

### Option 3: Go Install

```bash
go install github.com/mathysin/copyman-cli/src@latest
```

### Option 4: Build from Source

```bash
git clone https://github.com/mathysin/copyman-cli.git
cd copyman-cli
make build
# Binary will be at bin/copyman
```

## Updating

### Self-Update

```bash
# Check for updates and update automatically
copyman update
```

### Manual Update

Download the latest release from the [Releases page](https://github.com/mathysin/copyman-cli/releases) and replace your binary.

## Quick Start

```bash
# Create a session
copyman create --session-id mysession --password secret123

# Share some text
copyman push text "Hello, World!"

# Share a file
copyman push file /path/to/file.txt

# List content
copyman list

# Get content
copyman get <content-id>
```

## Commands

### Session Management

```bash
# Create session (with optional password)
copyman create --session-id mysession --password secret123

# Create temporary session (auto-generated ID, expires in ~4h)
copyman create --temp

# Login to existing session
copyman login --session-id mysession --password secret123

# Logout
copyman logout

# Check session status
copyman status
```

### Content Management

```bash
# Push text
copyman push text "Your message here"

# Push file
copyman push file /path/to/file

# List all content
copyman list

# List as JSON
copyman list --json

# Get content by ID
copyman get <content-id>

# Download file with custom name
copyman get <file-id> --output myfile.txt

# Interactive selection (arrow keys)
copyman get -i

# Delete content
copyman delete <content-id>
```

### End-to-End Encryption

E2EE is optional and separate from session passwords:

```bash
# 1. Create session with password (access control)
copyman create --session-id secure --password mypass

# 2. Enable E2EE (content will be encrypted)
copyman encryption enable --password mypass

# 3. Push content (automatically encrypted)
copyman push text "Secret message"

# 4. Content is decrypted automatically when viewing
copyman get <content-id>
```

**Note:**
- Password = Access control (who can join)
- E2EE = Content encryption (must be enabled separately)
- Mixed content: Some encrypted, some not - all works fine
- Encrypted content shows `[ENCRYPTED]` in list view

### Update Management

```bash
# Check for updates
copyman update

# Show current version
copyman version
```

## Examples

### Basic Usage

```bash
# Quick share without password
copyman create --session-id quickshare
copyman push text "Meeting at 3pm"
copyman push file presentation.pdf
```

### Secure Sharing

```bash
# Encrypted session
copyman create --session-id secrets --password SuperSecret123
copyman encryption enable --password SuperSecret123
copyman push text "API_KEY=sk-abc123"
```

### Interactive Mode

```bash
# Use arrow keys to select and download
copyman get -i
```

## Development

```bash
# Build for current platform
make build

# Build for all platforms
make build-all

# Run tests
make test

# Install locally
make install

# Clean build artifacts
make clean
```

## License

MIT
