# Project Agent Configuration

This project uses [Conventional Commits](https://www.conventionalcommits.org/) for commit messages.

## Commit Message Format

```
<type>(<scope>): <description>

[optional body]

[optional footer(s)]
```

### Types

- `feat`: A new feature
- `fix`: A bug fix
- `docs`: Documentation only changes
- `style`: Changes that do not affect the meaning of the code (formatting)
- `refactor`: Code change that neither fixes a bug nor adds a feature
- `perf`: A code change that improves performance
- `test`: Adding or updating tests
- `chore`: Changes to build process or auxiliary tools
- `ci`: Changes to CI configuration
- `build`: Changes to build system or dependencies

### Examples

```
feat(sync): add WebSocket-based real-time sync implementation
fix(publish): handle missing publish flag in frontmatter correctly
docs(readme): update installation instructions
chore: update go.mod to use Go 1.26
```

## Documentation

### `docs/` — Technical Reference

The `docs/` directory is the **canonical source of truth** for deep technical documentation — protocol specs, architecture design, and implementation details. These are plain Markdown files aimed at developers. They are exhaustive and authoritative.

### `website/` — User-Facing Documentation

The `website/src/architecture/` and `website/src/usage/` directories contain **curated, user-facing versions** that mirror or complement `docs/`. They use VuePress conventions (frontmatter, Mermaid diagrams, callout containers) and target both users and developers. See `website/AGENTS.md` for detailed VuePress conventions.

### Relationship Between `docs/` and `website/`

- `docs/` is the **authoritative technical reference**
- `website/` pages are **curated subsets** with diagrams, tables, and callouts
- **Do NOT duplicate large text blocks** between them — link or summarize instead
- When `docs/` changes, check if corresponding `website/` pages need updating

### Using the `scribe` Agent for Documentation

Delegate documentation work to the `scribe` agent for **any** updates to `docs/` or `website/`:

- Use `task` to delegate (scribe is write-capable)
- Scribe understands both plain Markdown and VuePress conventions (frontmatter, Mermaid, callouts)
- Use scribe for: creating new doc pages, updating existing docs, ensuring cross-file consistency

### Files to Keep in Sync

When changing sync logic or protocol behavior, the following files MUST be kept in sync:

- `docs/architecture.md` — canonical architecture overview
- `docs/sync-protocol.md` — canonical protocol specification
- `docs/encryption-protocol.md` — canonical encryption details
- `docs/circuit-breaker.md` — circuit breaker implementation docs
- `docs/parallel-downloads.md` — parallel download internals; update when sync execution internals change
- `website/src/architecture/sync-protocol.md` — curated VuePress version
- `website/src/usage/sync.md` — user-facing usage docs

Changes to protocol, architecture, or user behavior require updates in all relevant `docs/` files and their corresponding `website/` pages.

## Commit Frequency

Always commit along the way. Make multiple small, focused commits rather than one large monolithic commit. Each logical change gets its own commit. Commit messages follow Conventional Commits format with a clear scope.

- Commit after each bug fix, new feature, documentation update, or refactor step
- Keep commits small and reviewable — each commit should tell a clean story
- Never leave uncommitted changes at the end of a session

## Project Structure

- `legacy/` - TypeScript implementation (legacy)
- `src/` - Go implementation (active development)

### Source Code Location

All active source code lives in the `src/` directory. The Go module root is at `src/go.mod` and the main entry point is `src/cmd/ob-go/main.go`. See `src/AGENTS.md` for Go-specific agent configuration.

## Memory Management

When making design decisions, architectural changes, or significant implementation choices, save a memory using the `serena_write_memory` tool. Use descriptive topic paths (e.g., `src/logging/log-rotation`).

Before proposing or implementing new design changes, check existing memories with `serena_list_memories` and `serena_read_memory` to ensure consistency with prior decisions.

See `src/AGENTS.md` for Go-specific agent configuration.