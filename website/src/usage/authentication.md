---
title: Authentication
---

# Authentication

## Global Flags

These flags are available on all commands:

| Flag | Env Var | Default | Description |
|------|---------|---------|-------------|
| `--api-base` | `OBSIDIAN_API_BASE` | `https://api.obsidian.md` | Obsidian API base URL |
| `--timeout` | `OBSIDIAN_TIMEOUT` | `30` | HTTP timeout in seconds |
| `--log-level` | `OBSIDIAN_LOG_LEVEL` | `info` | Log level: debug, info, warn, error, fatal, panic, disabled, trace |

Environment variables are read automatically with the `OBSIDIAN_` prefix. Dashes in flag names become underscores in environment variables (e.g. `--api-base` → `OBSIDIAN_API_BASE`).

## `ob login`

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

## `ob logout`

Log out of the current account and clear stored credentials.

```bash
ob logout
```
