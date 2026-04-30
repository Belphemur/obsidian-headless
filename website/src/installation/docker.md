---
title: Docker Installation
---

# Docker

## From GitHub Container Registry

Images are published to the GitHub Container Registry on version tags. Multi-arch images are available for `linux/amd64` and `linux/arm64`.

```bash
docker pull ghcr.io/belphemur/obsidian-headless:latest
```

## Quick Start

### Step 1 — Log in (one-time)

Pull the image and run the interactive login helper:

```bash
docker run --rm -it \
  -v ./config:/home/obsidian/.config \
  --entrypoint get-token \
  ghcr.io/belphemur/obsidian-headless:latest
```

> **Note:** The login state persists in the config volume. You only need to run this once per machine (or after logging out / revoking the token).

### Step 2 — Find your remote vault name (one-time)

List the vaults available on your Obsidian Sync account:

```bash
docker run --rm \
  -v ./config:/home/obsidian/.config \
  --entrypoint ob \
  ghcr.io/belphemur/obsidian-headless:latest \
  sync-list-remote
```

> **⚠️ Note:** Using `--entrypoint ob` bypasses s6-overlay and runs the command as root. This is fine for read-only operations like `sync-list-remote`, but avoid running write commands this way as it may create root-owned files that conflict with the unprivileged service.

Note the exact vault name — you'll use it in `VAULT_NAME`.

### Step 3 — Configure your environment

```bash
cp .env.example .env
```

Edit `.env` and fill in at minimum:

```env
VAULT_NAME=My Vault
VAULT_HOST_PATH=./vault
CONFIG_HOST_PATH=./config
```

### Step 4 — Start continuous sync

```bash
docker compose up -d
```

The container runs `ob sync-setup` automatically on every start when `VAULT_NAME` is set. The setup command is idempotent — it safely re-links the vault if needed without duplicating configuration. Once linked, the container enters continuous sync mode.

Watch logs:

```bash
docker compose logs -f
```

## Docker Compose

```yaml
# compose.yml
services:
  obsidian-sync:
    image: ghcr.io/belphemur/obsidian-headless:latest
    environment:
      - VAULT_NAME=My Vault
      - VAULT_PASSWORD=your-password
      - DEVICE_NAME=synology-nas
      - PUID=1000
      - PGID=1000
    volumes:
      - /path/to/vault:/vault
      - obsidian-config:/home/obsidian/.config
    restart: unless-stopped

volumes:
  obsidian-config:
```

## Environment Variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `VAULT_NAME` | Yes (first run) | — | Exact name of the remote Obsidian Sync vault |
| `VAULT_PASSWORD` | If E2E enabled | — | Vault end-to-end encryption password (see below) |
| `VAULT_HOST_PATH` | Yes | `./vault` | Host path where vault files will be written |
| `CONFIG_HOST_PATH` | No | `./config` | Host path for persistent config (login state, keyring, etc.) |
| `PUID` | No | `1000` | UID that will own synced files (see below) |
| `PGID` | No | `1000` | GID that will own synced files (see below) |
| `VAULT_PATH` | No | `/vault` | In-container mount path (advanced) |
| `DEVICE_NAME` | No | `obsidian-docker` | Label shown in Obsidian Sync history |
| `CONFLICT_STRATEGY` | No | `merge` | `merge` or `conflict` |
| `EXCLUDED_FOLDERS` | No | — | Comma-separated vault folders to skip |
| `FILE_TYPES` | No | — | Extra types to sync: `image,audio,video,pdf,unsupported` |
| `SYNC_MODE` | No | `bidirectional` | Sync mode: `bidirectional`, `pull`, or `mirror` |
| `PERIODIC_SCAN` | No | `1h` | Periodic full rescan interval (e.g. `60s`, `5m`, `1h`); set to `0` to disable. Only active in bidirectional mode. |
| `SYNC_CONFIGS` | No | — | Comma-separated config categories to sync (see below) |
| `GHCR_REPO` | No | — | Override image repository when self-building |
| `IMAGE_TAG` | No | `latest` | Image tag to pull |

## File Ownership (PUID / PGID)

At startup the container adjusts its internal `obsidian` user to match the `PUID`/`PGID` you provide, then drops privileges before running any Obsidian commands. This means vault files on the host are owned by the UID/GID you choose.

**Regular Docker** (daemon runs as root):

```bash
# Find your UID and GID
id
# uid=1000(you) gid=1000(you) ...
```

```env
PUID=1000
PGID=1000
```

**Rootless Docker / Podman** (daemon runs as your user):

In rootless mode, container UID 0 already maps to your host user. Set both to `0`:

```env
PUID=0
PGID=0
```

## End-to-End Encryption (VAULT_PASSWORD)

Obsidian Sync supports optional end-to-end encryption with a separate vault password. If your vault has this enabled, `ob sync-setup` will fail to authenticate until the password is provided.

**To check:** In the Obsidian desktop app, go to **Settings → Sync** and look for an "Encryption password" field — if it's present and set, E2E is active.

Add the password to your `.env`:

```env
VAULT_PASSWORD=your-vault-encryption-password
```

> **Note:** `VAULT_PASSWORD` is the *vault encryption password* you chose in Obsidian, not your Obsidian account password. They are separate credentials.

## Sync Configuration (SYNC_MODE / SYNC_CONFIGS)

These variables map directly to `ob sync-config` options and are applied every time the container starts.

### SYNC_MODE

Controls how local and remote changes are reconciled.

| Value | Behaviour |
|---|---|
| `bidirectional` | Upload local changes **and** download remote changes (default) |
| `pull` | Download remote changes only — local changes are ignored |
| `mirror` | Download remote changes only — local changes are reverted |

```env
SYNC_MODE=pull
```

### SYNC_CONFIGS

Comma-separated list of Obsidian config categories to sync alongside vault notes. Leave blank to keep the vault's existing setting (all categories synced by default).

| Value | Syncs |
|---|---|
| `app` | Core app settings |
| `appearance` | Theme and appearance settings |
| `appearance-data` | Theme assets (CSS snippets, etc.) |
| `hotkey` | Keyboard shortcuts |
| `core-plugin` | Core plugin toggle states |
| `core-plugin-data` | Core plugin configuration data |
| `community-plugin` | Community plugin list and toggle states |
| `community-plugin-data` | Community plugin configuration data |

```env
# Sync only app settings and hotkeys
SYNC_CONFIGS=app,hotkey
```

## Architecture

This image uses [s6-overlay v3](https://github.com/just-containers/s6-overlay) as its init system.

The startup sequence runs through ordered s6-rc services:

1. **init-setup-user** — adjusts UID/GID to match `PUID`/`PGID`
2. **init-setup-vault** — runs `ob sync-setup` and applies optional config
3. **svc-obsidian-sync** — starts `ob sync-run --continuous` under s6 supervision

If any init step fails, the container exits immediately.

## Updating the Image

```bash
docker compose pull
docker compose up -d
```

## Stopping

```bash
docker compose down
```

Your vault files remain on disk at `VAULT_HOST_PATH`.

## Troubleshooting

**Container exits immediately**
- Check that `VAULT_NAME` is set: `docker compose config`
- Check init logs: the container stops on any init failure

**"Vault not found" error on setup**
- Confirm the vault name matches exactly (case-sensitive): run `ob sync-list-remote` as shown in Step 2.

**"Failed to validate password" on setup**
- Your vault has end-to-end encryption enabled. Set `VAULT_PASSWORD` in `.env` to the encryption password from **Obsidian → Settings → Sync**. This is distinct from your Obsidian account password.

**Sync stops after a while**
- The `restart: unless-stopped` policy in `compose.yml` will restart the container automatically. Within the container, s6 supervises the sync process and restarts it if it exits.

**Permission denied on vault files**
- The container adjusts its internal user to match `PUID`/`PGID` (default `1000:1000`). Set these in `.env` to match the host user who should own the files (`id` shows your values).
- For rootless Docker/Podman, set both to `0`.
