# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Chatlog is a WeChat chat history extraction and management tool written in Go. It supports:
- Extracting chat data from local WeChat database files (Windows/macOS, WeChat 3.x/4.x)
- Decrypting WeChat databases and multimedia content (images, voice messages, wxgf format)
- Auto-decrypting with webhook callbacks for new messages
- HTTP API server for querying chat history, contacts, groups, recent sessions
- MCP (Model Context Protocol) support via Streamable HTTP
- Terminal UI (TUI) and CLI modes
- Multi-account management

## Build Commands

```bash
# Build for current platform (requires CGO)
make build

# Clean build artifacts
make clean

# Run linter
make lint

# Tidy dependencies
make tidy

# Run tests
make test

# Cross-platform build
make crossbuild

# Full pipeline: clean, lint, tidy, test, build
make all
```

**Important**: This project has CGO dependencies and requires a C compiler environment.

## Development Commands

```bash
# Install from source
go install github.com/sjzar/chatlog@latest

# Run Terminal UI
./bin/chatlog

# Get WeChat encryption keys
./bin/chatlog key

# Decrypt database
./bin/chatlog decrypt

# Start HTTP server (default: http://127.0.0.1:5030)
./bin/chatlog server

# Show version
./bin/chatlog version
```

## Architecture

### Entry Point
- `main.go`: Minimal entry point that calls `cmd/chatlog/Execute()`

### Command Layer (`cmd/chatlog/`)
- `root.go`: Root cobra command, launches TUI by default
- `cmd_key.go`: Extract WeChat encryption keys
- `cmd_decrypt.go`: Decrypt WeChat databases
- `cmd_server.go`: Start HTTP API server
- `cmd_dumpmemory.go`: Memory dump utilities
- `cmd_version.go`: Version info

### Core Application (`internal/chatlog/`)
- `app.go`: TUI application using tview framework
- `manager.go`: Core business logic manager coordinating all services
- `conf/`: Configuration management (accounts, settings, webhooks)
- `ctx/`: Application context
- `database/`: Database service layer
- `http/`: HTTP API server implementation
- `webhook/`: Webhook notification system
- `wechat/`: WeChat-specific service integration

### WeChat Layer (`internal/wechat/`)
- `wechat.go`: WeChat service interface
- `manager.go`: WeChat instance manager
- `key/`: Platform-specific key extraction (Windows/macOS)
- `decrypt/`: Database decryption logic
- `process/`: WeChat process detection and memory access
- `model/`: WeChat data models

### Database Layer (`internal/wechatdb/`)
- `wechatdb.go`: WeChat database interface
- `datasource/`: SQLite database connections
- `repository/`: Data access layer for messages, contacts, chatrooms

### MCP Integration (`internal/mcp/`)
- MCP server implementation supporting Streamable HTTP protocol
- Tools for querying chat history, contacts, groups, sessions

### UI Components (`internal/ui/`)
- `menu/`: TUI menu system
- `form/`: Form components
- `infobar/`: Status information bar
- `footer/`: Footer bar
- `help/`: Help pages
- `style/`: Platform-specific styling

### Utilities (`pkg/`)
- `config/`: Viper-based configuration
- `version/`: Version information
- `appver/`: Application version detection
- `filemonitor/`: File system monitoring
- `filecopy/`: File operations
- `util/`: Various utilities (silk audio, lz4, zstd, dat2img conversion)

## Key Technical Details

### CGO Dependencies
The project uses CGO for:
- SQLite database access (`mattn/go-sqlite3`)
- Audio encoding/decoding (`go-lame`, `go-silk`)

### Platform-Specific Code
- macOS: Requires SIP disabled to extract keys, uses plist parsing
- Windows: Different process memory access patterns
- Both platforms have separate key extraction implementations in `internal/wechat/key/`

### Database Encryption
WeChat uses SQLCipher for database encryption. The tool:
1. Extracts data_key from WeChat process memory
2. Uses key to decrypt SQLite databases
3. Supports auto-decrypt with file monitoring

### Multimedia Decryption
- Images: Encrypted with image_key, real-time decryption in HTTP responses
- Voice: SILK format converted to MP3 on-the-fly
- Video/Files: Direct access with decryption

### HTTP API
- Gin framework for HTTP server
- RESTful endpoints under `/api/v1/`
- Multimedia content served under `/image/`, `/video/`, `/voice/`, `/file/`, `/data/`
- MCP endpoint at `/mcp`

### Configuration
- Config location: `$HOME/.chatlog/chatlog.json` (Windows: `%USERPROFILE%/.chatlog/chatlog.json`)
- Stores account history, last account, webhook settings
- Uses Viper for config management

## Testing Notes
- Tests use standard Go testing: `go test ./...`
- CGO-dependent tests may need C compiler setup
- Some tests may require mock WeChat data structures

## Platform-Specific Build
Cross-compilation requires platform-specific C compilers configured in `.goreleaser.yaml`:
- macOS: o64-clang (amd64), oa64-clang (arm64)
- Windows: x86_64-w64-mingw32-gcc (amd64), llvm-mingw (arm64)
- Linux: Standard gcc (amd64), aarch64-linux-gnu-gcc (arm64)

## Docker Deployment
- Dockerfile provided for containerized deployment
- docker-compose.yml for easy setup
- Key extraction must be done outside Docker (host machine)
- Mount WeChat data directory as volume
