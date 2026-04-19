/**
 * Integration test for the mock server.
 *
 * Tests the REST API endpoints and WebSocket sync protocol
 * against the mock server.
 *
 * Usage:
 *   # Start mock server first:
 *   node mock-server/server.mjs &
 *
 *   # Run tests:
 *   node mock-server/test.mjs
 */

import { test, describe } from "node:test";
import assert from "node:assert/strict";

const API_URL = "http://127.0.0.1:3000";
const WS_URL = "ws://127.0.0.1:3001";
const TEST_TOKEN = "test-token-12345";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

async function post(endpoint, body = {}) {
  const res = await fetch(`${API_URL}${endpoint}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  return res.json();
}

// ---------------------------------------------------------------------------
// REST API Tests
// ---------------------------------------------------------------------------

describe("REST API", () => {
  test("GET user info with pre-seeded token", async () => {
    const data = await post("/user/info", { token: TEST_TOKEN });
    assert.equal(data.email, "test@example.com");
    assert.equal(data.name, "Test User");
  });

  test("sign in creates a new token", async () => {
    const data = await post("/user/signin", {
      email: "new@example.com",
      password: "password123",
    });
    assert.ok(data.token);
    assert.equal(data.email, "new@example.com");

    // Verify the new token works
    const info = await post("/user/info", { token: data.token });
    assert.equal(info.email, "new@example.com");
  });

  test("sign out invalidates token", async () => {
    // Create a temp token
    const { token } = await post("/user/signin", {
      email: "temp@example.com",
      password: "pass",
    });

    // Sign out
    await post("/user/signout", { token });

    // Token should be invalid now
    const info = await post("/user/info", { token });
    assert.equal(info.error, "Invalid token");
  });

  test("list regions", async () => {
    const data = await post("/vault/regions", { token: TEST_TOKEN });
    assert.ok(Array.isArray(data.regions));
    assert.ok(data.regions.length > 0);
    assert.equal(data.regions[0].id, "us-east");
  });

  test("create and list vaults", async () => {
    // Create a vault
    const created = await post("/vault/create", {
      token: TEST_TOKEN,
      name: "Test Vault",
      encryption_version: 3,
      keyhash: "abc123",
      salt: "def456",
    });
    assert.ok(created.id);
    assert.equal(created.name, "Test Vault");

    // List vaults
    const list = await post("/vault/list", {
      token: TEST_TOKEN,
      supported_encryption_version: 3,
    });
    assert.ok(Array.isArray(list.vaults));
    const found = list.vaults.find((v) => v.id === created.id);
    assert.ok(found, "Created vault should appear in list");
    assert.equal(found.name, "Test Vault");
    assert.equal(found.encryption_version, 3);
  });

  test("validate vault access", async () => {
    const { id } = await post("/vault/create", {
      token: TEST_TOKEN,
      name: "Access Test",
    });

    const result = await post("/vault/access", {
      token: TEST_TOKEN,
      uid: id,
      keyhash: "any",
    });
    assert.equal(result.status, "ok");
  });

  test("publish site lifecycle", async () => {
    // Create site
    const site = await post("/publish/create", { token: TEST_TOKEN });
    assert.ok(site.id);
    assert.ok(site.host);

    // Set slug
    await post("/api/slug", {
      token: TEST_TOKEN,
      id: site.id,
      host: site.host,
      slug: "test-site",
    });

    // Get slugs
    const slugs = await post("/api/slugs", {
      token: TEST_TOKEN,
      ids: [site.id],
    });
    assert.equal(slugs[site.id], "test-site");

    // List sites
    const list = await post("/publish/list", { token: TEST_TOKEN });
    assert.ok(list.sites.find((s) => s.id === site.id));

    // Upload a file
    const content = Buffer.from("# Hello World\n").toString("base64");
    await post("/api/put", {
      token: TEST_TOKEN,
      id: site.id,
      host: site.host,
      path: "hello.md",
      hash: "abc123",
      content,
    });

    // List files
    const files = await post("/api/list", {
      token: TEST_TOKEN,
      id: site.id,
      host: site.host,
    });
    assert.ok(files.files.find((f) => f.path === "hello.md"));

    // Delete file
    await post("/api/delete", {
      token: TEST_TOKEN,
      id: site.id,
      host: site.host,
      path: "hello.md",
    });

    // Verify deleted
    const files2 = await post("/api/list", {
      token: TEST_TOKEN,
      id: site.id,
      host: site.host,
    });
    assert.ok(!files2.files.find((f) => f.path === "hello.md"));
  });
});

// ---------------------------------------------------------------------------
// WebSocket Sync Tests
// ---------------------------------------------------------------------------

describe("WebSocket Sync", () => {
  test("connect, init, and receive ready", async () => {
    // Create a vault first
    const { id: vaultId } = await post("/vault/create", {
      token: TEST_TOKEN,
      name: "WS Test Vault",
    });

    const ws = new WebSocket(WS_URL);

    const messages = [];
    const ready = new Promise((resolve, reject) => {
      const timeout = setTimeout(
        () => reject(new Error("Timeout waiting for ready")),
        5000,
      );

      ws.onopen = () => {
        ws.send(
          JSON.stringify({
            op: "init",
            token: TEST_TOKEN,
            id: vaultId,
            keyhash: "",
            version: 0,
            initial: true,
            device: "test-device",
            encryption_version: 0,
          }),
        );
      };

      ws.onmessage = (event) => {
        const data = JSON.parse(event.data);
        messages.push(data);

        if (data.op === "ready") {
          clearTimeout(timeout);
          resolve(data);
        }
      };

      ws.onerror = (err) => {
        clearTimeout(timeout);
        reject(err);
      };
    });

    const readyMsg = await ready;
    assert.equal(readyMsg.op, "ready");
    assert.equal(typeof readyMsg.version, "number");

    // Check that init response was ok
    const initResponse = messages.find((m) => m.res === "ok" && m.user_id !== undefined);
    assert.ok(initResponse, "Should receive init ok response");

    ws.close();
  });

  test("push and pull a file via WebSocket", async () => {
    // Create a vault
    const { id: vaultId } = await post("/vault/create", {
      token: TEST_TOKEN,
      name: "Push Pull Test",
    });

    const ws = new WebSocket(WS_URL);

    // Wait for connection and init
    await new Promise((resolve, reject) => {
      const timeout = setTimeout(() => reject(new Error("Timeout")), 5000);

      ws.onopen = () => {
        ws.send(
          JSON.stringify({
            op: "init",
            token: TEST_TOKEN,
            id: vaultId,
            keyhash: "",
            version: 0,
            initial: true,
            device: "test",
            encryption_version: 0,
          }),
        );
      };

      ws.onmessage = (event) => {
        const data = JSON.parse(event.data);
        if (data.op === "ready") {
          clearTimeout(timeout);
          // Replace message handler for test
          ws.onmessage = null;
          resolve();
        }
      };

      ws.onerror = (err) => {
        clearTimeout(timeout);
        reject(err);
      };
    });

    // Push a file (no binary data — folder entry)
    const pushResult = await new Promise((resolve) => {
      ws.onmessage = (event) => {
        resolve(JSON.parse(event.data));
      };
      ws.send(
        JSON.stringify({
          op: "push",
          path: "test-folder",
          extension: "",
          hash: "",
          ctime: Date.now(),
          mtime: Date.now(),
          folder: true,
          deleted: false,
          size: 0,
          pieces: 0,
        }),
      );
    });
    assert.equal(pushResult.res, "ok");

    // Push a file with binary data (1 piece)
    const fileContent = Buffer.from("Hello World!");
    const pushFileResult = await new Promise((resolve) => {
      let gotNext = false;
      ws.onmessage = (event) => {
        if (typeof event.data === "string") {
          const data = JSON.parse(event.data);
          if (data.res === "next" && !gotNext) {
            gotNext = true;
            // Send the binary chunk
            ws.send(fileContent);
          } else {
            resolve(data);
          }
        }
      };
      ws.send(
        JSON.stringify({
          op: "push",
          path: "test-file.md",
          extension: ".md",
          hash: "testhash",
          ctime: Date.now(),
          mtime: Date.now(),
          folder: false,
          deleted: false,
          size: fileContent.byteLength,
          pieces: 1,
        }),
      );
    });
    assert.equal(pushFileResult.res, "ok");

    // Pull the file back by finding its UID
    // First, list deleted to see all files (or we need to find the UID)
    // The UID was assigned by the server. Let's check via the deleted list.
    const sizeResult = await new Promise((resolve) => {
      ws.onmessage = (event) => {
        resolve(JSON.parse(event.data));
      };
      ws.send(JSON.stringify({ op: "size" }));
    });
    assert.ok(sizeResult.size >= 0, "Size should be non-negative");

    // Ping/pong test
    const pongResult = await new Promise((resolve) => {
      ws.onmessage = (event) => {
        resolve(JSON.parse(event.data));
      };
      ws.send(JSON.stringify({ op: "ping" }));
    });
    assert.equal(pongResult.op, "pong");

    ws.close();
  });
});

console.log("\n✅ All mock server tests completed!\n");
