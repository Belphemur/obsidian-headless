---
title: From Source
---

# From Source

## Go Install

```bash
go install github.com/Belphemur/obsidian-headless/src/cmd/ob-go@latest
ln -sf "$(go env GOPATH)/bin/ob-go" "$(go env GOPATH)/bin/ob"
```

## Build From Source

```bash
git clone https://github.com/Belphemur/obsidian-headless.git
cd obsidian-headless/src
go build -o ob ./cmd/ob-go
```
