---
title: Installation
---

# Installation

Choose your platform:

[[toc]]

## macOS

### Homebrew Cask
```bash
brew tap belphemur/tap
brew install obsidian-headless-go
```

## Linux

### Debian / Ubuntu (deb)
```bash
curl -LO https://github.com/Belphemur/obsidian-headless/releases/latest/download/obsidian-headless-go_amd64.deb
sudo dpkg -i obsidian-headless-go_amd64.deb
```

### Red Hat / Fedora (rpm)
```bash
curl -LO https://github.com/Belphemur/obsidian-headless/releases/latest/download/obsidian-headless-go_amd64.rpm
sudo rpm -i obsidian-headless-go_amd64.rpm
```

### Arch Linux (pacman / AUR)
```bash
# From AUR
yay -S obsidian-headless-go-bin

# Or download .pkg.tar.zst
curl -LO https://github.com/Belphemur/obsidian-headless/releases/latest/download/obsidian-headless-go_amd64.pkg.tar.zst
sudo pacman -U obsidian-headless-go_amd64.pkg.tar.zst
```

### Alpine Linux (apk)
```bash
curl -LO https://github.com/Belphemur/obsidian-headless/releases/latest/download/obsidian-headless-go_amd64.apk
sudo apk add --allow-untrusted obsidian-headless-go_amd64.apk
```

### Linux (systemd service)
```bash
# A user systemd service is included in the package
systemctl --user enable obsidian-headless-sync@<vault-name>
systemctl --user start obsidian-headless-sync@<vault-name>
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
docker run --rm -it ghcr.io/belphemur/obsidian-headless:latest get-token
```

## Verify Installation
```bash
ob --version
ob --help
```

## Next Steps
After installation, proceed to the [Usage Guide](/usage) to learn about all available commands.
