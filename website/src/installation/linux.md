---
title: Linux Installation
---

# Linux

## Manual Download & Checksum Verification

```bash
# Set your platform: linux | darwin | windows
# Set your architecture: amd64 | arm64
OS="linux"
ARCH="amd64"

# Get the latest release tag
TAG=$(curl -s https://api.github.com/repos/Belphemur/obsidian-headless/releases/latest | jq -r .tag_name)

# Download the archive and checksums
curl -LO "https://github.com/Belphemur/obsidian-headless/releases/download/${TAG}/obsidian-headless_${OS}_${ARCH}.tar.gz"
curl -LO "https://github.com/Belphemur/obsidian-headless/releases/download/${TAG}/checksums.txt"

# Verify the checksum
grep "obsidian-headless_${OS}_${ARCH}.tar.gz" checksums.txt | sha256sum -c -

# Extract and install
tar -xzf "obsidian-headless_${OS}_${ARCH}.tar.gz"
sudo mv ob /usr/local/bin/
```

## Debian / Ubuntu (deb)

```bash
TAG=$(curl -s https://api.github.com/repos/Belphemur/obsidian-headless/releases/latest | jq -r .tag_name)
curl -LO "https://github.com/Belphemur/obsidian-headless/releases/download/${TAG}/checksums.txt"
curl -LO "https://github.com/Belphemur/obsidian-headless/releases/download/${TAG}/obsidian-headless_linux_amd64.deb"
grep "obsidian-headless_linux_amd64.deb" checksums.txt | sha256sum -c -
sudo dpkg -i obsidian-headless_linux_amd64.deb
```

## Red Hat / Fedora (rpm)

```bash
TAG=$(curl -s https://api.github.com/repos/Belphemur/obsidian-headless/releases/latest | jq -r .tag_name)
curl -LO "https://github.com/Belphemur/obsidian-headless/releases/download/${TAG}/checksums.txt"
curl -LO "https://github.com/Belphemur/obsidian-headless/releases/download/${TAG}/obsidian-headless_linux_amd64.rpm"
grep "obsidian-headless_linux_amd64.rpm" checksums.txt | sha256sum -c -
sudo rpm -i obsidian-headless_linux_amd64.rpm
```

## Arch Linux (pacman / AUR)

```bash
# From AUR
yay -S obsidian-headless-go-bin

# Or download .pkg.tar.zst manually
TAG=$(curl -s https://api.github.com/repos/Belphemur/obsidian-headless/releases/latest | jq -r .tag_name)
curl -LO "https://github.com/Belphemur/obsidian-headless/releases/download/${TAG}/checksums.txt"
curl -LO "https://github.com/Belphemur/obsidian-headless/releases/download/${TAG}/obsidian-headless_linux_amd64.pkg.tar.zst"
grep "obsidian-headless_linux_amd64.pkg.tar.zst" checksums.txt | sha256sum -c -
sudo pacman -U obsidian-headless_linux_amd64.pkg.tar.zst
```

## Alpine Linux (apk)

```bash
TAG=$(curl -s https://api.github.com/repos/Belphemur/obsidian-headless/releases/latest | jq -r .tag_name)
curl -LO "https://github.com/Belphemur/obsidian-headless/releases/download/${TAG}/checksums.txt"
curl -LO "https://github.com/Belphemur/obsidian-headless/releases/download/${TAG}/obsidian-headless_linux_amd64.apk"
grep "obsidian-headless_linux_amd64.apk" checksums.txt | sha256sum -c -
sudo apk add --allow-untrusted obsidian-headless_linux_amd64.apk
```

## systemd service

```bash
# A user systemd service is included in the package
VAULT_PATH="/path/to/vault"
INSTANCE="$(systemd-escape --path "$VAULT_PATH")"
systemctl --user enable "obsidian-headless-sync@${INSTANCE}.service"
systemctl --user start "obsidian-headless-sync@${INSTANCE}.service"
```
