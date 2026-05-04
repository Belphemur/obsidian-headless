---
name: sync-docs
description: Enforce documentation updates when modifying sync logic. Triggers on changes to src/internal/sync/ or sync model types. Must update docs/architecture.md, docs/sync-protocol.md, website/src/architecture/sync-protocol.md, and website/src/usage/sync.md.
license: MIT
allowed-tools: Read
---

# Sync Documentation Maintenance

When modifying any file under `src/internal/sync/` or `src/internal/model/types.go`,
you MUST check and update the following documentation files:

| Change Type | Files to Update |
|------------|-----------------|
| Protocol (push/pull/handshake) | `docs/sync-protocol.md`, `website/src/architecture/sync-protocol.md` |
| Architecture (flow, modules, watcher) | `docs/architecture.md` |
| User behavior (CLI, config) | `website/src/usage/sync.md` |
| Data model (FileRecord fields) | `docs/sync-protocol.md`, `docs/architecture.md` |

## Verification Checklist

Before marking a sync task complete:
- [ ] `docs/architecture.md` reflects new/changed sync behavior
- [ ] `docs/sync-protocol.md` reflects new/changed protocol details
- [ ] `website/src/architecture/sync-protocol.md` mirrors the protocol docs
- [ ] `website/src/usage/sync.md` reflects user-facing behavior changes
