/**
 * Mock server for the Obsidian Sync and Publish protocols.
 *
 * Usage:
 *   node mock-server/server.mjs
 *
 * Starts:
 *   - HTTP API server on port 3000 (REST endpoints)
 *   - WebSocket server on port 3001 (sync protocol)
 *
 * All data is stored in-memory. Perfect for integration testing.
 */

import http from "node:http";
import { WebSocketServer } from "ws";
import crypto from "node:crypto";

// ---------------------------------------------------------------------------
// In-memory stores
// ---------------------------------------------------------------------------

/** auth tokens → user info */
const tokens = new Map();

/** vault ID → vault object */
const vaults = new Map();

/** vault ID → { uid → file record } */
const vaultFiles = new Map();

/** vault ID → next UID counter */
const vaultUidCounters = new Map();

/** site ID → site object */
const publishSites = new Map();

/** site ID → { path → file entry } */
const publishFiles = new Map();

/** site ID → slug mapping */
const publishSlugs = new Map();

// Pre-seed a test user and token
const TEST_TOKEN = "test-token-12345";
const TEST_USER = {
  uid: "user-1",
  name: "Test User",
  email: "test@example.com",
  mfa: false,
  license: "catalyst",
  credit: 0,
  discount: 0,
};
tokens.set(TEST_TOKEN, TEST_USER);

// ---------------------------------------------------------------------------
// HTTP API server
// ---------------------------------------------------------------------------

function parseBody(req) {
  return new Promise((resolve, reject) => {
    const chunks = [];
    req.on("data", (chunk) => chunks.push(chunk));
    req.on("end", () => {
      try {
        resolve(JSON.parse(Buffer.concat(chunks).toString()));
      } catch {
        resolve({});
      }
    });
    req.on("error", reject);
  });
}

function sendJson(res, data, status = 200) {
  res.writeHead(status, { "Content-Type": "application/json" });
  res.end(JSON.stringify(data));
}

function requireToken(body, res) {
  const user = tokens.get(body.token);
  if (!user) {
    sendJson(res, { error: "Invalid token" }, 401);
    return null;
  }
  return user;
}

const apiServer = http.createServer(async (req, res) => {
  // CORS headers for flexibility
  res.setHeader("Access-Control-Allow-Origin", "*");
  res.setHeader("Access-Control-Allow-Methods", "POST, OPTIONS");
  res.setHeader("Access-Control-Allow-Headers", "Content-Type");

  if (req.method === "OPTIONS") {
    res.writeHead(204);
    res.end();
    return;
  }

  if (req.method !== "POST") {
    sendJson(res, { error: "Method not allowed" }, 405);
    return;
  }

  const body = await parseBody(req);
  const url = req.url;

  console.log(`[API] ${url}`);

  try {
    switch (url) {
      // --- Auth ---
      case "/user/signin": {
        if (!body.email || !body.password) {
          sendJson(res, { error: "Missing email or password" }, 400);
          return;
        }
        const token = `tok-${crypto.randomUUID()}`;
        const user = {
          uid: `user-${Date.now()}`,
          name: body.email.split("@")[0],
          email: body.email,
          mfa: false,
          license: "catalyst",
          credit: 0,
          discount: 0,
        };
        tokens.set(token, user);
        sendJson(res, { token, name: user.name, email: user.email });
        return;
      }

      case "/user/signout": {
        tokens.delete(body.token);
        sendJson(res, { status: "ok" });
        return;
      }

      case "/user/info": {
        const user = requireToken(body, res);
        if (!user) return;
        sendJson(res, user);
        return;
      }

      // --- Vault ---
      case "/vault/regions": {
        const user = requireToken(body, res);
        if (!user) return;
        sendJson(res, {
          regions: [
            { id: "us-east", name: "US East" },
            { id: "eu-west", name: "EU West" },
          ],
        });
        return;
      }

      case "/vault/list": {
        const user = requireToken(body, res);
        if (!user) return;
        const userVaults = [...vaults.values()].filter(
          (v) => v.owner === user.uid,
        );
        sendJson(res, { vaults: userVaults, shared: [] });
        return;
      }

      case "/vault/create": {
        const user = requireToken(body, res);
        if (!user) return;
        const id = `vault-${crypto.randomUUID().slice(0, 8)}`;
        const vault = {
          id,
          uid: id,
          name: body.name || "Untitled",
          password: body.keyhash || "",
          salt: body.salt || "",
          created: Date.now(),
          host: `ws://127.0.0.1:3001`,
          size: 0,
          encryption_version: body.encryption_version ?? 0,
          owner: user.uid,
        };
        vaults.set(id, vault);
        vaultFiles.set(id, new Map());
        vaultUidCounters.set(id, 1);
        sendJson(res, { id, name: vault.name });
        return;
      }

      case "/vault/access": {
        const user = requireToken(body, res);
        if (!user) return;
        // Find vault by vault_uid
        const vaultUid = body.vault_uid;
        const vault = [...vaults.values()].find(
          (v) => v.id === vaultUid || v.uid === vaultUid,
        );
        if (!vault) {
          sendJson(res, { error: "Vault not found" }, 404);
          return;
        }
        // In a real server, validate keyhash. Here we accept any.
        sendJson(res, { status: "ok" });
        return;
      }

      // --- Publish ---
      case "/publish/list": {
        const user = requireToken(body, res);
        if (!user) return;
        const sites = [...publishSites.values()].filter(
          (s) => s.owner === user.uid,
        );
        sendJson(res, { sites, shared: [] });
        return;
      }

      case "/publish/create": {
        const user = requireToken(body, res);
        if (!user) return;
        const id = `site-${crypto.randomUUID().slice(0, 8)}`;
        const site = {
          id,
          slug: "",
          host: `http://127.0.0.1:3000`,
          created: Date.now(),
          owner: user.uid,
        };
        publishSites.set(id, site);
        publishFiles.set(id, new Map());
        sendJson(res, { id, host: site.host });
        return;
      }

      // --- Host API (publish file operations) ---
      case "/api/slug": {
        const user = requireToken(body, res);
        if (!user) return;
        const site = publishSites.get(body.id);
        if (site) {
          site.slug = body.slug;
          publishSlugs.set(body.id, body.slug);
        }
        sendJson(res, { status: "ok" });
        return;
      }

      case "/api/slugs": {
        const user = requireToken(body, res);
        if (!user) return;
        const result = {};
        for (const id of body.ids || []) {
          result[id] = publishSlugs.get(id) ?? "";
        }
        sendJson(res, result);
        return;
      }

      case "/api/list": {
        const user = requireToken(body, res);
        if (!user) return;
        const files = publishFiles.get(body.id);
        const fileList = files
          ? [...files.values()].map((f) => ({
              path: f.path,
              hash: f.hash,
              size: f.size,
            }))
          : [];
        sendJson(res, { files: fileList });
        return;
      }

      case "/api/upload": {
        const token = req.headers["obs-token"];
        const id = req.headers["obs-id"];
        const path = decodeURIComponent(req.headers["obs-path"] || "");
        const hash = req.headers["obs-hash"];
        const user = tokens.get(token);
        if (!user) {
          sendJson(res, { code: "auth", message: "Invalid token" }, 401);
          return;
        }
        const chunks = [];
        for await (const chunk of req) {
          chunks.push(chunk);
        }
        const content = Buffer.concat(chunks);
        const filesMap = publishFiles.get(id);
        if (filesMap) {
          filesMap.set(path, {
            path,
            hash,
            size: content.byteLength,
            content,
          });
        }
        sendJson(res, { status: "ok" });
        return;
      }

      case "/api/remove": {
        const user = requireToken(body, res);
        if (!user) return;
        const fm = publishFiles.get(body.id);
        if (fm) {
          fm.delete(body.path);
        }
        sendJson(res, { status: "ok" });
        return;
      }

      default:
        sendJson(res, { error: `Unknown endpoint: ${url}` }, 404);
    }
  } catch (err) {
    console.error("[API] Error:", err);
    sendJson(res, { error: err.message }, 500);
  }
});

// ---------------------------------------------------------------------------
// WebSocket Sync server
// ---------------------------------------------------------------------------

const wss = new WebSocketServer({ port: 3001 });

wss.on("connection", (ws) => {
  console.log("[WS] New connection");

  let authenticated = false;
  let vaultId = null;
  let userId = -1;
  let deviceName = "unknown";

  /** State for receiving binary push chunks */
  let pendingPush = null;
  let receivedChunks = [];

  ws.on("message", async (data, isBinary) => {
    // Handle binary data (push chunks)
    if (isBinary || data instanceof ArrayBuffer || Buffer.isBuffer(data) && !isJsonBuffer(data)) {
      if (pendingPush) {
        receivedChunks.push(Buffer.from(data));
        if (receivedChunks.length < pendingPush.pieces) {
          ws.send(JSON.stringify({ res: "next" }));
        } else {
          // All chunks received — store the file
          const content = Buffer.concat(receivedChunks);
          const files = vaultFiles.get(vaultId);
          if (files) {
            const uid = vaultUidCounters.get(vaultId) || 1;
            vaultUidCounters.set(vaultId, uid + 1);
            const record = {
              ...pendingPush,
              uid,
              size: content.byteLength,
              content,
            };
            files.set(uid, record);

            // Broadcast to other connections on same vault
            broadcastPush(vaultId, ws, record);
          }
          ws.send(JSON.stringify({ res: "ok" }));
          pendingPush = null;
          receivedChunks = [];
        }
      }
      return;
    }

    // Handle JSON messages
    let msg;
    try {
      msg = JSON.parse(data.toString());
    } catch {
      ws.send(JSON.stringify({ res: "err", msg: "Invalid JSON" }));
      return;
    }

    console.log(`[WS] op=${msg.op}`);

    switch (msg.op) {
      case "init": {
        const user = tokens.get(msg.token);
        if (!user) {
          ws.send(JSON.stringify({ res: "err", msg: "Authentication failed." }));
          ws.close();
          return;
        }
        vaultId = msg.id;
        userId = 1;
        deviceName = msg.device || "unknown";
        authenticated = true;

        // Tag connection with vault info
        ws._vaultId = vaultId;

        ws.send(
          JSON.stringify({
            res: "ok",
            user_id: userId,
            max_size: 200 * 1024 * 1024,
          }),
        );

        // Send all files (including deleted) as pushes, then ready
        const files = vaultFiles.get(vaultId);
        if (files) {
          for (const [, record] of files) {
            ws.send(
              JSON.stringify({
                op: "push",
                path: record.path,
                hash: record.hash || "",
                ctime: record.ctime || Date.now(),
                mtime: record.mtime || Date.now(),
                size: record.size || 0,
                folder: record.folder || false,
                deleted: record.deleted || false,
                uid: record.uid,
                device: record.device || deviceName,
                user: user.email,
              }),
            );
          }
        }

        // Determine version
        const version = files ? files.size : 0;
        ws.send(JSON.stringify({ op: "ready", version }));
        return;
      }

      case "ping": {
        ws.send(JSON.stringify({ op: "pong" }));
        return;
      }

      case "push": {
        if (!authenticated) {
          ws.send(JSON.stringify({ res: "err", msg: "Not authenticated" }));
          return;
        }

        if (msg.pieces > 0 && msg.size > 0) {
          // Need binary data
          pendingPush = {
            path: msg.path,
            relatedpath: msg.relatedpath,
            extension: msg.extension,
            hash: msg.hash,
            ctime: msg.ctime,
            mtime: msg.mtime,
            folder: msg.folder,
            deleted: msg.deleted,
            pieces: msg.pieces,
            device: deviceName,
          };
          receivedChunks = [];
          ws.send(JSON.stringify({ res: "next" }));
        } else {
          // No binary data (folder, deletion, or empty)
          const files = vaultFiles.get(vaultId);
          if (files) {
            const uid = vaultUidCounters.get(vaultId) || 1;
            vaultUidCounters.set(vaultId, uid + 1);
            const record = {
              path: msg.path,
              hash: msg.hash || "",
              ctime: msg.ctime,
              mtime: msg.mtime,
              size: 0,
              folder: msg.folder,
              deleted: msg.deleted,
              uid,
              device: deviceName,
              content: null,
            };
            files.set(uid, record);
            broadcastPush(vaultId, ws, record);
          }
          ws.send(JSON.stringify({ res: "ok" }));
        }
        return;
      }

      case "pull": {
        if (!authenticated) {
          ws.send(JSON.stringify({ res: "err", msg: "Not authenticated" }));
          return;
        }
        const files = vaultFiles.get(vaultId);
        const file = files?.get(msg.uid);

        if (!file) {
          ws.send(
            JSON.stringify({ res: "ok", size: 0, pieces: 0, deleted: true }),
          );
          return;
        }

        if (file.deleted || !file.content) {
          ws.send(
            JSON.stringify({
              res: "ok",
              size: 0,
              pieces: 0,
              deleted: file.deleted ?? true,
            }),
          );
          return;
        }

        const CHUNK_SIZE = 2 * 1024 * 1024;
        const content = file.content;
        const pieces = Math.ceil(content.byteLength / CHUNK_SIZE);

        ws.send(
          JSON.stringify({
            res: "ok",
            size: content.byteLength,
            pieces,
            deleted: false,
          }),
        );

        // Send binary chunks
        for (let i = 0; i < pieces; i++) {
          const start = i * CHUNK_SIZE;
          const end = Math.min(start + CHUNK_SIZE, content.byteLength);
          ws.send(content.subarray(start, end));
        }
        return;
      }

      case "deleted": {
        if (!authenticated) {
          ws.send(JSON.stringify({ res: "err", msg: "Not authenticated" }));
          return;
        }
        const files = vaultFiles.get(vaultId);
        const deletedItems = [];
        if (files) {
          for (const [, record] of files) {
            if (record.deleted) {
              deletedItems.push({
                path: record.path,
                hash: record.hash || "",
                ctime: record.ctime,
                mtime: record.mtime,
                size: 0,
                folder: record.folder,
                deleted: true,
                uid: record.uid,
                device: record.device,
                user: "test@example.com",
              });
            }
          }
        }
        ws.send(JSON.stringify({ res: "ok", items: deletedItems }));
        return;
      }

      case "history": {
        if (!authenticated) {
          ws.send(JSON.stringify({ res: "err", msg: "Not authenticated" }));
          return;
        }
        // Return all versions of the file matching the path
        const files = vaultFiles.get(vaultId);
        const items = [];
        if (files) {
          for (const [, record] of files) {
            if (record.path === msg.path) {
              items.push({
                path: record.path,
                hash: record.hash,
                ctime: record.ctime,
                mtime: record.mtime,
                size: record.size,
                folder: record.folder,
                deleted: record.deleted,
                uid: record.uid,
                device: record.device,
                user: "test@example.com",
              });
            }
          }
        }
        ws.send(JSON.stringify({ res: "ok", items }));
        return;
      }

      case "restore": {
        if (!authenticated) {
          ws.send(JSON.stringify({ res: "err", msg: "Not authenticated" }));
          return;
        }
        const files = vaultFiles.get(vaultId);
        const file = files?.get(msg.uid);
        if (file) {
          file.deleted = false;
        }
        ws.send(JSON.stringify({ res: "ok" }));
        return;
      }

      case "size": {
        if (!authenticated) {
          ws.send(JSON.stringify({ res: "err", msg: "Not authenticated" }));
          return;
        }
        const files = vaultFiles.get(vaultId);
        let totalSize = 0;
        if (files) {
          for (const [, record] of files) {
            totalSize += record.size || 0;
          }
        }
        ws.send(JSON.stringify({ res: "ok", size: totalSize }));
        return;
      }

      default:
        ws.send(JSON.stringify({ res: "err", msg: `Unknown op: ${msg.op}` }));
    }
  });

  ws.on("close", () => {
    console.log("[WS] Connection closed");
  });

  ws.on("error", (err) => {
    console.error("[WS] Error:", err.message);
  });
});

/**
 * Broadcast a push notification to all other WebSocket clients on the same vault.
 */
function broadcastPush(vaultId, sender, record) {
  for (const client of wss.clients) {
    // Production broadcasts to ALL clients on the vault, including the sender
    if (client._vaultId === vaultId && client.readyState === 1) {
      client.send(
        JSON.stringify({
          op: "push",
          path: record.path,
          hash: record.hash || "",
          ctime: record.ctime || Date.now(),
          mtime: record.mtime || Date.now(),
          size: record.size || 0,
          folder: record.folder || false,
          deleted: record.deleted || false,
          uid: record.uid,
          device: record.device || "unknown",
          user: "test@example.com",
        }),
      );
    }
  }
}

/**
 * Heuristic to check if a Buffer looks like JSON (starts with '{' after trimming).
 */
function isJsonBuffer(buf) {
  if (buf.length === 0) return false;
  const first = buf[0];
  // '{' = 123
  return first === 123;
}

// ---------------------------------------------------------------------------
// Start servers
// ---------------------------------------------------------------------------

const API_PORT = process.env.API_PORT || 3000;
const WS_PORT = 3001;

apiServer.listen(API_PORT, () => {
  console.log(`[Mock Server] HTTP API listening on http://127.0.0.1:${API_PORT}`);
  console.log(`[Mock Server] WebSocket Sync listening on ws://127.0.0.1:${WS_PORT}`);
  console.log(`[Mock Server] Pre-seeded test token: ${TEST_TOKEN}`);
  console.log("");
  console.log("Ready for testing.");
});
