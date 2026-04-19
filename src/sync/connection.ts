/**
 * @module sync/connection
 *
 * Manages the WebSocket connection to the Obsidian Sync server.
 * Handles authentication, heartbeat keep-alive, request/response
 * serialization, and server push notifications.
 */

import { AsyncQueue, createDeferred, type Deferred } from "../utils/async.js";
import { basename, extname } from "../utils/paths.js";
import type { EncryptionProvider } from "../encryption/types.js";

/* ------------------------------------------------------------------ */
/*  Globals                                                            */
/* ------------------------------------------------------------------ */

/** Use the global WebSocket available in Node 22+ / Node 24+. */
const WS = WebSocket;

/** Extract hostname from a WebSocket URL. */
const getHostname = (url: string) => new URL(url).hostname;

/* ------------------------------------------------------------------ */
/*  Constants                                                          */
/* ------------------------------------------------------------------ */

/** Interval between heartbeat checks (ms). */
const HEARTBEAT_INTERVAL = 20_000;

/** Send a ping if no message has been received for this duration (ms). */
const HEARTBEAT_SEND_THRESHOLD = 10_000;

/** Server considers connection dead after this duration of silence (ms). */
const CONNECTION_TIMEOUT = 120_000;

/** Maximum size of a single binary chunk sent to the server (2 MB). */
const CHUNK_SIZE = 2 * 1024 * 1024;

/* ------------------------------------------------------------------ */
/*  WebSocket close codes                                              */
/* ------------------------------------------------------------------ */

const WS_CLOSE_CODES: Record<number, string> = {
  1000: "Disconnected",
  1001: "Going Away",
  1002: "Protocol Error",
  1003: "Unsupported Data",
  1004: "(For future)",
  1005: "No Status Received",
  1006: "Disconnected",
  1007: "Invalid frame payload data",
  1008: "Policy Violation",
  1009: "Message too big",
  1010: "Missing Extension",
  1011: "Internal Error",
  1012: "Service Restart",
  1013: "Try Again Later",
  1014: "Bad Gateway",
  1015: "TLS Handshake",
};

/**
 * Return a human-readable description for a WebSocket close code.
 */
function getCloseCodeDescription(code: number): string {
  if (code >= 0 && code <= 999) return "(Unused)";
  if (code >= 1016) {
    if (code <= 1999) return "(For WebSocket standard)";
    if (code <= 2999) return "(For WebSocket extensions)";
    if (code <= 3999) return "(For libraries and frameworks)";
    if (code <= 4999) return "(For applications)";
  }
  return WS_CLOSE_CODES[code] ?? "(Unknown)";
}

/* ------------------------------------------------------------------ */
/*  Interfaces                                                         */
/* ------------------------------------------------------------------ */

/** A file record pushed from the server or returned by list operations. */
export interface ServerPushFile {
  path: string;
  hash: string;
  ctime: number;
  mtime: number;
  size: number;
  folder: boolean;
  deleted: boolean;
  uid: number;
  device: string;
  user: string;
  /** Set to `true` when this push originated from us in the current session. */
  wasJustPushed?: boolean;
}

/* ------------------------------------------------------------------ */
/*  SyncServerConnection                                               */
/* ------------------------------------------------------------------ */

/**
 * Manages a single WebSocket connection to the Obsidian Sync server.
 *
 * All request/response pairs are serialized through an {@link AsyncQueue}
 * to prevent interleaving. Server push notifications are processed in a
 * separate queue so they do not block request flow.
 */
export class SyncServerConnection {
  /** The active WebSocket connection. */
  socket: WebSocket | null = null;

  /** Serializes all request/response operations. */
  queue = new AsyncQueue();

  /** Processes server push notifications. */
  notifyQueue = new AsyncQueue();

  /** Buffered binary data received from the server. */
  dataQueue: ArrayBuffer[] = [];

  /** Timestamp of the last message received from the server. */
  lastMessageTs = 0;

  /** Timestamp of the last network request we initiated. */
  lastNetworkRequestTs = 0;

  /** Callback invoked when the connection is lost or closed. */
  onDisconnect: (() => void) | null = null;

  /** Maximum file size the server accepts (bytes). */
  perFileMax: number = 199 * 1024 * 1024;

  /** Authenticated user ID, set after successful init handshake. */
  userId: number = -1;

  /** Tracks the most recently pushed file to detect self-echoes. */
  justPushed: {
    path: string;
    folder: boolean;
    deleted: boolean;
    mtime: number;
    hash: string;
  } | null = null;

  /** Encryption provider for path encoding and content encryption. */
  encryptionProvider: EncryptionProvider;

  /** Handle for the heartbeat interval timer. */
  heartbeat: ReturnType<typeof setInterval> | null = null;

  /** Pending response promise for the current request. */
  responsePromise: Deferred<any> | null = null;

  /** Pending binary data promise. */
  dataPromise: Deferred<ArrayBuffer> | null = null;

  /** Callback invoked when the server signals readiness with a version. */
  onReady: ((version: number) => void) | null = null;

  /** Callback invoked when the server pushes a file change notification. */
  onPush: ((file: ServerPushFile) => void) | null = null;

  constructor(encryptionProvider: EncryptionProvider) {
    this.encryptionProvider = encryptionProvider;
  }

  /* ---------------------------------------------------------------- */
  /*  Connection state                                                 */
  /* ---------------------------------------------------------------- */

  /** Whether the socket is currently in the CONNECTING state. */
  isConnecting(): boolean {
    return this.socket?.readyState === WS.CONNECTING;
  }

  /** Whether the socket is currently open and ready for communication. */
  isConnected(): boolean {
    return this.socket?.readyState === WS.OPEN;
  }

  /** Whether a socket instance exists (may be connecting, open, or closing). */
  hasConnection(): boolean {
    return this.socket !== null;
  }

  /* ---------------------------------------------------------------- */
  /*  Connect                                                          */
  /* ---------------------------------------------------------------- */

  /**
   * Establish a WebSocket connection and authenticate with the sync server.
   *
   * @param url - WebSocket server URL (wss://...)
   * @param token - Authentication token
   * @param vaultId - Remote vault identifier
   * @param version - Last known sync version number
   * @param initial - Whether this is the initial sync
   * @param deviceName - Name of this device
   * @param onReady - Callback when server sends "ready" with new version
   * @param onPush - Callback when server pushes a file change
   */
  async connect(
    url: string,
    token: string,
    vaultId: string,
    version: number,
    initial: boolean,
    deviceName: string,
    onReady: (version: number) => void,
    onPush: (file: ServerPushFile) => void,
  ): Promise<void> {
    if (this.isConnected()) return;

    this.onReady = onReady;
    this.onPush = onPush;

    const keyhash = this.encryptionProvider.keyHash;
    const encryption_version = this.encryptionProvider.encryptionVersion;

    return new Promise<void>((resolve, reject) => {
      const socket = new WS(url);

      // Security: only allow connections to Obsidian servers or localhost
      const hostname = getHostname(url);
      if (!hostname.endsWith(".obsidian.md") && hostname !== "127.0.0.1") {
        socket.close();
        reject(new Error(`Refusing to connect to untrusted host: ${hostname}`));
        return;
      }

      this.socket = socket;
      socket.binaryType = "arraybuffer";

      socket.onclose = (event) => {
        if (event.code === 1006) {
          reject(new Error("Unable to connect to server."));
        }
      };

      socket.onopen = () => {
        this.startHeartbeat();
        this.send({
          op: "init",
          token,
          id: vaultId,
          keyhash,
          version,
          initial,
          device: deviceName,
          encryption_version,
        });
      };

      socket.onmessage = (event) => {
        const data =
          typeof event.data === "string" ? JSON.parse(event.data) : null;
        if (!data) {
          reject(new Error("Unexpected binary message during handshake."));
          return;
        }

        // Ignore pong during handshake
        if (data.op === "pong") return;

        // Check for errors
        if (data.status === "err" || data.res === "err") {
          reject(
            new Error(data.msg || data.message || "Authentication failed."),
          );
          return;
        }

        if (data.res !== "ok" && data.status !== "ok") {
          reject(new Error("Unexpected server response during init."));
          return;
        }

        // Validate perFileMax
        if (
          data.max_size !== undefined &&
          Number.isInteger(data.max_size) &&
          data.max_size >= 0
        ) {
          this.perFileMax = data.max_size;
        }

        this.userId = data.user_id ?? -1;
        this.lastMessageTs = Date.now();

        // Switch to normal message handler
        socket.onmessage = (ev) => this.onMessage(ev);
        socket.onclose = () => this.handleDisconnect();

        resolve();
      };
    });
  }

  /* ---------------------------------------------------------------- */
  /*  Disconnect                                                       */
  /* ---------------------------------------------------------------- */

  /** Close the connection and clean up all pending state. */
  disconnect(): void {
    if (this.socket) {
      this.socket.onclose = null;
      this.socket.onmessage = null;
      this.socket.close();
      this.socket = null;
    }

    this.stopHeartbeat();

    if (this.responsePromise) {
      this.responsePromise.reject(new Error("Connection closed."));
      this.responsePromise = null;
    }
    if (this.dataPromise) {
      this.dataPromise.reject(new Error("Connection closed."));
      this.dataPromise = null;
    }

    this.dataQueue = [];

    if (this.onDisconnect) {
      this.onDisconnect();
    }
  }

  /* ---------------------------------------------------------------- */
  /*  Pull                                                             */
  /* ---------------------------------------------------------------- */

  /**
   * Pull file content from the server by UID.
   *
   * @param uid - The unique file identifier on the server
   * @returns Decrypted file content, or `null` if the file was deleted
   */
  async pull(uid: number): Promise<ArrayBuffer | null> {
    return this.queue.queue(async () => {
      const res = await this.request({ op: "pull", uid });

      if (res.deleted) {
        return null;
      }

      const size: number = res.size;
      const pieces: number = res.pieces;

      // Collect binary data chunks
      const chunks: ArrayBuffer[] = [];
      for (let i = 0; i < pieces; i++) {
        const chunk = await this.dataResponse();
        chunks.push(chunk);
      }

      // Concatenate chunks
      let buffer: ArrayBuffer;
      if (chunks.length === 1) {
        buffer = chunks[0];
      } else {
        const totalLength = chunks.reduce((sum, c) => sum + c.byteLength, 0);
        const combined = new Uint8Array(totalLength);
        let offset = 0;
        for (const chunk of chunks) {
          combined.set(new Uint8Array(chunk), offset);
          offset += chunk.byteLength;
        }
        buffer = combined.buffer;
      }

      // Decrypt if there's actual content
      if (buffer.byteLength > 0) {
        buffer = await this.encryptionProvider.decrypt(buffer);
      }

      return buffer;
    });
  }

  /* ---------------------------------------------------------------- */
  /*  Push                                                             */
  /* ---------------------------------------------------------------- */

  /**
   * Push a file (or deletion/folder event) to the server.
   *
   * @param path - Vault-relative file path
   * @param relatedPath - Related path (e.g., rename source)
   * @param isFolder - Whether this entry is a folder
   * @param isDeleted - Whether this is a deletion event
   * @param ctime - Creation time (ms since epoch)
   * @param mtime - Modification time (ms since epoch)
   * @param hash - Content hash of the file
   * @param data - File content (null for folders/deletions)
   */
  async push(
    path: string,
    relatedPath: string | null,
    isFolder: boolean,
    isDeleted: boolean,
    ctime: number,
    mtime: number,
    hash: string,
    data: ArrayBuffer | null,
  ): Promise<void> {
    return this.queue.queue(async () => {
      const encodedPath =
        await this.encryptionProvider.deterministicEncodeStr(path);
      const encodedRelatedPath = relatedPath
        ? await this.encryptionProvider.deterministicEncodeStr(relatedPath)
        : undefined;
      const ext = extname(basename(path));

      // Folder or deletion: no binary data needed
      if (isFolder || isDeleted || !data) {
        this.justPushed = { path, folder: isFolder, deleted: isDeleted, mtime, hash };
        await this.request({
          op: "push",
          path: encodedPath,
          relatedpath: encodedRelatedPath,
          extension: ext,
          hash: "",
          ctime,
          mtime,
          folder: isFolder,
          deleted: isDeleted,
          size: 0,
          pieces: 0,
        });
        this.justPushed = null;
        return;
      }

      // Encrypt content and hash
      const encryptedData = await this.encryptionProvider.encrypt(data);
      const encryptedHash =
        await this.encryptionProvider.deterministicEncodeStr(hash);
      const encryptedSize = encryptedData.byteLength;
      const pieces = Math.ceil(encryptedSize / CHUNK_SIZE);

      this.justPushed = { path, folder: isFolder, deleted: isDeleted, mtime, hash };

      const res = await this.request({
        op: "push",
        path: encodedPath,
        relatedpath: encodedRelatedPath,
        extension: ext,
        hash: encryptedHash,
        ctime,
        mtime,
        folder: isFolder,
        deleted: isDeleted,
        size: encryptedSize,
        pieces,
      });

      // Server already has the content (deduplication)
      if (res.res === "ok" || res.status === "ok") {
        this.justPushed = null;
        return;
      }

      // Stream binary chunks
      for (let i = 0; i < pieces; i++) {
        const start = i * CHUNK_SIZE;
        const end = Math.min(start + CHUNK_SIZE, encryptedSize);
        const chunk = encryptedData.slice(start, end);
        this.sendBinary(chunk);
        await this.response();
      }

      this.justPushed = null;
    });
  }

  /* ---------------------------------------------------------------- */
  /*  List deleted files                                               */
  /* ---------------------------------------------------------------- */

  /**
   * Retrieve the list of deleted files from the server.
   *
   * @returns Array of deleted file records with decrypted paths
   */
  async listDeleted(): Promise<ServerPushFile[]> {
    return this.queue.queue(async () => {
      const res = await this.request({
        op: "deleted",
        suppressrenames: true,
      });

      const items: ServerPushFile[] = res.items || [];
      for (const item of items) {
        item.path = await this.encryptionProvider.deterministicDecodeStr(
          item.path,
        );
      }
      return items;
    });
  }

  /* ---------------------------------------------------------------- */
  /*  History                                                          */
  /* ---------------------------------------------------------------- */

  /**
   * Retrieve the version history for a specific file.
   *
   * @param path - Vault-relative file path
   * @param lastUid - Only return versions after this UID
   * @returns Object containing an array of history items
   */
  async listHistory(
    path: string,
    lastUid: number,
  ): Promise<{ items: ServerPushFile[] }> {
    return this.queue.queue(async () => {
      const encodedPath =
        await this.encryptionProvider.deterministicEncodeStr(path);
      const res = await this.request({
        op: "history",
        path: encodedPath,
        last: lastUid,
      });

      const items: ServerPushFile[] = res.items || [];
      for (const item of items) {
        item.path = await this.encryptionProvider.deterministicDecodeStr(
          item.path,
        );
        if ((item as any).relatedpath) {
          (item as any).relatedpath =
            await this.encryptionProvider.deterministicDecodeStr(
              (item as any).relatedpath,
            );
        }
      }
      return { items };
    });
  }

  /* ---------------------------------------------------------------- */
  /*  Simple operations                                                */
  /* ---------------------------------------------------------------- */

  /** Restore a deleted file by UID. */
  async restore(uid: number): Promise<any> {
    return this.queue.queue(() => this.request({ op: "restore", uid }));
  }

  /** Get all usernames associated with this vault. */
  async getUsernames(): Promise<any> {
    return this.queue.queue(() => this.request({ op: "usernames" }));
  }

  /** Purge all vault data from the server. */
  async purge(): Promise<any> {
    return this.queue.queue(() => this.request({ op: "purge" }));
  }

  /** Get total vault storage size on the server. */
  async size(): Promise<any> {
    return this.queue.queue(() => this.request({ op: "size" }));
  }

  /* ---------------------------------------------------------------- */
  /*  Server push handling                                             */
  /* ---------------------------------------------------------------- */

  /**
   * Handle a server push notification for a file change.
   * Detects self-echoes (files we just pushed) and marks them accordingly.
   */
  onServerPush(msg: any): void {
    const file = msg as ServerPushFile;

    // Detect if this push is an echo of our own operation
    if (this.justPushed) {
      const jp = this.justPushed;
      if (
        jp.folder === file.folder &&
        jp.deleted === file.deleted &&
        jp.mtime === file.mtime
      ) {
        file.wasJustPushed = true;
        this.justPushed = null;
      }
    }

    this.notifyQueue.queue(async () => {
      file.path = await this.encryptionProvider.deterministicDecodeStr(
        file.path,
      );
      if (file.hash) {
        file.hash = await this.encryptionProvider.deterministicDecodeStr(
          file.hash,
        );
      }
      if (this.onPush) {
        this.onPush(file);
      }
    });
  }

  /* ---------------------------------------------------------------- */
  /*  Request/response primitives                                      */
  /* ---------------------------------------------------------------- */

  /**
   * Create a deferred promise for the next JSON response from the server.
   */
  response(): Promise<any> {
    this.responsePromise = createDeferred<any>();
    return this.responsePromise.promise;
  }

  /**
   * Wait for the next binary data message.
   * Returns immediately if data is already buffered.
   */
  dataResponse(): Promise<ArrayBuffer> {
    if (this.dataQueue.length > 0) {
      return Promise.resolve(this.dataQueue.shift()!);
    }
    this.dataPromise = createDeferred<ArrayBuffer>();
    return this.dataPromise.promise;
  }

  /**
   * Send a request and await the response with a timeout.
   *
   * @param msg - Message object to send
   * @param timeout - Maximum time to wait for response (ms)
   * @returns Parsed server response
   */
  async request(msg: Record<string, any>, timeout = 60_000): Promise<any> {
    this.lastNetworkRequestTs = Date.now();
    this.send(msg);

    const responseP = this.response();
    const timeoutP = new Promise<never>((_, reject) => {
      setTimeout(
        () => reject(new Error(`Request timed out after ${timeout}ms`)),
        timeout,
      );
    });

    try {
      return await Promise.race([responseP, timeoutP]);
    } catch (err) {
      this.disconnect();
      throw err;
    }
  }

  /* ---------------------------------------------------------------- */
  /*  Message handler                                                  */
  /* ---------------------------------------------------------------- */

  /**
   * Handle incoming WebSocket messages after the handshake is complete.
   */
  onMessage(event: MessageEvent): void {
    this.lastMessageTs = Date.now();

    if (typeof event.data === "string") {
      const data = JSON.parse(event.data);

      if (data.op === "pong") {
        return;
      }

      if (data.op === "ready") {
        this.notifyQueue.queue(async () => {
          if (this.onReady) {
            this.onReady(data.version);
          }
        });
        return;
      }

      if (data.op === "push") {
        this.onServerPush(data);
        return;
      }

      // Regular response: resolve the pending response promise
      if (this.responsePromise) {
        const p = this.responsePromise;
        this.responsePromise = null;
        p.resolve(data);
      }
    } else {
      // Binary data
      const buffer = event.data as ArrayBuffer;
      if (this.dataPromise) {
        const p = this.dataPromise;
        this.dataPromise = null;
        p.resolve(buffer);
      } else {
        this.dataQueue.push(buffer);
      }
    }
  }

  /* ---------------------------------------------------------------- */
  /*  Send helpers                                                     */
  /* ---------------------------------------------------------------- */

  /** Serialize and send a JSON message. */
  send(msg: Record<string, any>): void {
    if (this.socket && this.isConnected()) {
      this.socket.send(JSON.stringify(msg));
    }
  }

  /** Send raw binary data. */
  sendBinary(data: ArrayBuffer): void {
    if (this.socket && this.isConnected()) {
      this.socket.send(data);
    }
  }

  /* ---------------------------------------------------------------- */
  /*  Heartbeat                                                        */
  /* ---------------------------------------------------------------- */

  /** Start the keep-alive heartbeat interval. */
  private startHeartbeat(): void {
    this.stopHeartbeat();
    this.lastMessageTs = Date.now();

    this.heartbeat = setInterval(() => {
      const now = Date.now();
      const timeSinceLastMessage = now - this.lastMessageTs;

      // Disconnect if server has been silent too long
      if (timeSinceLastMessage >= CONNECTION_TIMEOUT) {
        this.disconnect();
        return;
      }

      // Send a ping if no message received recently
      if (timeSinceLastMessage >= HEARTBEAT_SEND_THRESHOLD) {
        this.send({ op: "ping" });
      }
    }, HEARTBEAT_INTERVAL);
  }

  /** Stop the heartbeat interval. */
  private stopHeartbeat(): void {
    if (this.heartbeat) {
      clearInterval(this.heartbeat);
      this.heartbeat = null;
    }
  }

  /** Handle unexpected disconnection. */
  private handleDisconnect(): void {
    this.socket = null;
    this.stopHeartbeat();

    if (this.responsePromise) {
      this.responsePromise.reject(new Error("Connection lost."));
      this.responsePromise = null;
    }
    if (this.dataPromise) {
      this.dataPromise.reject(new Error("Connection lost."));
      this.dataPromise = null;
    }

    this.dataQueue = [];

    if (this.onDisconnect) {
      this.onDisconnect();
    }
  }
}
