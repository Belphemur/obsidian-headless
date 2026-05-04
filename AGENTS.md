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

When changing sync logic or protocol behavior, the following files MUST be kept in sync:

- `docs/architecture.md` — architecture overview
- `docs/sync-protocol.md` — protocol specification
- `website/src/architecture/sync-protocol.md` — mirrored VuePress docs
- `website/src/usage/sync.md` — user-facing usage docs

Changes to protocol, architecture, or user behavior require updates in all four locations.

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