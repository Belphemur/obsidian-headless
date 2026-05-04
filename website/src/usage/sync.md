---
title: Sync
---

# Sync

## `ob sync-list-remote`

List remote vaults associated with the logged-in account.

```bash
ob sync-list-remote
```

## `ob sync-list-local`

List locally configured sync vaults.

```bash
ob sync-list-local
```

## `ob sync-create-remote`

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

## `ob sync-setup`

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
| `--state-path` | | Custom state database path (default: `~/.config/obsidian-headless/sync/{vaultID}/state.db`) |
| `--periodic-scan` | `1h` | Periodic full rescan interval (e.g. `60s`, `5m`, `1h`); set to `0` to disable |

## `ob sync-config`

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
| `--state-path` | | Custom state database path (default: `~/.config/obsidian-headless/sync/{vaultID}/state.db`) |
| `--periodic-scan` | | Periodic full rescan interval (e.g. `60s`, `5m`, `1h`); set to `0` to disable |

When called without any update flags, displays the current configuration.

## `ob sync-status`

Show sync configuration for a vault.

```bash
ob sync-status [--path <path>]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--path` | `.` | Local vault path |

## `ob sync-unlink`

Remove local sync configuration for a vault.

```bash
ob sync-unlink [--path <path>]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--path` | `.` | Local vault path |

## `ob sync`

Run sync for a configured vault.

```bash
ob sync [--path <path>] [--continuous]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--path` | `.` | Local vault path |
| `--continuous` | `false` | Run continuously (watch for changes) |

### Remote rename handling

When files are renamed on another device (e.g., via the Obsidian desktop app), the sync engine detects the rename automatically via UID matching:

- **Unmodified local files** are renamed in-place — no re-download is needed.
- **Modified local files** are preserved at their original path. The remote version is downloaded to the new path as a separate file, and the conflict is logged.

This works in both one-shot (`ob sync`) and continuous (`ob sync --continuous`) modes without any configuration.
