---
home: true
title: Home
heroImage: /obsidian-sync-headless.png
heroAlt: Obsidian Headless Go Logo
heroText: Obsidian Headless Go
tagline: Headless Go CLI client for Obsidian Sync and Obsidian Publish
actions:
  - text: Get Started
    link: /installation
    type: primary
  - text: View on GitHub
    link: https://github.com/Belphemur/obsidian-headless
    type: secondary
features:
  - title: Headless Sync
    details: Run Obsidian Sync on servers, NAS devices, or Raspberry Pi. Continuous daemon mode keeps your vault in sync without a GUI.
  - title: Headless Publish
    details: Publish your vault to Obsidian Publish from the command line. Automate deployments in CI/CD pipelines.
  - title: E2E Encryption
    details: End-to-end encrypted vaults with V2/V3 encryption. Your data is encrypted before it leaves your device — zero-knowledge.
  - title: Cross-Platform
    details: Available on Linux, macOS, and Windows. Native packages for apt, dnf, pacman, apk; Homebrew Cask for macOS; Winget for Windows.
  - title: Docker Native
    details: Official Docker images on ghcr.io with s6-overlay process supervision. Run sync as a container with environment-based configuration.
  - title: CLI-First
    details: Full Cobra-based CLI with every Obsidian Sync and Publish operation. Automate, script, and integrate into your workflow.
footer: GPL-3.0 Licensed | Copyright © 2024-present Belphemur | [GitHub](https://github.com/Belphemur/obsidian-headless)
---

## Quick Start

```bash
# Install via package manager
brew install --cask obsidian-headless

# Login to your Obsidian account
ob login

# List your remote vaults
ob sync-list-remote

# Setup sync for a vault
ob sync-setup --vault "My Vault" --path /path/to/vault

# Run continuous sync
ob sync --continuous
```

## What is Obsidian Headless Go?

Obsidian Headless Go is the official CLI companion for [Obsidian Sync](https://obsidian.md/sync) and [Obsidian Publish](https://obsidian.md/publish). It brings the full power of Obsidian's sync and publish engines to the command line — no GUI required.

Perfect for:
- **Servers & NAS:** Keep vaults in sync on headless Linux machines
- **CI/CD:** Publish documentation sites automatically on push
- **Docker:** Run sync as a container with environment variables
- **Automation:** Script sync and publish operations in your workflow
