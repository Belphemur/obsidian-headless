---
title: Configuration
---

# Configuration

## Directory Structure

Configuration and state are stored in the following locations:

| OS | Base Directory |
|----|----------------|
| Linux | `~/.config/obsidian-headless/` |
| macOS | `~/.obsidian-headless/` |

## Files

| File | Purpose |
|------|---------|
| `auth_token` | Not stored on disk — token is saved in the OS keyring (or encrypted `credentials.db` fallback) |
| `credentials.db` | Encrypted credentials database |
| `master.key` | Master encryption key |
| `sync/{vaultID}/config.json` | Vault sync configuration |
| `sync/{vaultID}/state.db` | Sync state database |
| `publish/{siteID}/config.json` | Publish site configuration |
| `publish/{siteID}/cache.json` | File hash cache for publish |

## Auth Token Precedence

The auth token is stored via the OS keyring (with an encrypted SQLite fallback). The CLI reads the token from the secret store on each command that requires authentication.

## Vault Selection

Commands that accept a vault selector (`--vault`) match by:
1. Vault ID
2. Vault UID
3. Vault name

## Site Selection

Commands that accept a site selector (`--site`) match by:
1. Site ID
2. Site slug

## Publish Selection Rules

When publishing, files are selected in the following priority:
1. `publish: true/false` frontmatter flag (highest priority)
2. Include/exclude patterns from publish config
3. `--all` flag to publish untagged files
