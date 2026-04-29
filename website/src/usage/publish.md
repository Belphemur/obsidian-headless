---
title: Publish
---

# Publish

## `ob publish-list-sites`

List publish sites associated with the logged-in account.

```bash
ob publish-list-sites
```

## `ob publish-create-site`

Create a publish site.

```bash
ob publish-create-site --slug <slug>
```

| Flag | Description |
|------|-------------|
| `--slug` | Site slug *(required)* |

## `ob publish-setup`

Attach a vault to a publish site.

```bash
ob publish-setup --site <site> [--path <path>]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--site` | *(required)* | Site ID or slug |
| `--path` | `.` | Local vault path |

## `ob publish-config`

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

## `ob publish-unlink`

Remove local publish configuration for a site.

```bash
ob publish-unlink [--path <path>]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--path` | `.` | Local vault path |

## `ob publish`

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
