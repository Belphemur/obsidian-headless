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

## Project Structure

- `src/` - TypeScript implementation (legacy)
- `src-go/` - Go implementation (active development)

## Memory Management

When making design decisions, architectural changes, or significant implementation choices, save a memory using the `serena_write_memory` tool. Use descriptive topic paths (e.g., `src-go/logging/log-rotation`).

Before proposing or implementing new design changes, check existing memories with `serena_list_memories` and `serena_read_memory` to ensure consistency with prior decisions.

See `src-go/AGENTS.md` for Go-specific agent configuration.