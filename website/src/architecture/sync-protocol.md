---
title: Sync Protocol
---

# Sync Protocol

## Overview

The Headless Go client's sync engine communicates with Obsidian's cloud servers over a **WebSocket (WSS)** connection. The protocol enables bidirectional file synchronization between a local vault and the Obsidian cloud, supporting encrypted file uploads, downloads, and real-time change notifications.

::: tip Design Philosophy
All control messages are JSON objects sent as **text** WebSocket frames. File content is streamed as **binary** WebSocket frames (`ArrayBuffer`). This separation keeps control logic simple while allowing efficient large-file transfers.
:::

## Connection

### Transport

| Parameter | Value |
|-----------|-------|
| **Protocol** | WebSocket Secure (WSS) |
| **URL** | `wss://<host>/` — where `<host>` is the vault's assigned sync server (e.g., `sync-1.obsidian.md`) |
| **Binary type** | `arraybuffer` |
| **Security** | Production sync uses Obsidian sync hosts. Localhost may be used for local development/testing. The current implementation does not enforce a host allowlist. |

### Heartbeat

To detect dead connections, the client maintains a heartbeat:

| Parameter | Value |
|-----------|-------|
| Check interval | 20 seconds |
| Send threshold | 10 seconds (send ping if no message received) |
| Connection timeout | 120 seconds (server considers dead after this) |

The client sends:

```json
{ "op": "ping" }
```

And expects the server to respond with:

```json
{ "op": "pong" }
```

## Connection Lifecycle

### 1. Init Request (Client → Server)

The client opens the connection and sends an `init` message to authenticate and start the sync session:

```json
{
  "op": "init",
  "token": "<auth-token>",
  "id": "<vault-id>",
  "keyhash": "<key-hash>",
  "version": 0,
  "initial": true,
  "device": "My Device",
  "encryption_version": 3
}
```

| Field | Type | Description |
|-------|------|-------------|
| `op` | string | Always `"init"` |
| `token` | string | Authentication token from login |
| `id` | string | Remote vault identifier |
| `keyhash` | string | Hex-encoded hash of the encryption key |
| `version` | number | Last known sync version (`0` for fresh sync) |
| `initial` | boolean | `true` if this is the first sync |
| `device` | string | Human-readable device name |
| `encryption_version` | number | Encryption version (`0`, `2`, or `3`) |

### 2. Init Response (Server → Client)

On success:

```json
{
  "res": "ok",
  "user_id": 12345,
  "max_size": 208666624
}
```

On error:

```json
{
  "res": "err",
  "msg": "Authentication failed."
}
```

| Field | Type | Description |
|-------|------|-------------|
| `res` | string | `"ok"` or `"err"` |
| `user_id` | number | Authenticated user ID |
| `max_size` | number | Maximum file size in bytes (default: ~199 MB) |
| `msg` | string | Error message (only on error) |

### 3. Synchronous Handshake

After the initial `{"res":"ok"}` response, the server performs a **synchronous** handshake before entering the normal read loop:

1. Client sends `init` request
2. Server responds with `{"res":"ok", ...}` (or error)
3. Server sends **all** existing file records as `push` messages, **including** deleted files (`deleted: true`)
4. Server sends `{"op":"ready","version":N}` to signal completion

::: warning Critical Ordering
The client **must** read these messages synchronously. Starting a concurrent reader before the handshake completes will race with the synchronous reads and cause hangs or lost messages.
:::

### 4. Ready Notification

Once the handshake completes, the server sends:

```json
{
  "op": "ready",
  "version": 150
}
```

The `version` number should be stored and sent in the next `init` to resume from this point.

## Protocol Operations

| Operation | Direction | Description |
|-----------|-----------|-------------|
| `init` | Client → Server | Authenticate and start sync session |
| `push` | Client → Server | Upload a file, folder, or deletion |
| `pull` | Client → Server | Download file content by UID |
| `deleted` | Client → Server | Request list of deleted files |
| `history` | Client → Server | Request version history for a file |
| `restore` | Client → Server | Restore a deleted file |
| `size` | Client → Server | Request vault storage usage |
| `ping` | Client → Server | Heartbeat ping |
| `pong` | Server → Client | Heartbeat response |
| `ready` | Server → Client | Server is ready, provides current version |
| `push` | Server → Client | Server pushes a file change notification |

### Push

#### Push Request (Client → Server)

```json
{
  "op": "push",
  "path": "<encrypted-path>",
  "relatedpath": "<encrypted-related-path>",
  "extension": ".md",
  "hash": "<encrypted-hash>",
  "ctime": 1700000000000,
  "mtime": 1700000000000,
  "folder": false,
  "deleted": false,
  "size": 1024,
  "pieces": 1
}
```

| Field | Type | Description |
|-------|------|-------------|
| `op` | string | Always `"push"` |
| `path` | string | Encrypted vault-relative file path |
| `relatedpath` | string? | Encrypted related path (e.g., rename source) |
| `extension` | string | File extension (e.g., `.md`, `.png`) |
| `hash` | string | Encrypted content hash (empty for folders/deletions) |
| `ctime` | number | Creation time in ms since epoch |
| `mtime` | number | Modification time in ms since epoch |
| `folder` | boolean | Whether this is a folder entry |
| `deleted` | boolean | Whether this is a deletion |
| `size` | number | Encrypted content size in bytes (`0` for folders/deletions) |
| `pieces` | number | Number of binary chunks to follow (`0` for folders/deletions) |

#### Push Response

If the server already has the content (deduplication):

```json
{ "res": "ok" }
```

If the server needs the content:

```json
{ "res": "next" }
```

After `"next"`, the client sends binary chunks (max 2 MB each) as binary frames. After each chunk, the server responds with another `"next"` or, on the final chunk:

```json
{ "res": "ok" }
```

#### Server Push Notification (Server → Client)

When any device pushes a change, the server broadcasts a push notification to **all** connected clients on the vault:

```json
{
  "op": "push",
  "path": "<encrypted-path>",
  "hash": "<encrypted-hash>",
  "ctime": 1700000000000,
  "mtime": 1700000000000,
  "size": 1024,
  "folder": false,
  "deleted": false,
  "uid": 42,
  "device": "Other Device",
  "user": "user@example.com"
}
```

::: warning Avoid Infinite Loops
The server echoes push notifications back to the sender. The client must detect and ignore these self-echoes. A common approach is to compare the `device` and `mtime` fields with the most recently pushed file.
:::

### Pull

#### Pull Request (Client → Server)

```json
{
  "op": "pull",
  "uid": 42
}
```

#### Pull Response

```json
{
  "res": "ok",
  "size": 1024,
  "pieces": 1,
  "deleted": false,
  "hash": "<encrypted-or-raw-hash>"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `res` | string | `"ok"` or `"err"` |
| `size` | number | Decrypted file size in bytes |
| `pieces` | number | Number of binary chunks to follow |
| `deleted` | boolean | Whether the file was deleted |
| `hash` | string | Encrypted content hash (may be raw hex for legacy files) |

If `deleted` is `true`, no binary data follows. Otherwise, the server sends `pieces` binary frames containing the encrypted file content, which the client concatenates and decrypts.

### Deleted Files

Request a list of deleted files:

```json
// Request
{ "op": "deleted", "suppressrenames": true }

// Response
{
  "res": "ok",
  "items": [
    {
      "path": "<encrypted-path>",
      "hash": "...",
      "ctime": 1700000000000,
      "mtime": 1700000000000,
      "size": 0,
      "folder": false,
      "deleted": true,
      "uid": 43,
      "device": "...",
      "user": "..."
    }
  ]
}
```

### History

Request version history for a file:

```json
// Request
{ "op": "history", "path": "<encrypted-path>", "last": 0 }

// Response
{
  "res": "ok",
  "items": [
    {
      "path": "<encrypted-path>",
      "hash": "...",
      "ctime": 1700000000000,
      "mtime": 1700000000000,
      "size": 1024,
      "folder": false,
      "deleted": false,
      "uid": 41,
      "device": "...",
      "user": "..."
    }
  ]
}
```

### Size

Request vault storage usage:

```json
// Request
{ "op": "size" }

// Response
{ "res": "ok", "size": 52428800 }
```

### Restore

Restore a deleted file by UID:

```json
// Request
{ "op": "restore", "uid": 43 }

// Response
{ "res": "ok" }
```

## Binary Data Streaming

File content is streamed as binary WebSocket frames in chunks of up to **2 MB** (2,097,152 bytes). The number of chunks is communicated in the `pieces` field.

### Push Flow

1. Client sends push metadata (JSON)
2. If server responds `{ "res": "next" }`, client streams chunks
3. Each chunk gets a JSON response
4. Final chunk gets `{ "res": "ok" }`

### Pull Flow

1. Client sends pull request (JSON)
2. Server responds with metadata including `pieces` count
3. Server sends `pieces` binary frames
4. Client concatenates and decrypts

## Parallel Downloads

The pull protocol has no request ID field — each `pull` request expects an immediate response. This means concurrent pulls on a single WebSocket connection are not possible because responses would interleave unpredictably.

To parallelize downloads, the sync client opens **multiple WebSocket connections**, one per worker. Each connection completes its own `init` handshake independently. Workers then pull files sequentially on their own connection.

::: tip Performance
Opening multiple connections allows the client to saturate bandwidth when downloading many small files, rather than blocking on each sequential round-trip.
:::

## Version Tracking & Conflict Resolution

### Version Numbers

The server assigns a monotonically increasing `version` number to every change in a vault. When a client connects, it sends the last known version in the `init` request. The server then streams all changes with a higher version during the handshake.

```json
{
  "op": "ready",
  "version": 150
}
```

::: tip Resume Sync
Always persist the `version` returned by `ready`. On reconnection, send it in `init` to resume from the exact point you left off, avoiding a full re-sync.
:::

### Conflict Resolution

The Obsidian sync protocol uses a **last-write-wins** strategy based on `mtime` (modification time). When a client receives a server push notification for a file it has also modified locally, it compares `mtime` values:

- If the server's `mtime` is newer, the client accepts the server version and overwrites the local file.
- If the local `mtime` is newer, the client pushes its local version to the server.

Because the server echoes push notifications, the client must ignore self-echoes (matching `device` and `mtime`) to avoid ping-ponging the same change indefinitely.

## Error Handling

Server errors use one of two formats:

```json
{ "res": "err", "msg": "Human-readable error message" }
```

Or (observed during init):

```json
{ "status": "err", "message": "Human-readable error message" }
```

The client should check both `res` and `status` fields for errors.

### Reconnection Strategy

The client uses exponential backoff for reconnection:

| Parameter | Value |
|-----------|-------|
| Base delay | 5 seconds |
| Max delay | 60 seconds |
| Jitter | Applied to avoid thundering herd |
| Reset | On successful connection |

## Path Encoding

All file paths transmitted over the wire are encrypted using the vault's encryption provider:

- **V0**: Paths are base64-encoded then AES-GCM encrypted
- **V2/V3**: Paths are AES-SIV encrypted (deterministic, allowing server-side dedup)

The encryption is applied to the full path string, so `/` directory separators are encrypted as part of the path rather than preserved on the wire. See [Encryption Protocol](./encryption.md) for details.
