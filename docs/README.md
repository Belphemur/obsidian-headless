# Obsidian Headless – Documentation

This directory contains the protocol and architecture documentation for the
Obsidian Headless CLI, a command-line client for Obsidian Sync and Obsidian
Publish services.

## Documents

| Document | Description |
|----------|-------------|
| [Architecture Overview](./architecture.md) | High-level module layout and data flow |
| [Sync Protocol](./sync-protocol.md) | WebSocket sync protocol specification |
| [Encryption Protocol](./encryption-protocol.md) | Encryption versions (V0, V2, V3) and key derivation |
| [REST API](./rest-api.md) | HTTP API endpoints for authentication, vault and publish management |
| [CLI Commands](./cli-commands.md) | Complete CLI command reference |
| [Mock Server](./mock-server.md) | How to run and use the mock server for testing |
| [Parallel Downloads](./parallel-downloads.md) | Connection pool design for concurrent file downloads |

## Quick Start

```bash
# Build and run the Go CLI
cd src
go build -o ob ./cmd/ob-go
./ob --help

# Run mock server (for development/testing)
node mock-server/server.mjs
```
