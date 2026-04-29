---
title: Installation
---

# Installation

Choose your platform:

[[toc]]

## macOS

### Homebrew Cask
```bash
brew install --cask obsidian-headless
```

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

## Linux

### Debian / Ubuntu (deb)
```bash
TAG=$(curl -s https://api.github.com/repos/Belphemur/obsidian-headless/releases/latest | jq -r .tag_name)
curl -LO "https://github.com/Belphemur/obsidian-headless/releases/download/${TAG}/checksums.txt"
curl -LO "https://github.com/Belphemur/obsidian-headless/releases/download/${TAG}/obsidian-headless_linux_amd64.deb"
grep "obsidian-headless_linux_amd64.deb" checksums.txt | sha256sum -c -
sudo dpkg -i obsidian-headless_linux_amd64.deb
```

### Red Hat / Fedora (rpm)
```bash
TAG=$(curl -s https://api.github.com/repos/Belphemur/obsidian-headless/releases/latest | jq -r .tag_name)
curl -LO "https://github.com/Belphemur/obsidian-headless/releases/download/${TAG}/checksums.txt"
curl -LO "https://github.com/Belphemur/obsidian-headless/releases/download/${TAG}/obsidian-headless_linux_amd64.rpm"
grep "obsidian-headless_linux_amd64.rpm" checksums.txt | sha256sum -c -
sudo rpm -i obsidian-headless_linux_amd64.rpm
```

### Arch Linux (pacman / AUR)
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

### Alpine Linux (apk)
```bash
TAG=$(curl -s https://api.github.com/repos/Belphemur/obsidian-headless/releases/latest | jq -r .tag_name)
curl -LO "https://github.com/Belphemur/obsidian-headless/releases/download/${TAG}/checksums.txt"
curl -LO "https://github.com/Belphemur/obsidian-headless/releases/download/${TAG}/obsidian-headless_linux_amd64.apk"
grep "obsidian-headless_linux_amd64.apk" checksums.txt | sha256sum -c -
sudo apk add --allow-untrusted obsidian-headless_linux_amd64.apk
```

### Linux (systemd service)
```bash
# A user systemd service is included in the package
VAULT_PATH="/path/to/vault"
INSTANCE="$(systemd-escape --path "$VAULT_PATH")"
systemctl --user enable "obsidian-headless-sync@${INSTANCE}.service"
systemctl --user start "obsidian-headless-sync@${INSTANCE}.service"
```

## Windows

### Winget
```powershell
winget install Belphemur.ObsidianHeadless
```

## Multi-Platform

### Go Install
```bash
go install github.com/Belphemur/obsidian-headless/src-go/cmd/ob-go@latest
ln -sf "$(go env GOPATH)/bin/ob-go" "$(go env GOPATH)/bin/ob"
```

### From Source
```bash
git clone https://github.com/Belphemur/obsidian-headless.git
cd obsidian-headless/src
go build -o ob ./cmd/ob-go
```

## Docker

### From GitHub Container Registry
```bash
docker pull ghcr.io/belphemur/obsidian-headless:latest
```

### Docker Compose
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
      - obsidian-config:/config
    restart: unless-stopped

volumes:
  obsidian-config:
```

### Docker Authentication
```bash
# Interactive login via Docker
docker run --rm -it --entrypoint get-token ghcr.io/belphemur/obsidian-headless:latest
```

## Verify Installation
```bash
ob --version
ob --help
```

## Next Steps
After installation, proceed to the [Usage Guide](/usage) to learn about all available commands.
