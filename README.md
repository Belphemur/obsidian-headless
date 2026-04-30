# Obsidian Headless Go

Headless client for [Obsidian Sync](https://obsidian.md/sync) and [Obsidian Publish](https://obsidian.md/publish).
Sync and publish your vaults from the command line without the desktop app.

Full documentation: https://belphemur.github.io/obsidian-headless/

Built with Go `1.26`.

- Entry point: `src/cmd/ob-go/main.go`
- Key libraries: Cobra, Viper, zerolog, `modernc.org/sqlite`, `gorilla/websocket`, `fsnotify`

## Install

### macOS (Homebrew)

```bash
brew install --cask belphemur/homebrew-tap/obsidian-headless
```

### Linux

Pre-built packages are available on the [GitHub Releases](https://github.com/Belphemur/obsidian-headless/releases/latest) page:

| Distribution | Package | Command |
|---|---|---|
| Debian / Ubuntu | `.deb` | `sudo apt install ./obsidian-headless*.deb` |
| Fedora / RHEL | `.rpm` | `sudo dnf install ./obsidian-headless*.rpm` |
| Alpine Linux | `.apk` | `sudo apk add --allow-untrusted ./obsidian-headless*.apk` |
| Arch Linux | `.pkg.tar.zst` | `sudo pacman -U ./obsidian-headless*.pkg.tar.zst` |
| Arch Linux (AUR) | AUR | `yay -S obsidian-headless-bin` |

### Windows

```powershell
winget install Belphemur.ObsidianHeadless
```

Or download the `.zip` from [GitHub Releases](https://github.com/Belphemur/obsidian-headless/releases/latest).

### Go Install

```bash
go install github.com/Belphemur/obsidian-headless/src-go/cmd/ob-go@latest
```

### Build from Source

```bash
cd src
go build -o ob ./cmd/ob-go
```

## Authentication

Login interactively:

```bash
ob login
```

If already logged in, `ob login` displays your account info. To switch accounts, pass `--email` and/or `--password` to log in again.

## Quick start

```bash
# Login
ob login

# List your remote vaults
ob sync-list-remote

# Setup a vault for syncing
cd ~/vaults/my-vault
ob sync-setup --vault "My Vault"

# Run a one-time sync
ob sync

# Run continuous sync (watches for changes)
ob sync --continuous
```

## Docker

A ready-to-use Docker image is available for running continuous sync in a container. See [`build/README.md`](build/README.md) for the full Docker quick start, environment variables, and troubleshooting guide.

```bash
docker run --rm -it \
  -v ./config:/home/obsidian/.config \
  --entrypoint get-token \
  ghcr.io/belphemur/obsidian-headless:latest
```

## Global options

These flags are available on every command:

| Option | Description |
|---|---|
| `--api-base <url>` | Obsidian API base URL (default: `https://api.obsidian.md`) |
| `--timeout <seconds>` | HTTP timeout in seconds (default: `30`) |
| `--log-level <level>` | Log level: `debug`, `info`, `warn`, `error`, `fatal`, `panic`, `disabled`, `trace` (default: `info`) |

## Commands

### `ob login`

Login to your Obsidian account, or display login status if already logged in.

```
ob login [--email <email>] [--password <password>] [--mfa <code>]
```

All options are interactive when omitted — email and password are prompted, and 2FA is requested automatically if enabled on the account.

### `ob logout`

Logout and clear stored credentials.

### `ob sync-list-remote`

List all remote vaults available to your account, including shared vaults.

### `ob sync-list-local`

List locally configured vaults and their paths.

### `ob sync-create-remote`

Create a new remote vault.

```
ob sync-create-remote --name "Vault Name" [--encryption <standard|e2ee>] [--password <password>] [--region <region>]
```

| Option | Description |
|---|---|
| `--name` | Vault name (required) |
| `--encryption` | `standard` for managed encryption, `e2ee` for end-to-end (default: `e2ee`) |
| `--password` | End-to-end encryption password (prompted if omitted) |
| `--region` | Server region (automatic if omitted) |

### `ob sync-setup`

Set up sync between a local vault and a remote vault.

```
ob sync-setup --vault <id-or-name> [--path <local-path>] [--password <password>] [--device-name <name>] [--config-dir <name>] [--state-path <path>] [--periodic-scan <duration>]
```

| Option | Description |
|---|---|
| `--vault` | Remote vault ID or name (required) |
| `--path` | Local directory (default: current directory) |
| `--password` | E2E encryption password (prompted if omitted) |
| `--device-name` | Device name to identify this client in the sync version history (default: hostname) |
| `--config-dir` | Config directory name (default: `.obsidian`) |
| `--state-path` | Custom path for the SQLite state database (default: auto) |
| `--periodic-scan` | Periodic full rescan interval, e.g. `60s`, `5m`, `1h`; set to `0` to disable. Only active in bidirectional mode (default: `1h`) |

### `ob sync`

Run sync for a configured vault.

```
ob sync [--path <local-path>] [--continuous]
```

| Option | Description |
|---|---|
| `--path` | Local vault path (default: current directory) |
| `--continuous` | Run continuously, watching for changes |

### `ob sync-config`

View or change sync settings for a vault.

```
ob sync-config [--path <local-path>] [options]
```

Run with no options to display the current configuration.

| Option | Description |
|---|---|
| `--path` | Local vault path (default: current directory) |
| `--mode` | Sync mode: `bidirectional` (default), `pull` (only download, ignore local changes), or `mirror` (only download, revert local changes) |
| `--conflict-strategy` | `merge` or `conflict` |
| `--file-types` | Attachment types to sync: `image`, `audio`, `video`, `pdf`, `unsupported` (comma-separated, empty to clear) |
| `--configs` | Config categories to sync: `app`, `appearance`, `appearance-data`, `hotkey`, `core-plugin`, `core-plugin-data`, `community-plugin`, `community-plugin-data` (comma-separated, empty to disable config syncing) |
| `--excluded-folders` | Folders to exclude (comma-separated, empty to clear) |
| `--device-name` | Device name to identify this client in the sync version history |
| `--config-dir` | Config directory name (default: `.obsidian`) |
| `--state-path` | Custom path for the SQLite state database |
| `--periodic-scan` | Periodic full rescan interval, e.g. `60s`, `5m`, `1h`; set to `0` to disable. Only active in bidirectional mode |

### `ob sync-status`

Show sync status and configuration for a vault.

```
ob sync-status [--path <local-path>]
```

### `ob sync-unlink`

Disconnect a vault from sync and remove stored credentials.

```
ob sync-unlink [--path <local-path>]
```

### `ob publish-list-sites`

List all publish sites available to your account, including shared sites.

### `ob publish-create-site`

Create a new publish site.

```
ob publish-create-site --slug <slug>
```

| Option | Description |
|---|---|
| `--slug` | Site slug used in the publish URL (required) |

### `ob publish-setup`

Connect a local vault to a publish site.

```
ob publish-setup --site <id-or-slug> [--path <local-path>]
```

| Option | Description |
|---|---|
| `--site` | Site ID or slug (required) |
| `--path` | Local vault path (default: current directory) |

### `ob publish`

Publish vault changes to a connected site. Scans for changes by comparing local file hashes against the remote site, then uploads new/changed files and removes deleted ones.

Files are selected for publishing based on: frontmatter `publish: true/false` flag (highest priority), included/excluded patterns (configured via `publish-config`), and the `--all` flag for untagged files.

```
ob publish [--path <local-path>] [--dry-run] [--yes] [--all]
```

| Option | Description |
|---|---|
| `--path` | Local vault path (default: current directory) |
| `--dry-run` | Show changes without publishing |
| `--yes` | Publish without prompting for confirmation |
| `--all` | Include files without a publish flag |

### `ob publish-config`

View or change publish settings for a vault.

```
ob publish-config [--path <local-path>] [--includes <patterns>] [--excludes <patterns>]
```

Run with no options to display the current configuration.

| Option | Description |
|---|---|
| `--path` | Local vault path (default: current directory) |
| `--includes` | Include patterns, comma-separated (empty string to clear) |
| `--excludes` | Exclude patterns, comma-separated (empty string to clear) |

### `ob publish-unlink`

Disconnect a vault from a publish site.

```
ob publish-unlink [--path <local-path>]
```

## Documentation

Full documentation is available at **[belphemur.github.io/obsidian-headless](https://belphemur.github.io/obsidian-headless)**:

- [Installation Guide](https://belphemur.github.io/obsidian-headless/installation) — Install via brew, apt, rpm, apk, pacman, winget, go install, or Docker
- [Usage Guide](https://belphemur.github.io/obsidian-headless/usage) — Complete CLI command reference for Sync and Publish
- [Architecture](https://belphemur.github.io/obsidian-headless/architecture/) — Sync protocol, encryption, and REST API deep-dive

Additional reference docs in the `docs/` directory:

| Document | Description |
|----------|-------------|
| [Architecture Overview](docs/architecture.md) | High-level module layout and data flow |
| [Sync Protocol](docs/sync-protocol.md) | WebSocket sync protocol specification |
| [Encryption Protocol](docs/encryption-protocol.md) | Encryption versions and key derivation |
| [REST API](docs/rest-api.md) | HTTP API endpoints for authentication, vault and publish management |
| [CLI Commands](docs/cli-commands.md) | Complete CLI command reference |
| [Mock Server](docs/mock-server.md) | How to run and use the mock server for testing |

Implementation progress is tracked in `docs/go-port-progress.md`.

## Configuration

Configuration and state are stored under:

- **Linux**: `~/.config/obsidian-headless/`
- **macOS**: `~/.obsidian-headless/`

Key files:

| Path | Description |
|------|-------------|
| `auth_token` | Obsidian authentication token |
| `credentials.db` | SQLite database for encrypted credentials |
| `master.key` | Master encryption key |
| `sync/{vaultID}/config.json` | Per-vault sync configuration |
| `sync/{vaultID}/state.db` | Per-vault SQLite sync state (local/server file records) |
| `publish/{siteID}/config.json` | Per-site publish configuration |

Sensitive values (auth tokens, vault encryption keys, salts) are stored via the OS keyring when available, with an encrypted SQLite fallback.
