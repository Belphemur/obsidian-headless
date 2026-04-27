# Mock Server

The mock server provides a local implementation of the Obsidian Sync and Publish
APIs for development and testing. All data is stored in-memory and lost on restart.

## Quick Start

```bash
# Install the ws package (required for WebSocket server)
cd legacy
npm install --save-dev ws

# Run the mock server
node legacy/mock-server/server.mjs
```

## Ports

| Service | Port | URL |
|---------|------|-----|
| HTTP REST API | 3000 | `http://127.0.0.1:3000` |
| WebSocket Sync | 3001 | `ws://127.0.0.1:3001` |

## Pre-seeded Test Data

The server starts with a test user and token:

| Property | Value |
|----------|-------|
| Token | `test-token-12345` |
| Email | `test@example.com` |
| Name | `Test User` |

## Testing with the CLI

### Using the decompiled CLI

```bash
# Build the TypeScript source
cd legacy
npm run build

# Login (creates a new token on the mock server)
node legacy/dist/cli/main.js login --email test@example.com --password any

# Or use the pre-seeded token by writing it to the config
mkdir -p ~/.config/obsidian-headless
echo -n "test-token-12345" > ~/.config/obsidian-headless/auth_token
```

### Using the original minified CLI

```bash
# Login
node legacy/cli.js login --email test@example.com --password any
```

## Supported Endpoints

### REST API

- `POST /user/signin` — Create a new auth token
- `POST /user/signout` — Invalidate a token
- `POST /user/info` — Get user profile
- `POST /vault/regions` — List regions
- `POST /vault/list` — List vaults
- `POST /vault/create` — Create a vault
- `POST /vault/access` — Validate vault access
- `POST /publish/list` — List publish sites
- `POST /publish/create` — Create a publish site
- `POST /api/slug` — Set site slug
- `POST /api/slugs` — Get slug mappings
- `POST /api/list` — List published files
- `POST /api/upload` — Upload a file (raw binary with `obs-token`, `obs-id`, `obs-path`, `obs-hash` headers)
- `POST /api/remove` — Delete a file

### WebSocket Sync

- `init` — Authenticate and start session
- `push` — Upload file/folder/deletion (with binary chunks)
- `pull` — Download file content
- `deleted` — List deleted files
- `history` — Get file version history
- `restore` — Restore a deleted file
- `size` — Get vault storage usage
- `ping`/`pong` — Heartbeat

## Customization

Set the API port via environment variable:

```bash
API_PORT=4000 node legacy/mock-server/server.mjs
```

The WebSocket port is always 3001.

## Limitations

- No actual encryption validation (key hashes are accepted as-is)
- No persistent storage (data lost on restart)
- No rate limiting
- No file size limits enforcement
- Simplified push broadcast (all connected clients on same vault)
