# Sync Protocol Specification

This document describes the WebSocket-based synchronization protocol used by the
Obsidian Sync service. The protocol enables bidirectional file synchronization
between local vaults and the Obsidian cloud.

## Connection

### Transport

- **Protocol**: WebSocket (WSS)
- **URL**: `wss://<host>/` where `<host>` is the vault's assigned sync server
  (e.g., `sync-1.obsidian.md`)
- **Binary type**: `arraybuffer`
- **Security**: Only connections to `*.obsidian.md` and `127.0.0.1` are permitted

### Heartbeat

The client maintains a heartbeat to detect dead connections:

| Parameter | Value |
|-----------|-------|
| Check interval | 20 seconds |
| Send threshold | 10 seconds (send ping if no message received for this long) |
| Connection timeout | 120 seconds (server considers dead after this) |

The client sends `{"op": "ping"}` and expects `{"op": "pong"}` from the server.

## Message Format

All control messages are JSON objects sent as text WebSocket frames.
File content is sent as binary WebSocket frames (ArrayBuffer).

### Operations

| Operation | Direction | Description |
|-----------|-----------|-------------|
| `init` | Client → Server | Authenticate and start sync session |
| `push` | Client → Server | Upload a file/folder/deletion to the server |
| `pull` | Client → Server | Download file content by UID |
| `deleted` | Client → Server | Request list of deleted files |
| `history` | Client → Server | Request version history for a file |
| `restore` | Client → Server | Restore a deleted file |
| `size` | Client → Server | Request vault storage usage |
| `ping` | Client → Server | Heartbeat ping |
| `pong` | Server → Client | Heartbeat response |
| `ready` | Server → Client | Server is ready, provides current version |
| `push` | Server → Client | Server pushes a file change notification |

## Handshake

### Init Request (Client → Server)

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
| `version` | number | Last known sync version (0 for fresh sync) |
| `initial` | boolean | `true` if this is the first sync |
| `device` | string | Human-readable device name |
| `encryption_version` | number | Encryption version (0, 2, or 3) |

### Init Response (Server → Client)

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

### Init Handshake Flow

After the initial `{"res":"ok"}` response, the server performs a **synchronous**
handshake before the connection enters the normal read loop:

1. Client sends `init` request
2. Server responds with `{"res":"ok", ...}` (or error)
3. Server sends **all** existing file records as `push` messages, **including**
   deleted files (`deleted: true`)
4. Server sends `{"op":"ready","version":N}` to signal completion

The client **must** read these messages synchronously; starting a concurrent
reader before the handshake completes will race with the synchronous reads and
cause hangs or lost messages.

## Push Protocol

### Push Request (Client → Server)

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
| `hash` | string | Encrypted content hash (empty string for folders/deletions) |
| `ctime` | number | Creation time in ms since epoch |
| `mtime` | number | Modification time in ms since epoch |
| `folder` | boolean | Whether this is a folder entry |
| `deleted` | boolean | Whether this is a deletion |
| `size` | number | Encrypted content size in bytes (0 for folders/deletions) |
| `pieces` | number | Number of binary chunks to follow (0 for folders/deletions) |

### Push Response

If the server already has the content (deduplication):
```json
{ "res": "ok" }
```

If the server needs the content:
```json
{ "res": "next" }
```

After `"next"`, the client sends binary chunks (max 2 MB each) as binary frames.
After each chunk, the server responds:
```json
{ "res": "next" }
```
or on the last chunk:
```json
{ "res": "ok" }
```

### Server Push Notification (Server → Client)

When any device (including the current client) pushes a change, the server
broadcasts a push notification to **all** connected clients on the vault:

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

**Important**: The server echoes push notifications back to the sender. The
client must detect and ignore these self-echoes to avoid infinite sync loops.
A common approach is to compare the `device` and `mtime` fields with the most
recently pushed file.

### Ready Notification (Server → Client)

Sent after all pending pushes have been delivered:
```json
{
  "op": "ready",
  "version": 150
}
```

The `version` number should be stored and sent in the next `init` to resume from
this point.

## Pull Protocol

### Pull Request (Client → Server)

```json
{
  "op": "pull",
  "uid": 42
}
```

### Pull Response

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

If `deleted` is `true`, no binary data follows.

Otherwise, the server sends `pieces` binary frames containing the encrypted file
content. The client concatenates the chunks and decrypts the result.

### Parallel Downloads

The pull protocol has no request ID field — each `pull` request expects an
immediate response. This means concurrent pulls on a single WebSocket connection
are not possible (responses would interleave).

To parallelize downloads, the Go client opens multiple WebSocket connections,
one per worker goroutine. Each connection completes its own init handshake
independently. Workers then pull files sequentially on their own connection.

See [Parallel Downloads](./parallel-downloads.md) for the full design.

## Other Operations

### List Deleted Files

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

### File History

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

### Vault Size

```json
// Request
{ "op": "size" }

// Response
{ "res": "ok", "size": 52428800 }
```

### Restore

```json
// Request
{ "op": "restore", "uid": 43 }

// Response
{ "res": "ok" }
```

## Binary Data Streaming

File content is streamed as binary WebSocket frames in chunks of up to **2 MB**
(2,097,152 bytes). The number of chunks is communicated in the `pieces` field.

For push operations:
1. Client sends push metadata (JSON)
2. If server responds `{ "res": "next" }`, client streams chunks
3. Each chunk gets a JSON response
4. Final chunk gets `{ "res": "ok" }`

For pull operations:
1. Client sends pull request (JSON)
2. Server responds with metadata including `pieces` count
3. Server sends `pieces` binary frames
4. Client concatenates and decrypts

## Path Encoding

All file paths transmitted over the wire are encrypted using the vault's
encryption provider:

- **V0**: Paths are base64-encoded then AES-GCM encrypted
- **V2/V3**: Paths are AES-SIV encrypted (deterministic, allowing server-side dedup)

The encryption is applied per-path-segment, preserving the `/` directory separator
structure. See [Encryption Protocol](./encryption-protocol.md) for details.

## Error Handling

Server errors use one of two formats:

```json
{ "res": "err", "msg": "Human-readable error message" }
```

or (observed during init):

```json
{ "status": "err", "message": "Human-readable error message" }
```

The client should check both `res` and `status` fields for errors.

The client uses exponential backoff for reconnection:
- **Base delay**: 5 seconds
- **Max delay**: 5 minutes
- **Jitter**: Applied to avoid thundering herd
- **Reset**: On successful connection
