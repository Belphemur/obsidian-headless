---
title: Docker Installation
---

# Docker

## From GitHub Container Registry

```bash
docker pull ghcr.io/belphemur/obsidian-headless:latest
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
      - obsidian-config:/config
    restart: unless-stopped

volumes:
  obsidian-config:
```

## Docker Authentication

```bash
# Interactive login via Docker
docker run --rm -it --entrypoint get-token ghcr.io/belphemur/obsidian-headless:latest
```
