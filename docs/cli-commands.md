# CLI Command Reference

## Global Options

```
ob --help        Show help
ob --version     Show version number
```

## Authentication

### `ob login`

Log in to your Obsidian account. If already logged in, shows current user info.

```bash
ob login
ob login --email user@example.com --password secret
ob login --email user@example.com --password secret --mfa 123456
```

| Option | Description |
|--------|-------------|
| `--email <email>` | Account email address |
| `--password <password>` | Account password |
| `--mfa <code>` | Two-factor authentication code |

### `ob logout`

Sign out and clear the stored authentication token.

```bash
ob logout
```

## Vault Sync Commands

### `ob sync-list-remote`

List all remote vaults associated with your account.

```bash
ob sync-list-remote
```

### `ob sync-list-local`

List all locally configured sync vaults on this machine.

```bash
ob sync-list-local
```

### `ob sync-create-remote`

Create a new remote vault.

```bash
ob sync-create-remote --name "My Vault"
ob sync-create-remote --name "My Vault" --encryption standard
ob sync-create-remote --name "My Vault" --encryption e2ee --password secret
ob sync-create-remote --name "My Vault" --region us-east
```

| Option | Description | Default |
|--------|-------------|---------|
| `--name <name>` | Vault name (required) | — |
| `--encryption <type>` | `standard` or `e2ee` | `e2ee` |
| `--password <password>` | Encryption password (prompted if e2ee) | — |
| `--region <region>` | Server region | — |

### `ob sync-setup`

Set up a local directory for vault syncing. Associates a local path with a remote
vault. If the vault uses end-to-end encryption, prompts for the password.

```bash
ob sync-setup --vault "My Vault" --path ~/notes
ob sync-setup --vault abc123 --path ~/notes --device-name "server-1"
```

| Option | Description | Default |
|--------|-------------|---------|
| `--vault <idOrName>` | Vault ID or name (required) | — |
| `--path <path>` | Local vault directory | `.` |
| `--password <password>` | Encryption password | — |
| `--device-name <name>` | Device name | hostname |
| `--config-dir <dir>` | Config directory name | `.obsidian` |
| `--state-path <path>` | Custom path for the SQLite state database | auto |

### `ob sync-config`

Update sync configuration for a vault.

```bash
ob sync-config --path ~/notes --mode pull-only
ob sync-config --path ~/notes --conflict-strategy merge
ob sync-config --excluded-folders "archive,temp"
ob sync-config --file-types "image,audio,pdf"
ob sync-config --configs "appearance,hotkeys"
```

| Option | Description |
|--------|-------------|
| `--path <path>` | Vault path (default: `.`) |
| `--conflict-strategy <strategy>` | `merge` or `conflict` |
| `--excluded-folders <folders>` | Comma-separated folders to exclude |
| `--file-types <types>` | Comma-separated file types to sync |
| `--configs <categories>` | Comma-separated config categories |
| `--device-name <name>` | Device name |
| `--mode <mode>` | `bidirectional`, `pull-only`, or `mirror-remote` |
| `--config-dir <dir>` | Config directory name |
| `--state-path <path>` | Custom path for the SQLite state database |

### `ob sync-status`

Display the current sync configuration for a vault.

```bash
ob sync-status --path ~/notes
```

### `ob sync-unlink`

Disconnect a local vault from remote sync. Does not delete local files or the
remote vault.

```bash
ob sync-unlink --path ~/notes
```

### `ob sync`

Run the vault synchronization process.

```bash
ob sync --path ~/notes
ob sync --path ~/notes --continuous
```

| Option | Description |
|--------|-------------|
| `--path <path>` | Vault path (default: `.`) |
| `--continuous` | Run continuously, watching for changes |

**Sync Modes:**
- **Bidirectional** (default): Full two-way sync
- **Pull-only**: Only download changes, never upload
- **Mirror-remote**: Mirror the remote state exactly

**Conflict Resolution:**
- **Merge**: Three-way merge for `.md` files, JSON object merge for config files
- **Conflict**: Creates "Conflicted copy" files with device name and timestamp

## Publish Commands

### `ob publish-list-sites`

List all Obsidian Publish sites on your account.

```bash
ob publish-list-sites
```

### `ob publish-create-site`

Create a new Obsidian Publish site with a slug.

```bash
ob publish-create-site --slug my-digital-garden
```

### `ob publish-setup`

Connect a local vault to a publish site.

```bash
ob publish-setup --site my-digital-garden --path ~/notes
ob publish-setup --site site-id --path ~/notes
```

| Option | Description | Default |
|--------|-------------|---------|
| `--site <idOrSlug>` | Site ID or slug (required) | — |
| `--path <path>` | Local vault directory | `.` |

### `ob publish`

Publish changes from the local vault to the site.

```bash
ob publish --path ~/notes
ob publish --path ~/notes --dry-run
ob publish --path ~/notes --yes --all
```

| Option | Description |
|--------|-------------|
| `--path <path>` | Vault path (default: `.`) |
| `--dry-run` | Show changes without publishing |
| `--yes` | Skip confirmation prompt |
| `--all` | Publish all files, not just changed ones |

### `ob publish-config`

Update publish configuration.

```bash
ob publish-config --path ~/notes --includes "*.md" --excludes "drafts/**"
```

| Option | Description |
|--------|-------------|
| `--path <path>` | Vault path (default: `.`) |
| `--includes <patterns>` | Comma-separated include patterns |
| `--excludes <patterns>` | Comma-separated exclude patterns |

### `ob publish-unlink`

Disconnect a local vault from a publish site.

```bash
ob publish-unlink --path ~/notes
```
