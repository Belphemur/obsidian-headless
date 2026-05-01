---
title: Getting Started
---

# Getting Started

::: danger ⚠️ Third-Party Disclaimer
**Obsidian Headless Go is a third-party, community-maintained tool.** It is NOT an official Obsidian product, and it is NOT supported by Obsidian.

If you encounter any issues, do **NOT** contact Obsidian support. Instead, please [open an issue on GitHub](https://github.com/Belphemur/obsidian-headless/issues).
:::

Choose your setup: run Obsidian Headless Go as a Docker container or install the CLI directly on your machine.

## Docker Quick Start

The Docker image handles everything — just log in, configure, and start. This is a third-party tool; see the disclaimer above.

### Step 1 — Log in (one-time)

Pull the image and run the interactive login helper. It will prompt for your Obsidian email, password, and MFA code (if enabled), then store the auth token securely in the config volume.

```bash
docker run --rm -it \
  -v ./config:/home/obsidian/.config \
  --entrypoint get-token \
  ghcr.io/belphemur/obsidian-headless:latest
```

> **Note:** The login state persists in the config volume. You only need to run this once per machine.

### Step 2 — Find your remote vault name (one-time)

List the vaults available on your Obsidian Sync account:

```bash
docker run --rm \
  -v ./config:/home/obsidian/.config \
  --entrypoint ob \
  ghcr.io/belphemur/obsidian-headless:latest \
  sync-list-remote
```

Note the exact vault name — you'll use it in `VAULT_NAME`.

### Step 3 — Configure your environment

Create a `.env` file:

```bash
cp .env.example .env
```

Edit `.env` and fill in at minimum:

```env
VAULT_NAME=My Vault
VAULT_HOST_PATH=./vault
CONFIG_HOST_PATH=./config
```

| Variable | Required | Default | Description |
|---|---|---|---|
| `VAULT_NAME` | Yes (first run) | — | Exact name of the remote Obsidian Sync vault |
| `VAULT_PASSWORD` | If E2E enabled | — | Vault end-to-end encryption password |
| `VAULT_HOST_PATH` | Yes | `./vault` | Host path where vault files will be written |
| `CONFIG_HOST_PATH` | No | `./config` | Host path for persistent config (login state, keyring, etc.) |
| `PUID` | No | `1000` | UID that will own synced files |
| `PGID` | No | `1000` | GID that will own synced files |
| `VAULT_PATH` | No | `/vault` | In-container mount path (advanced) |
| `DEVICE_NAME` | No | `obsidian-docker` | Label shown in Obsidian Sync history |
| `CONFLICT_STRATEGY` | No | `merge` | `merge` or `conflict` |
| `EXCLUDED_FOLDERS` | No | — | Comma-separated vault folders to skip |
| `FILE_TYPES` | No | — | Extra types to sync: `image,audio,video,pdf,unsupported` |
| `SYNC_MODE` | No | `bidirectional` | Sync mode: `bidirectional`, `pull`, or `mirror` |
| `PERIODIC_SCAN` | No | `1h` | Periodic full rescan interval (e.g. `60s`, `5m`, `1h`); set to `0` to disable |
| `SYNC_CONFIGS` | No | — | Comma-separated config categories to sync |
| `GHCR_REPO` | No | — | Override image repository when self-building |
| `IMAGE_TAG` | No | `latest` | Image tag to pull |

### Step 4 — Start continuous sync

Create a `compose.yml` (or add the service to your existing compose file):

<details>
<summary>Example compose.yml</summary>

```yaml
services:
  obsidian-sync:
    image: ghcr.io/belphemur/obsidian-headless:latest
    environment:
      - VAULT_NAME=My Vault
      - VAULT_HOST_PATH=./vault
      - CONFIG_HOST_PATH=./config
      - PUID=1000
      - PGID=1000
    volumes:
      - ./vault:/vault
      - ./config:/home/obsidian/.config
    restart: unless-stopped
```
</details>

Start the container:

```bash
docker compose up -d
```

The container runs `ob sync-setup` automatically on every start when `VAULT_NAME` is set. The setup command is idempotent — it safely re-links the vault if needed without duplicating configuration. Once linked, the container enters continuous sync mode.

Watch logs:

```bash
docker compose logs -f
```

::: tip PUID / PGID
For **regular Docker** (daemon runs as root), set `PUID`/`PGID` to your host user's UID/GID (`id` to find them).

For **rootless Docker / Podman**, set both to `0` — container UID 0 already maps to your host user.
:::

## Local Quick Start

Install the CLI on your machine, log in, and set up sync.

### Step 1 — Install the CLI

Choose your platform from the [Installation Guide](./installation/README.md):

- [macOS](./installation/macos.md) — `brew install --cask obsidian-headless`
- [Linux](./installation/linux.md) — deb, rpm, pacman, apk
- [Windows](./installation/windows.md) — `winget install obsidian-headless`
- [From Source](./installation/from-source.md) — `go install`

Verify installation:

```bash
ob --version
```

### Step 2 — Log in to your Obsidian account

The `ob login` command authenticates you with Obsidian Sync and Publish. When running login, you will be prompted to accept the third-party disclaimer first:

```bash
ob login
```

It will prompt for your email, password, and MFA code if two-factor authentication is enabled.

> **Security:** Avoid passing passwords directly on the command line. They can be exposed in shell history and process lists. Use the interactive prompt or a credential manager instead.

The auth token is stored securely in your OS keyring (with an encrypted SQLite fallback). You only need to log in once.

### Step 3 — List your remote vaults

Find the exact name of the vault you want to sync:

```bash
ob sync-list-remote
```

### Step 4 — Set up sync

The `ob sync-setup` command links a local directory to a remote vault:

```bash
ob sync-setup --vault "My Vault" --path /path/to/vault
```

This creates the necessary configuration and state files that connect your local folder to the Obsidian Sync server.

::: warning Encrypted vaults
If your vault has end-to-end encryption enabled, you'll be prompted for the vault encryption password. You can also pass it with `--password`.
:::

### Step 5 — Configure sync (optional)

After setup, you can customize sync behavior with `ob sync-config`:

```bash
# View current config
ob sync-config --path /path/to/vault

# Set sync mode to pull-only
ob sync-config --path /path/to/vault --mode pull

# Set conflict strategy
ob sync-config --path /path/to/vault --conflict-strategy merge
```

### Step 6 — Run sync

Start syncing:

```bash
# One-time sync
ob sync-run

# Continuous sync (keeps watching for changes)
ob sync-run --continuous
```

The `--continuous` flag runs the sync daemon in the foreground, watching for local file changes and syncing them in real time.

## Next Steps

- [Usage Guide](./usage/README.md) — Full command reference for sync and publish
- [Configuration](./usage/configuration.md) — Directory structure and config files
- [Architecture](./architecture/README.md) — How Sync Protocol, Encryption, and REST API work
