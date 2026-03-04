# Copyman CLI

A CLI tool for [copyman.fr](https://copyman.fr) - fully non-interactive, designed for AI agents and automation.

## Installation

```bash
go build -o copyman ./src/main.go
```

## Usage

### Login to a session

```bash
copyman login --session-id <your-session-id> --password <your-password>
```

### Logout

```bash
copyman logout
```

### Push content

Push text:
```bash
copyman push text "Your note content here"
```

Push file(s):
```bash
copyman push file /path/to/file.png
copyman push file file1.txt file2.txt file3.pdf
```

### List content

Human-readable format:
```bash
copyman list
```

JSON format (for AI agents/automation):
```bash
copyman list --json
```

Example output:
```json
[
  {
    "id": "abc123",
    "type": "note",
    "content": "Hello world",
    "createdAt": 1708400000000,
    "updatedAt": 1708400000000
  },
  {
    "id": "def456",
    "type": "attachment",
    "attachmentUrl": "https://...",
    "attachmentPath": "document.pdf",
    "createdAt": 1708400000000,
    "updatedAt": 1708400000000
  }
]
```

### Get content by ID

For text notes, prints content to stdout:
```bash
copyman get abc123
```

For files, downloads to specified path:
```bash
copyman get def456 --output ./downloaded-file.pdf
```

### Delete content

```bash
copyman delete abc123
```

## AI Agent Integration

All commands are non-interactive and return structured output:

1. Use `--json` flag with `list` for machine-readable output
2. Use `get` with `--output` flag to download files to specific paths
3. All errors are written to stderr with non-zero exit codes
