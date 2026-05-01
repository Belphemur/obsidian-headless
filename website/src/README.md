---
home: true
title: Home
heroImage: /logo.svg
heroAlt: Obsidian Headless Go Logo
heroText: Obsidian Headless Go
tagline: Headless Go CLI client for Obsidian Sync and Obsidian Publish
footerHtml: true
actions:
  - text: Get Started
    link: /getting-started
    type: primary
  - text: View on GitHub
    link: https://github.com/Belphemur/obsidian-headless
    type: secondary
features:
  - title: Headless Sync
    icon: fa-solid:rotate
    details: Run Obsidian Sync on servers, NAS devices, or Raspberry Pi. Continuous daemon mode keeps your vault in sync without a GUI.
  - title: Headless Publish
    icon: fa-solid:globe
    details: Publish your vault to Obsidian Publish from the command line. Automate deployments in CI/CD pipelines.
  - title: E2E Encryption
    icon: fa-solid:lock
    details: End-to-end encrypted vaults with V2/V3 encryption. Your data is encrypted before it leaves your device — zero-knowledge.
  - title: Cross-Platform
    icon: fa-solid:computer
    details: Available on Linux, macOS, and Windows. Native packages for apt, dnf, pacman, apk; Homebrew Cask for macOS; Winget for Windows.
  - title: Docker Native
    icon: fa-brands:docker
    details: Official Docker images on ghcr.io with s6-overlay process supervision. Run sync as a container with environment-based configuration.
  - title: CLI-First
    icon: fa-solid:terminal
    details: Full Cobra-based CLI with every Obsidian Sync and Publish operation. Automate, script, and integrate into your workflow.
footer: GPL-3.0 Licensed | Copyright © 2026-present Belphemur | <a href="https://github.com/Belphemur/obsidian-headless" target="_blank" rel="noopener noreferrer">GitHub</a>
---

## Getting Started

Choose your setup and get Obsidian Headless Go running in under 5 minutes:

- **[Docker](./getting-started.md#docker-quick-start)** — Run sync as a container. Login, configure environment variables, and start.
- **[Local CLI](./getting-started.md#local-quick-start)** — Install the binary, log in, set up sync, and run it directly on your machine.

```bash
# Docker: one login, then docker compose up
docker run --rm -it -v ./config:/home/obsidian/.config --entrypoint get-token ghcr.io/belphemur/obsidian-headless:latest

# Local: install, login, setup, run
brew install --cask obsidian-headless   # (or your platform's package)
ob login
ob sync-setup --vault "My Vault" --path /path/to/vault
ob sync-run --continuous
```

See the full **[Getting Started guide](./getting-started.md)** for detailed step-by-step instructions.

## What is Obsidian Headless Go?

Obsidian Headless Go is a community-maintained CLI companion for [Obsidian Sync](https://obsidian.md/sync) and [Obsidian Publish](https://obsidian.md/publish). It brings the full power of Obsidian's sync and publish engines to the command line — no GUI required.

Perfect for:

- **Servers & NAS:** Keep vaults in sync on headless Linux machines
- **CI/CD:** Publish documentation sites automatically on push
- **Docker:** Run sync as a container with environment variables
- **Automation:** Script sync and publish operations in your workflow

::: danger ⚠️ Third-Party Disclaimer
**Obsidian Headless Go is a third-party, community-maintained tool.** It is NOT an official Obsidian product, and it is NOT supported by Obsidian.

If you encounter any issues, do **NOT** contact Obsidian support. Instead, please [open an issue on GitHub](https://github.com/Belphemur/obsidian-headless/issues).
:::
