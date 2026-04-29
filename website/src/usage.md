---
title: Usage
---

# Usage

[[toc]]

## Global Flags

These flags are available on all commands:

| Flag | Env Var | Default | Description |
|------|---------|---------|-------------|
| `--api-base` | `OBSIDIAN_API_BASE` | `https://api.obsidian.md` | Obsidian API base URL |
| `--timeout` | `OBSIDIAN_TIMEOUT` | `30` | HTTP timeout in seconds |
| `--log-level` | `OBSIDIAN_LOG_LEVEL` | `info` | Log level: debug, info, warn, error, fatal, panic, disabled, trace |

Environment variables are read automatically with the `OBSIDIAN_` prefix. Dashes in flag names become underscores in environment variables (e.g. `--api-base` → `OBSIDIAN_API_BASE`).

## Authentication

### `ob login`

Log in to an Obsidian account.

::: tip
If already logged in and no credentials are provided, shows the current account info instead.
:::

```bash
ob login [--email <email>] [--password <password>] [--mfa <code>]
```

| Flag | Description |
|------|-------------|
| `--email` | Account email |
| `--password` | Account password (prompts if not provided) |
| `--mfa` | MFA/2FA code |

If 2FA is required and `--mfa` is omitted, the CLI will prompt for the code interactively.

### `ob logout`

Log out of the current account and clear stored credentials.

```bash
ob logout
```

## Sync

### `ob sync-list-remote`

List remote vaults associated with the logged-in account.

```bash
ob sync-list-remote
```

### `ob sync-list-local`

List locally configured sync vaults.

```bash
ob sync-list-local
```

### `ob sync-create-remote`

Create a remote sync vault with optional encryption.

```bash
ob sync-create-remote --name <name> [--encryption <mode>] [--password <password>] [--region <region>]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--name` | *(required)* | Vault name |
| `--encryption` | `e2ee` | Encryption mode: `standard` or `e2ee` |
| `--password` | | Encryption password (required for `e2ee`) |
| `--region` | | Vault region |

Encryption modes:
- `standard` — No encryption
- `e2ee` — End-to-end encrypted (requires `--password`)

### `ob sync-setup`

Attach a local folder to a remote vault.

```bash
ob sync-setup --vault <vault> [--path <path>] [--password <password>] [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--vault` | *(required)* | Vault ID or name |
| `--path` | `.` | Local vault path |
| `--password` | | Encryption password (prompts for encrypted vaults) |
| `--device-name` | | Device name |
| `--config-dir` | `.obsidian` | Config directory |
| `--state-path` | | Custom state database path |
| `--periodic-scan` | `1h` | Periodic full rescan interval (e.g. `60s`, `5m`, `1h`); set to `0` to disable |

### `ob sync-config`

View or update sync settings for a configured vault.

```bash
ob sync-config [--path <path>] [--mode <mode>] [--conflict-strategy <strategy>] [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--path` | `.` | Local vault path |
| `--mode` | | Sync mode: `bidirectional`, `pull`, or `mirror` |
| `--conflict-strategy` | `merge` | Conflict resolution: `merge` or `conflict` |
| `--excluded-folders` | | Comma-separated folder names to exclude |
| `--file-types` | | Comma-separated file types to allow |
| `--configs` | | Comma-separated config categories to allow |
| `--device-name` | | Device name |
| `--config-dir` | `.obsidian` | Config directory |
| `--state-path` | | Custom state database path |
| `--periodic-scan` | | Periodic full rescan interval (e.g. `60s`, `5m`, `1h`); set to `0` to disable |

When called without any update flags, displays the current configuration.

### `ob sync-status`

Show sync configuration for a vault.

```bash
ob sync-status [--path <path>]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--path` | `.` | Local vault path |

### `ob sync-unlink`

Remove local sync configuration for a vault.

```bash
ob sync-unlink [--path <path>]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--path` | `.` | Local vault path |

### `ob sync`

Run sync for a configured vault.

```bash
ob sync [--path <path>] [--continuous]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--path` | `.` | Local vault path |
| `--continuous` | `false` | Run continuously (watch for changes) |

## Publish

### `ob publish-list-sites`

List publish sites associated with the logged-in account.

```bash
ob publish-list-sites
```

### `ob publish-create-site`

Create a publish site.

```bash
ob publish-create-site --slug <slug>
```

| Flag | Description |
|------|-------------|
| `--slug` | Site slug *(required)* |

### `ob publish-setup`

Attach a vault to a publish site.

```bash
ob publish-setup --site <site> [--path <path>]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--site` | *(required)* | Site ID or slug |
| `--path` | `.` | Local vault path |

### `ob publish-config`

View or update publish settings for a configured site.

```bash
ob publish-config [--path <path>] [--includes <patterns>] [--excludes <patterns>]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--path` | `.` | Local vault path |
| `--includes` | | Comma-separated include patterns |
| `--excludes` | | Comma-separated exclude patterns |

When called without any update flags, displays the current configuration.

### `ob publish-unlink`

Remove local publish configuration for a site.

```bash
ob publish-unlink [--path <path>]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--path` | `.` | Local vault path |

### `ob publish`

Publish vault changes to the configured site.

```bash
ob publish [--path <path>] [--dry-run] [--yes] [--all]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--path` | `.` | Local vault path |
| `--dry-run` | `false` | Show changes without publishing |
| `--yes` | `false` | Apply changes without confirmation |
| `--all` | `false` | Publish untagged files too |

## Configuration

### Directory Structure

Configuration and state are stored in the following locations:

| OS | Base Directory |
|----|----------------|
| Linux | `~/.config/obsidian-headless/` |
| macOS | `~/.obsidian-headless/` |

### Files

| File | Purpose |
|------|---------|
| `auth_token` | Not stored on disk — token is saved in the OS keyring (or encrypted `credentials.db` fallback) |
| `credentials.db` | Encrypted credentials database |
| `master.key` | Master encryption key |
| `sync/{vaultID}/config.json` | Vault sync configuration |
| `sync/{vaultID}/state.db` | Sync state database |
| `publish/{siteID}/config.json` | Publish site configuration |

### Auth Token Precedence

The auth token is stored via the OS keyring (with an encrypted SQLite fallback). The CLI reads the token from the secret store on each command that requires authentication.

### Vault Selection

Commands that accept a vault selector (`--vault`) match by:
1. Vault ID
2. Vault UID
3. Vault name

### Site Selection

Commands that accept a site selector (`--site`) match by:
1. Site ID
2. Site slug

### Publish Selection Rules

When publishing, files are selected in the following priority:
1. `publish: true/false` frontmatter flag (highest priority)
2. Include/exclude patterns from publish config
3. `--all` flag to publish untagged files
