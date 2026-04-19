/**
 * @module sync/engine
 *
 * The main sync engine that coordinates all synchronization operations between
 * the local vault file system and the remote Obsidian Sync server.
 *
 * This module implements bidirectional file synchronization with support for:
 * - Three-way merge conflict resolution for Markdown files
 * - JSON object-key merging for configuration files
 * - Exponential backoff for reconnection attempts
 * - Pull-only, mirror-remote, and bidirectional sync modes
 * - Debounced uploads to avoid syncing partially-written files
 * - Configurable conflict strategies (merge vs. conflicted copy)
 *
 * @example
 * ```ts
 * const engine = new SyncEngine({
 *   config: myConfig,
 *   token: authToken,
 *   encryption: encryptionProvider,
 *   continuous: true,
 * });
 * await engine.sync();
 * ```
 */

import fs from "node:fs";
import path from "node:path";
import url from "node:url";
import { SyncServerConnection, type ServerPushFile } from "./connection.js";
import { ExponentialBackoff } from "./backoff.js";
import { SyncFilter } from "./filter.js";
import { threeWayMerge } from "./merge.js";
import { StateStore, type FileRecord } from "../storage/state-store.js";
import { FileSystemAdapter } from "../fs/adapter.js";
import type { EncryptionProvider } from "../encryption/types.js";
import type { SyncConfig } from "../config/index.js";
import { sha256Hex } from "../utils/crypto.js";
import { bufferToString, stringToBuffer } from "../utils/encoding.js";
import {
  extname,
  basename,
  dirname,
  isValidFilename,
  normalizePath,
  sanitizeFilename,
  isHiddenPath,
} from "../utils/paths.js";
import { formatBytes } from "../utils/format.js";
import { sleep } from "../utils/async.js";
import { getStatePath } from "../config/index.js";

/* ------------------------------------------------------------------ */
/*  Constants                                                          */
/* ------------------------------------------------------------------ */

/** Minimum milliseconds between consecutive network sync requests. */
const SYNC_RATE_LIMIT_MS = 50;

/** Sync interval for periodic sync checks (ms). */
const SYNC_INTERVAL_MS = 30_000;

/** Debounce thresholds for recently-modified files (ms). */
const DEBOUNCE_SMALL = 10_000;
const DEBOUNCE_MEDIUM = 20_000;
const DEBOUNCE_LARGE = 30_000;

/** Size thresholds for debounce classification (bytes). */
const SIZE_SMALL = 10 * 1024;
const SIZE_MEDIUM = 100 * 1024;

/** Max retry delay for failed individual file syncs (ms). */
const MAX_FILE_RETRY_MS = 5 * 60 * 1000;

/** Time threshold below which a file is considered "recently created" (ms). */
const RECENTLY_CREATED_THRESHOLD_MS = 3 * 60 * 1000;

/* ------------------------------------------------------------------ */
/*  Types                                                              */
/* ------------------------------------------------------------------ */

/**
 * Options for constructing a {@link SyncEngine} instance.
 */
export interface SyncEngineOptions {
  /** Vault sync configuration containing paths, host, encryption settings. */
  config: SyncConfig;
  /** Authentication token for the sync server. */
  token: string;
  /** Encryption provider for encrypting/decrypting file content and paths. */
  encryption: EncryptionProvider;
  /** Whether to run in continuous (watch) mode or one-shot sync. */
  continuous?: boolean;
}

/* ------------------------------------------------------------------ */
/*  SyncEngine                                                         */
/* ------------------------------------------------------------------ */

/**
 * Coordinates bidirectional synchronization between a local vault and
 * the Obsidian Sync server.
 *
 * The engine manages the full sync lifecycle:
 * 1. Connects to the server with exponential backoff on failure
 * 2. Receives push notifications for remote changes
 * 3. Downloads new/modified remote files with conflict resolution
 * 4. Uploads new/modified local files respecting size and rate limits
 * 5. Handles deletions, renames, and folder operations
 *
 * Conflict resolution strategies:
 * - **merge**: Three-way merge using diff-match-patch for .md files
 * - **conflict**: Creates "Conflicted copy" files with device name + timestamp
 * - **JSON merge**: Object-key merging for .json config files (server wins)
 *
 * Sync modes:
 * - **""** (bidirectional): Full two-way sync (default)
 * - **"pull"**: Only download changes, never upload
 * - **"mirror"**: Mirror the remote state exactly (restore missing, delete extra)
 */
export class SyncEngine {
  /** Active server connection, or null if disconnected. */
  server: SyncServerConnection | null = null;

  /** Local file state indexed by vault-relative path. */
  localFiles: Record<string, FileRecord> = {};

  /** Server file state indexed by vault-relative path. */
  serverFiles: Record<string, FileRecord> = {};

  /** Queue of pending server file changes awaiting processing. */
  newServerFiles: FileRecord[] = [];

  /** Current sync version counter from the server. */
  version = 0;

  /** Whether we are in initial sync (first full download). */
  initial = true;

  /** Whether the server has signalled readiness. */
  ready = false;

  /** Whether a sync operation is currently in progress. */
  syncing = false;

  /** Whether the engine has completed its first full load cycle. */
  loaded = false;

  /** Backoff controller for server reconnection (0ms min, 5min max, 5s base). */
  backoff = new ExponentialBackoff(0, 5 * 60 * 1000, 5000, true);

  /** Per-file retry tracking for failed sync operations. */
  fileRetry: Record<string, { count: number; ts: number }> = {};

  /** Files skipped during sync with a reason string. */
  skippedFiles: Record<string, string> = {};

  /** Path currently being synced (for re-entrancy detection). */
  syncingPath = "";

  /** Whether the config directory needs re-scanning. */
  scanSpecialFiles = false;

  /** Queue of special file paths discovered during config-dir scanning. */
  scanSpecialFileQueue: string[] = [];

  /** Resolver function for the stop signal promise. */
  resolveStop: (() => void) | null = null;

  /** Sync server hostname. */
  private host: string;

  /** Authentication token. */
  private token: string;

  /** Remote vault identifier. */
  private vaultId: string;

  /** Whether to keep running after initial sync completes. */
  private continuous: boolean;

  /** Encryption provider for content and path encoding. */
  private encryption: EncryptionProvider;

  /** SQLite state store for persisting sync metadata. */
  private stateStore: StateStore;

  /** Conflict resolution strategy: "merge" or "conflict". */
  private conflictStrategy: string;

  /** Sync mode: "" (bidirectional), "pull", or "mirror". */
  private syncMode: string;

  /** Device name used in conflict copies and server identification. */
  private deviceName: string;

  /** File system adapter for vault I/O operations. */
  private adapter: FileSystemAdapter;

  /** Filter for determining which files should be synced. */
  private filter: SyncFilter;

  /** Handle for the periodic sync interval timer. */
  private syncInterval: ReturnType<typeof setInterval> | null = null;

  /** Debounced sync request to coalesce rapid triggers. */
  private debouncedRequestSync: ReturnType<typeof setTimeout> | null = null;

  /**
   * Create a new sync engine.
   *
   * Initializes the state store, file system adapter, sync filter, and
   * loads persisted state from the SQLite database.
   *
   * @param options - Engine configuration options.
   */
  constructor(options: SyncEngineOptions) {
    const { config, token, encryption, continuous = false } = options;

    this.host = config.host;
    this.token = token;
    this.vaultId = config.vaultId;
    this.continuous = continuous;
    this.encryption = encryption;

    // Initialize SQLite state store
    this.stateStore = new StateStore(getStatePath(config.vaultId));

    // Configuration with defaults
    this.conflictStrategy = config.conflictStrategy || "merge";
    this.syncMode = config.syncMode || "";
    this.deviceName = config.deviceName || "Sync Client";

    const configDir = config.configDir || ".obsidian";

    // Initialize file system adapter
    this.adapter = new FileSystemAdapter(
      fs,
      path,
      url,
      () => false, // no-op trash function
      null, // no native btime setter
      "file:///",
      config.vaultPath,
    );

    // Initialize sync filter
    this.filter = new SyncFilter(configDir);
    this.filter.init(
      config.allowTypes,
      config.allowSpecialFiles,
      config.ignoreFolders,
    );

    // Load persisted state
    this.loadState();
  }

  /* ---------------------------------------------------------------- */
  /*  State persistence                                                */
  /* ---------------------------------------------------------------- */

  /**
   * Load persisted sync state from the SQLite database.
   *
   * Restores the version counter, initial sync flag, local file records,
   * server file records, and any pending (unprocessed) server file changes.
   */
  private loadState(): void {
    this.version = this.stateStore.getVersion();
    this.initial = this.stateStore.isInitial();
    this.localFiles = this.stateStore.getAllLocalFileRecords();
    this.serverFiles = this.stateStore.getAllServerFileRecords();

    // Restore pending server files that weren't processed before shutdown
    const pending = this.stateStore.getPendingFileRecords();
    this.newServerFiles = pending;
  }

  /* ---------------------------------------------------------------- */
  /*  Server push handling                                             */
  /* ---------------------------------------------------------------- */

  /**
   * Handle a file change pushed from the server.
   *
   * Updates the sync version, creates a FileRecord from the push data,
   * and either accepts a self-echo (wasJustPushed) immediately or queues
   * the change for processing during the next sync cycle.
   *
   * @param pushFile - The server push notification containing file metadata.
   */
  handlePush(pushFile: ServerPushFile): void {
    // Update version from server
    if (pushFile.uid > this.version) {
      this.version = pushFile.uid;
    }

    // Skip deletions during initial sync (we'll get full state)
    if (this.initial && pushFile.deleted) {
      return;
    }

    // Create FileRecord from push data
    const record: FileRecord = {
      path: pushFile.path,
      hash: pushFile.hash,
      ctime: pushFile.ctime,
      mtime: pushFile.mtime,
      size: pushFile.size,
      folder: pushFile.folder,
      deleted: pushFile.deleted,
      uid: pushFile.uid,
      device: pushFile.device,
      user: pushFile.user,
      initial: this.initial ? true : undefined,
    };

    // Self-echo: this push originated from us in this session
    if (pushFile.wasJustPushed) {
      // Remove from pending queue if present
      this.newServerFiles = this.newServerFiles.filter(
        (f) => f.path !== record.path,
      );
      this.stateStore.deletePendingFileRecord(record.path);

      // Update server state directly
      this.serverFiles[record.path] = record;
      this.stateStore.setServerFileRecord(record);
      return;
    }

    // Queue for processing
    this.newServerFiles.push(record);
    this.stateStore.addPendingFileRecord(record);
    this.stateStore.setVersion(this.version);

    this.log(`[push] ${record.deleted ? "deleted" : "changed"}: ${record.path}`);
    this.scheduleSync();
  }

  /* ---------------------------------------------------------------- */
  /*  Main sync lifecycle                                              */
  /* ---------------------------------------------------------------- */

  /**
   * Start the sync engine.
   *
   * In continuous mode, this sets up file watching and a periodic sync
   * interval, then blocks until {@link stop} is called. In one-shot mode,
   * performs a single sync cycle and returns.
   *
   * @returns A promise that resolves when sync is stopped or completes.
   */
  async sync(): Promise<void> {
    const stopPromise = new Promise<void>((resolve) => {
      this.resolveStop = resolve;
    });

    // First-time initialization
    if (!this.loaded) {
      // Start watching the file system for changes
      await this.adapter.watch((event, filePath, oldPath, stat) => {
        this.onChange(event, filePath, oldPath, stat);
      });

      // Clean up stale local file entries that no longer exist on disk
      for (const filePath of Object.keys(this.localFiles)) {
        if (!this.filter.allowSyncFile(filePath, this.localFiles[filePath].folder)) {
          delete this.localFiles[filePath];
          this.stateStore.deleteLocalFileRecord(filePath);
        }
      }

      this.scanSpecialFiles = true;
      this.loaded = true;
    }

    // Periodic sync interval
    this.syncInterval = setInterval(() => {
      this.scheduleSync();
    }, SYNC_INTERVAL_MS);

    // Trigger the first sync
    await this.requestSync();

    if (this.continuous) {
      await stopPromise;
    }
  }

  /**
   * Stop the sync engine gracefully.
   *
   * Disconnects from the server, closes the state store, stops file
   * watching, cancels pending debounced operations, and resolves the
   * stop promise to unblock {@link sync}.
   */
  stop(): void {
    // Disconnect from server
    if (this.server) {
      this.server.disconnect();
      this.server = null;
    }

    // Close state database
    this.stateStore.close();

    // Stop file system watching
    this.adapter.stopWatch();

    // Cancel periodic sync
    if (this.syncInterval) {
      clearInterval(this.syncInterval);
      this.syncInterval = null;
    }

    // Cancel debounced sync request
    if (this.debouncedRequestSync) {
      clearTimeout(this.debouncedRequestSync);
      this.debouncedRequestSync = null;
    }

    // Signal completion
    if (this.resolveStop) {
      this.resolveStop();
      this.resolveStop = null;
    }
  }

  /* ---------------------------------------------------------------- */
  /*  Sync request coordination                                        */
  /* ---------------------------------------------------------------- */

  /**
   * Schedule a sync request via debouncing to coalesce rapid triggers.
   * @internal
   */
  private scheduleSync(): void {
    if (this.debouncedRequestSync) return;
    this.debouncedRequestSync = setTimeout(() => {
      this.debouncedRequestSync = null;
      this.requestSync();
    }, 100);
  }

  /**
   * Execute the sync loop if not already running.
   *
   * Repeatedly calls {@link _sync} until no more work remains, with a
   * minimum rate limit between iterations to avoid overwhelming the server.
   * Handles errors by logging and applying backoff (or throwing in one-shot mode).
   */
  async requestSync(): Promise<void> {
    if (this.syncing) return;
    this.syncing = true;

    try {
      let hasMore = true;
      while (hasMore) {
        try {
          hasMore = await this._sync();
        } catch (err) {
          const message = err instanceof Error ? err.message : String(err);
          this.log(`[error] Sync failed: ${message}`);
          this.failedSync("");

          if (!this.continuous) {
            throw err;
          }
          hasMore = false;
        }

        // Rate limit: ensure minimum gap between network requests
        if (hasMore) {
          await sleep(SYNC_RATE_LIMIT_MS);
        }
      }
    } finally {
      this.syncing = false;
    }
  }

  /* ---------------------------------------------------------------- */
  /*  Rate limiting & retry helpers                                     */
  /* ---------------------------------------------------------------- */

  /**
   * Check whether a file at `filePath` is eligible for sync at the given timestamp.
   *
   * Consults the per-file retry map: if a file has failed recently and its
   * retry timeout hasn't elapsed, blocks sync for that path and any sub-paths.
   *
   * @param timestamp - Current time in milliseconds.
   * @param filePath  - Vault-relative path to check.
   * @returns `true` if sync is allowed, `false` if blocked by retry.
   */
  canSyncPath(timestamp: number, filePath: string): boolean {
    for (const [retryPath, entry] of Object.entries(this.fileRetry)) {
      if (timestamp < entry.ts && filePath.startsWith(retryPath)) {
        return false;
      }
    }
    return true;
  }

  /**
   * Check whether a local file record is eligible for upload.
   *
   * Applies debounce rules based on file size to avoid syncing
   * partially-written files:
   * - Files < 10 KB: wait 10 seconds after last modification
   * - Files 10-100 KB: wait 20 seconds
   * - Files > 100 KB: wait 30 seconds
   *
   * @param timestamp - Current time in milliseconds.
   * @param record    - The local file record to evaluate.
   * @returns `true` if enough time has elapsed since last modification.
   */
  canSyncLocalFile(timestamp: number, record: FileRecord): boolean {
    if (record.folder) return true;

    let debounceMs: number;
    if (record.size < SIZE_SMALL) {
      debounceMs = DEBOUNCE_SMALL;
    } else if (record.size < SIZE_MEDIUM) {
      debounceMs = DEBOUNCE_MEDIUM;
    } else {
      debounceMs = DEBOUNCE_LARGE;
    }

    return timestamp - record.mtime >= debounceMs;
  }

  /**
   * Record a failed sync attempt for a path and compute the next retry time.
   *
   * Uses exponential backoff per file: `2^count * 5000` ms, capped at 5 minutes.
   *
   * @param filePath - The vault-relative path that failed.
   */
  failedSync(filePath: string): void {
    const entry = this.fileRetry[filePath] || { count: 0, ts: 0 };
    entry.count++;
    const delay = Math.min(MAX_FILE_RETRY_MS, Math.pow(2, entry.count) * 5000);
    entry.ts = Date.now() + delay;
    this.fileRetry[filePath] = entry;
  }

  /* ---------------------------------------------------------------- */
  /*  Server connection management                                     */
  /* ---------------------------------------------------------------- */

  /**
   * Get or establish a server connection with backoff management.
   *
   * Handles:
   * - Reconnecting stale/disconnected sockets
   * - Respecting backoff timing between connection attempts
   * - Creating new connections with appropriate callbacks
   * - Fatal error handling (subscription expired, vault not found)
   *
   * @returns The active server connection, or `null` if not ready to connect.
   * @throws On fatal authentication/subscription errors.
   */
  async getServer(): Promise<SyncServerConnection | null> {
    // Check existing connection health
    if (this.server && !this.server.isConnected() && !this.server.isConnecting()) {
      this.server.disconnect();
      this.server = null;
      this.backoff.fail();
    }

    // Respect backoff timing
    if (!this.server && !this.backoff.isReady()) {
      this.log("[connect] Waiting for backoff...");
      return null;
    }

    // Create new connection if needed
    if (!this.server) {
      const conn = new SyncServerConnection(this.encryption);

      if (!conn.isConnected()) {
        const protocol = this.host.includes("127.0.0.1") ? "ws://" : "wss://";
        const wsUrl = `${protocol}${this.host}`;

        try {
          await conn.connect(
            wsUrl,
            this.token,
            this.vaultId,
            this.version,
            this.initial,
            this.deviceName,
            // onReady callback
            (serverVersion: number) => {
              this.ready = true;
              if (this.initial) {
                this.initial = false;
                this.stateStore.setInitial(false);
              }
              if (serverVersion > this.version) {
                this.version = serverVersion;
                this.stateStore.setVersion(this.version);
              }
              this.scheduleSync();
            },
            // onPush callback
            (file: ServerPushFile) => {
              this.handlePush(file);
            },
          );

          // Set disconnect handler
          conn.onDisconnect = () => {
            this.log("[connect] Disconnected from server");
            if (this.continuous) {
              this.scheduleSync();
            }
          };

          this.server = conn;
          this.backoff.success();
        } catch (err) {
          const message = err instanceof Error ? err.message : String(err);

          // Fatal errors that should stop the engine
          if (
            message.includes("subscription expired") ||
            message.includes("vault not found")
          ) {
            this.stop();
            throw err;
          }

          // Transient error: apply backoff
          conn.disconnect();
          this.backoff.fail();
          this.log(`[connect] Connection failed: ${message}`);
          return null;
        }
      }
    }

    return this.server;
  }

  /* ---------------------------------------------------------------- */
  /*  Core sync loop                                                   */
  /* ---------------------------------------------------------------- */

  /**
   * Execute a single sync iteration.
   *
   * This is the heart of the sync engine. Each call processes pending
   * server changes, uploads local modifications, and handles deletions.
   *
   * **Algorithm overview:**
   *
   * 1. Establish/verify server connection
   * 2. Scan config directory for special files (if flagged)
   * 3. Process scan queue (update local file state from disk)
   * 4. Process pending server files (download, merge, or conflict):
   *    - Self-echo detection (skip files we just pushed)
   *    - Filter/validation checks
   *    - Hash comparison for no-op detection
   *    - Three-way merge for concurrent .md edits
   *    - JSON key-merge for .json config files
   *    - Conflicted copy creation when merge is impossible
   * 5. Handle mirror-remote mode (restore missing, delete extra)
   * 6. Delete remote files that were locally deleted
   * 7. Upload new/modified local files respecting debounce and size limits
   * 8. Determine if sync is complete or more work remains
   *
   * @returns `true` if more sync iterations are needed, `false` if complete.
   */
  private async _sync(): Promise<boolean> {
    // 1. Get server connection
    const server = await this.getServer();
    if (!server) return false;

    const timestamp = Date.now();
    let pendingCount = 0;
    const configDir = this.filter.configDir;

    // 4. Special file scanning (config directory)
    if (this.scanSpecialFiles) {
      this.scanSpecialFiles = false;

      // Clear existing config-dir entries
      for (const filePath of Object.keys(this.localFiles)) {
        if (filePath.startsWith(configDir + "/")) {
          delete this.localFiles[filePath];
        }
      }

      // Scan config directory structure
      await this.scanConfigDirectory(configDir);
    }

    // 5. Process scan queue
    for (const filePath of this.scanSpecialFileQueue) {
      const exists = await this.adapter.exists(filePath);
      if (!exists) continue;

      const stat = await this.adapter.stat(filePath);
      if (!stat) continue;

      const existing = this.localFiles[filePath];
      if (existing) {
        // Update if mtime or size changed
        if (existing.mtime !== stat.mtime || existing.size !== stat.size) {
          existing.mtime = stat.mtime!;
          existing.size = stat.size!;
          existing.hash = ""; // Clear hash to force re-computation
          this.stateStore.setLocalFileRecord(existing);
        }
      } else {
        // Create new entry
        const record: FileRecord = {
          path: filePath,
          hash: "",
          ctime: stat.ctime!,
          mtime: stat.mtime!,
          size: stat.size!,
          folder: stat.type === "folder",
        };
        this.localFiles[filePath] = record;
        this.stateStore.setLocalFileRecord(record);
      }
    }
    this.scanSpecialFileQueue = [];

    // 6. Process pending server files (newServerFiles)
    const processedIndices: number[] = [];

    for (let i = 0; i < this.newServerFiles.length; i++) {
      const record = this.newServerFiles[i];
      const filePath = record.path;

      /**
       * Accept a server file change: update state and optionally update server record.
       */
      const accept = (silent: boolean, skipServerUpdate?: boolean): void => {
        processedIndices.push(i);
        this.stateStore.deletePendingFileRecord(filePath);
        if (!skipServerUpdate) {
          if (record.deleted) {
            delete this.serverFiles[filePath];
            this.stateStore.deleteServerFileRecord(filePath);
          } else {
            this.serverFiles[filePath] = record;
            this.stateStore.setServerFileRecord(record);
          }
        }
        if (!silent) {
          this.log(`[sync] accepted: ${filePath}`);
        }
      };

      // Filter check
      if (!this.filter.allowSyncFile(filePath, record.folder)) {
        accept(true);
        continue;
      }

      // Validate filename
      const name = basename(filePath);
      if (!record.folder && !isValidFilename(name)) {
        this.logSkip(filePath, "invalid filename");
        accept(true);
        continue;
      }

      // Security: reject paths containing ".."
      if (filePath.includes("..")) {
        this.logSkip(filePath, "path traversal");
        accept(true);
        continue;
      }

      // Check retry eligibility
      if (!this.canSyncPath(timestamp, filePath)) {
        pendingCount++;
        continue;
      }

      // Ensure local file hash is computed
      const localFile = this.localFiles[filePath];
      if (localFile && !localFile.folder && !localFile.hash) {
        try {
          const data = await this.adapter.readBinary(filePath);
          localFile.hash = await sha256Hex(data);
          this.stateStore.setLocalFileRecord(localFile);
        } catch {
          // File may have been deleted since state was loaded
          delete this.localFiles[filePath];
          this.stateStore.deleteLocalFileRecord(filePath);
        }
      }

      // Hash match: no changes needed
      const currentLocal = this.localFiles[filePath];
      if (currentLocal && !currentLocal.folder && currentLocal.hash === record.hash) {
        accept(true);
        continue;
      }

      // Skip duplicates (same path later in queue takes precedence)
      const laterDuplicate = this.newServerFiles.findIndex(
        (f, idx) => idx > i && f.path === filePath,
      );
      if (laterDuplicate !== -1) {
        accept(true, true);
        continue;
      }

      // No local file exists
      if (!currentLocal) {
        if (record.deleted) {
          accept(true);
        } else {
          // Download from server
          await this.syncFileDown(server, record);
          accept(false);
        }
        continue;
      }

      // Both are folders: handle folder deletion
      if (currentLocal.folder && record.folder) {
        if (record.deleted) {
          // Only delete if folder is empty on disk
          const contents = await this.adapter.list(filePath);
          if (contents.files.length === 0 && contents.folders.length === 0) {
            try {
              await this.adapter.rmdir(filePath);
              delete this.localFiles[filePath];
              this.stateStore.deleteLocalFileRecord(filePath);
            } catch {
              // Folder not empty or already gone
            }
          }
          accept(false);
        } else {
          accept(true);
        }
        continue;
      }

      // Pull-only or mirror-remote mode: server always wins
      if (this.syncMode === "pull" || this.syncMode === "mirror") {
        if (record.deleted) {
          try {
            await this.adapter.remove(filePath);
            delete this.localFiles[filePath];
            this.stateStore.deleteLocalFileRecord(filePath);
          } catch {
            // File already gone
          }
        } else {
          await this.syncFileDown(server, record);
        }
        accept(false);
        continue;
      }

      // Server hash matches old local hash: local hasn't changed, apply server
      const oldServerRecord = this.serverFiles[filePath];
      if (oldServerRecord && currentLocal.hash === oldServerRecord.hash) {
        if (record.deleted) {
          try {
            await this.adapter.remove(filePath);
            delete this.localFiles[filePath];
            this.stateStore.deleteLocalFileRecord(filePath);
          } catch {
            // File already gone
          }
        } else {
          await this.syncFileDown(server, record);
        }
        accept(false);
        continue;
      }

      // File-to-folder conflict: rename local with conflicted copy suffix
      if (!currentLocal.folder && record.folder) {
        const conflictName = this.getConflictedPath(filePath);
        try {
          await this.adapter.rename(filePath, conflictName);
          this.localFiles[conflictName] = { ...currentLocal, path: conflictName };
          this.stateStore.setLocalFileRecord(this.localFiles[conflictName]);
          delete this.localFiles[filePath];
          this.stateStore.deleteLocalFileRecord(filePath);
        } catch {
          // Rename failed
        }
        await this.syncFileDown(server, record);
        accept(false);
        continue;
      }

      // Initial sync with newer server mtime: download server version
      if (record.initial && record.mtime > currentLocal.mtime) {
        await this.syncFileDown(server, record);
        accept(false);
        continue;
      }

      // MERGE LOGIC for .md files (non-initial, hash mismatch)
      const ext = extname(filePath);
      if (ext === "md" && !record.initial) {
        let localContent: string;
        try {
          localContent = await this.adapter.read(filePath);
        } catch {
          // Can't read local: download server version
          await this.syncFileDown(server, record);
          accept(false);
          continue;
        }

        // Empty local file: just download
        if (!localContent) {
          await this.syncFileDown(server, record);
          accept(false);
          continue;
        }

        // Pull base (previous server version) and new server version
        const baseRecord = this.serverFiles[filePath];
        let baseContent: string | null = null;
        let remoteContent: string | null = null;

        // Get remote content
        try {
          const remoteData = await server.pull(record.uid!);
          if (remoteData) {
            remoteContent = bufferToString(remoteData);
          }
        } catch {
          this.failedSync(filePath);
          pendingCount++;
          continue;
        }

        // Contents match: no conflict
        if (remoteContent !== null && localContent === remoteContent) {
          accept(true);
          continue;
        }

        // Conflict strategy: create conflicted copy
        if (this.conflictStrategy === "conflict") {
          const conflictPath = this.getConflictedPath(filePath);
          try {
            await this.adapter.write(conflictPath, localContent);
            const newRecord: FileRecord = {
              path: conflictPath,
              hash: currentLocal.hash,
              ctime: currentLocal.ctime,
              mtime: currentLocal.mtime,
              size: currentLocal.size,
              folder: false,
            };
            this.localFiles[conflictPath] = newRecord;
            this.stateStore.setLocalFileRecord(newRecord);
            this.logMerge(filePath, "conflicted copy created");
          } catch {
            // Write failed
          }
          await this.syncFileDown(server, record);
          accept(false);
          continue;
        }

        // Merge strategy
        if (baseRecord && baseRecord.uid) {
          // Pull base version
          try {
            const baseData = await server.pull(baseRecord.uid);
            if (baseData) {
              baseContent = bufferToString(baseData);
            }
          } catch {
            // Can't get base: fall through to mtime comparison
          }
        }

        // No base version available
        if (baseContent === null) {
          // Recently created file (< 3 min): download server version
          if (currentLocal.ctime && timestamp - currentLocal.ctime < RECENTLY_CREATED_THRESHOLD_MS) {
            await this.syncFileDown(server, record);
            accept(false);
            continue;
          }

          // Use mtime comparison: newer wins
          if (record.mtime > currentLocal.mtime) {
            await this.syncFileDown(server, record);
          }
          // Otherwise keep local (it will be uploaded in the upload phase)
          accept(false);
          continue;
        }

        // Three-way merge
        if (remoteContent !== null) {
          const merged = threeWayMerge(baseContent, localContent, remoteContent);
          const mergedBuffer = stringToBuffer(merged);
          const mergedHash = await sha256Hex(mergedBuffer);

          await this.adapter.write(filePath, merged, {
            mtime: Math.max(record.mtime, currentLocal.mtime),
          });

          // Update local file record
          const updatedLocal: FileRecord = {
            ...currentLocal,
            hash: mergedHash,
            mtime: Math.max(record.mtime, currentLocal.mtime),
            size: mergedBuffer.byteLength,
          };
          this.localFiles[filePath] = updatedLocal;
          this.stateStore.setLocalFileRecord(updatedLocal);
          this.logMerge(filePath, "three-way merge applied");
        }

        accept(false);
        continue;
      }

      // Config file JSON merge: for .json files in configDir
      if (ext === "json" && filePath.startsWith(configDir + "/")) {
        try {
          const localContent = await this.adapter.read(filePath);
          const remoteData = await server.pull(record.uid!);
          if (remoteData) {
            const remoteContent = bufferToString(remoteData);
            const localObj = JSON.parse(localContent);
            const remoteObj = JSON.parse(remoteContent);

            // Merge: server keys win for conflicts
            const merged = { ...localObj, ...remoteObj };
            const mergedStr = JSON.stringify(merged, null, 2);
            const mergedBuffer = stringToBuffer(mergedStr);
            const mergedHash = await sha256Hex(mergedBuffer);

            await this.adapter.write(filePath, mergedStr);

            const updatedLocal: FileRecord = {
              ...currentLocal,
              hash: mergedHash,
              mtime: Date.now(),
              size: mergedBuffer.byteLength,
            };
            this.localFiles[filePath] = updatedLocal;
            this.stateStore.setLocalFileRecord(updatedLocal);
            this.logMerge(filePath, "JSON merge (server wins)");
          }
        } catch {
          // JSON parse failed or pull failed: download server version
          await this.syncFileDown(server, record);
        }
        accept(false);
        continue;
      }

      // Default: reject server change (keep local for upload)
      accept(true);
    }

    // Remove processed items from queue (in reverse order to maintain indices)
    for (const idx of processedIndices.sort((a, b) => b - a)) {
      this.newServerFiles.splice(idx, 1);
    }

    // 7. If not ready, don't proceed to upload phase
    if (!this.ready) return false;

    // 8. Mirror-remote mode: restore missing, delete extra, revert modified
    if (this.syncMode === "mirror") {
      // Restore files that exist on server but not locally
      for (const [filePath, record] of Object.entries(this.serverFiles)) {
        if (record.deleted) continue;
        if (!this.localFiles[filePath]) {
          await this.syncFileDown(server, record);
        }
      }

      // Delete local files not on server
      for (const [filePath, localRecord] of Object.entries(this.localFiles)) {
        if (!this.serverFiles[filePath] || this.serverFiles[filePath].deleted) {
          try {
            if (localRecord.folder) {
              await this.adapter.rmdir(filePath);
            } else {
              await this.adapter.remove(filePath);
            }
            delete this.localFiles[filePath];
            this.stateStore.deleteLocalFileRecord(filePath);
          } catch {
            // Already gone or permission error
          }
        }
      }

      // Revert locally modified files to server version
      for (const [filePath, localRecord] of Object.entries(this.localFiles)) {
        if (localRecord.folder) continue;
        const serverRecord = this.serverFiles[filePath];
        if (serverRecord && !serverRecord.deleted && localRecord.hash !== serverRecord.hash) {
          await this.syncFileDown(server, serverRecord);
        }
      }
    }

    // 9. Pull-only / mirror-remote: check if fully synced
    if (this.syncMode === "pull" || this.syncMode === "mirror") {
      if (pendingCount === 0 && this.newServerFiles.length === 0) {
        this.log("[sync] Fully synced (pull/mirror mode)");
        this.fileRetry = {};
        if (!this.continuous) {
          this.stop();
        }
        return false;
      }
      return pendingCount > 0;
    }

    // 10. Bidirectional: Delete remote files that were locally deleted
    for (const [filePath, serverRecord] of Object.entries(this.serverFiles)) {
      if (serverRecord.deleted) continue;
      if (!this.filter.allowSyncFile(filePath, serverRecord.folder)) continue;

      // If file existed locally but no longer does, push deletion
      if (!this.localFiles[filePath]) {
        this.syncingPath = filePath;
        try {
          await server.push(
            filePath,
            null,
            serverRecord.folder,
            true, // deleted
            serverRecord.ctime,
            Date.now(),
            "",
            null,
          );
          delete this.serverFiles[filePath];
          this.stateStore.deleteServerFileRecord(filePath);
          this.log(`[sync] deleted remote: ${filePath}`);
        } catch (err) {
          this.failedSync(filePath);
        }
        this.syncingPath = "";
      }
    }

    // 11. Bidirectional: Upload local files that are new or modified
    const toUpload: FileRecord[] = [];

    for (const [filePath, localRecord] of Object.entries(this.localFiles)) {
      if (!this.filter.allowSyncFile(filePath, localRecord.folder)) continue;
      if (!this.canSyncPath(timestamp, filePath)) continue;
      if (!this.canSyncLocalFile(timestamp, localRecord)) {
        pendingCount++;
        continue;
      }

      // Check file size limit
      if (!localRecord.folder && localRecord.size > server.perFileMax) {
        this.skippedFiles[filePath] = `exceeds size limit (${formatBytes(localRecord.size)} > ${formatBytes(server.perFileMax)})`;
        continue;
      }

      const serverRecord = this.serverFiles[filePath];

      // New file (not on server) or modified (hash differs)
      if (!serverRecord || serverRecord.deleted) {
        toUpload.push(localRecord);
      } else if (!localRecord.folder && localRecord.hash && localRecord.hash !== serverRecord.hash) {
        toUpload.push(localRecord);
      }
    }

    // Sort: folders first (shortest path), then files (smallest first)
    toUpload.sort((a, b) => {
      if (a.folder && !b.folder) return -1;
      if (!a.folder && b.folder) return 1;
      if (a.folder && b.folder) return a.path.length - b.path.length;
      return a.size - b.size;
    });

    // Upload each file
    for (const record of toUpload) {
      const filePath = record.path;
      this.syncingPath = filePath;

      try {
        // Compute hash if needed
        if (!record.folder && !record.hash) {
          const data = await this.adapter.readBinary(filePath);
          record.hash = await sha256Hex(data);
          this.stateStore.setLocalFileRecord(record);
        }

        // Skip if server already has this version
        const currentServer = this.serverFiles[filePath];
        if (currentServer && !currentServer.deleted && record.hash === currentServer.hash) {
          this.syncingPath = "";
          continue;
        }

        // Read file data for upload
        let data: ArrayBuffer | null = null;
        if (!record.folder) {
          data = await this.adapter.readBinary(filePath);
        }

        // Push to server
        await server.push(
          filePath,
          null,
          record.folder,
          false, // not deleted
          record.ctime,
          record.mtime,
          record.hash || "",
          data,
        );

        // Update server state
        const serverRecord: FileRecord = {
          path: filePath,
          hash: record.hash || "",
          ctime: record.ctime,
          mtime: record.mtime,
          size: record.size,
          folder: record.folder,
        };
        this.serverFiles[filePath] = serverRecord;
        this.stateStore.setServerFileRecord(serverRecord);

        this.log(`[sync] uploaded: ${filePath} (${formatBytes(record.size)})`);
      } catch (err) {
        this.failedSync(filePath);
      }

      this.syncingPath = "";
    }

    // 12. If nothing pending: fully synced
    if (pendingCount === 0 && this.newServerFiles.length === 0 && toUpload.length === 0) {
      this.log("[sync] Fully synced");
      this.fileRetry = {};
      if (!this.continuous) {
        this.stop();
      }
      return false;
    }

    return pendingCount > 0;
  }

  /* ---------------------------------------------------------------- */
  /*  File system event handling                                        */
  /* ---------------------------------------------------------------- */

  /**
   * Handle file system change events from the adapter.
   *
   * Translates raw FS events into local file record updates and triggers
   * sync when relevant changes are detected.
   *
   * @param event   - Event type: "file-created", "folder-created", "modified",
   *                  "file-removed", "folder-removed", "renamed", "raw"
   * @param filePath - Vault-relative path of the affected file.
   * @param oldPath - Previous path (for rename events).
   * @param stat    - File stat info (ctime, mtime, size) for create/modify events.
   */
  onChange(
    event: string,
    filePath: string,
    oldPath?: string,
    stat?: { ctime: number; mtime: number; size: number },
  ): void {
    // Skip files we're currently syncing (to avoid feedback loops)
    if (filePath === this.syncingPath) return;

    const configDir = this.filter.configDir;

    switch (event) {
      case "file-created": {
        if (!this.filter.allowSyncFile(filePath, false)) return;
        const record: FileRecord = {
          path: filePath,
          hash: "",
          ctime: stat?.ctime || Date.now(),
          mtime: stat?.mtime || Date.now(),
          size: stat?.size || 0,
          folder: false,
        };
        this.localFiles[filePath] = record;
        this.stateStore.setLocalFileRecord(record);
        this.scheduleSync();
        break;
      }

      case "folder-created": {
        if (!this.filter.allowSyncFile(filePath, true)) return;
        const record: FileRecord = {
          path: filePath,
          hash: "",
          ctime: stat?.ctime || Date.now(),
          mtime: stat?.mtime || Date.now(),
          size: 0,
          folder: true,
        };
        this.localFiles[filePath] = record;
        this.stateStore.setLocalFileRecord(record);
        this.scheduleSync();
        break;
      }

      case "modified": {
        const existing = this.localFiles[filePath];
        if (!existing) return;
        existing.mtime = stat?.mtime || Date.now();
        existing.size = stat?.size || existing.size;
        existing.hash = ""; // Invalidate hash to force re-computation
        this.stateStore.setLocalFileRecord(existing);
        this.scheduleSync();
        break;
      }

      case "file-removed": {
        if (this.localFiles[filePath]) {
          delete this.localFiles[filePath];
          this.stateStore.deleteLocalFileRecord(filePath);
          this.scheduleSync();
        }
        break;
      }

      case "folder-removed": {
        if (this.localFiles[filePath]) {
          delete this.localFiles[filePath];
          this.stateStore.deleteLocalFileRecord(filePath);
          this.scheduleSync();
        }
        break;
      }

      case "renamed": {
        if (oldPath && this.localFiles[oldPath]) {
          const record = this.localFiles[oldPath];
          delete this.localFiles[oldPath];
          this.stateStore.deleteLocalFileRecord(oldPath);
          record.path = filePath;
          record.previouspath = oldPath;
          this.localFiles[filePath] = record;
          this.stateStore.setLocalFileRecord(record);
          this.scheduleSync();
        }
        break;
      }

      case "raw": {
        // Raw events for config directory changes trigger special file scan
        if (filePath.startsWith(configDir + "/") || filePath === configDir) {
          if (!this.scanSpecialFileQueue.includes(filePath)) {
            this.scanSpecialFileQueue.push(filePath);
          }
          this.scheduleSync();
        }
        break;
      }
    }
  }

  /* ---------------------------------------------------------------- */
  /*  File download                                                     */
  /* ---------------------------------------------------------------- */

  /**
   * Download a file from the server and write it to the local vault.
   *
   * Handles both folders (mkdir) and files (pull, decrypt, write with timestamps).
   * Updates the local file record with the new hash and sync metadata.
   *
   * @param server - Active server connection to pull data from.
   * @param record - The server file record describing what to download.
   */
  async syncFileDown(server: SyncServerConnection, record: FileRecord): Promise<void> {
    const filePath = record.path;
    this.syncingPath = filePath;

    try {
      if (record.folder) {
        // Create directory
        await this.adapter.mkdir(filePath);
        const localRecord: FileRecord = {
          path: filePath,
          hash: "",
          ctime: record.ctime,
          mtime: record.mtime,
          size: 0,
          folder: true,
          synchash: "",
        };
        this.localFiles[filePath] = localRecord;
        this.stateStore.setLocalFileRecord(localRecord);
      } else {
        // Pull file content from server
        const data = await server.pull(record.uid!);
        if (data === null) {
          // File was deleted on server between push and pull
          return;
        }

        // Create parent directory if needed
        const dir = dirname(filePath);
        if (dir && dir !== ".") {
          await this.adapter.mkdir(dir);
        }

        // Compute hash of downloaded content
        const hash = await sha256Hex(data);

        // Write file with server timestamps
        await this.adapter.writeBinary(filePath, data, {
          ctime: record.ctime,
          mtime: record.mtime,
        });

        // Update local file record
        const localRecord: FileRecord = {
          path: filePath,
          hash,
          ctime: record.ctime,
          mtime: record.mtime,
          size: data.byteLength,
          folder: false,
          synchash: record.hash,
        };
        this.localFiles[filePath] = localRecord;
        this.stateStore.setLocalFileRecord(localRecord);

        this.log(`[download] ${filePath} (${formatBytes(data.byteLength)})`);
      }
    } finally {
      this.syncingPath = "";
    }
  }

  /* ---------------------------------------------------------------- */
  /*  Config directory scanning                                        */
  /* ---------------------------------------------------------------- */

  /**
   * Scan the vault's configuration directory for special files that
   * should be included in sync (themes, snippets, plugins, etc.).
   *
   * @param configDir - The config directory name (e.g. ".obsidian").
   */
  private async scanConfigDirectory(configDir: string): Promise<void> {
    const exists = await this.adapter.exists(configDir);
    if (!exists) return;

    try {
      // Root-level JSON files
      const rootContents = await this.adapter.list(configDir);
      for (const file of rootContents.files) {
        if (extname(file) === "json") {
          this.scanSpecialFileQueue.push(file);
        }
      }

      // themes/{name}/theme.css and themes/{name}/manifest.json
      const themesDir = `${configDir}/themes`;
      if (await this.adapter.exists(themesDir)) {
        const themes = await this.adapter.list(themesDir);
        for (const themeFolder of themes.folders) {
          const themeContents = await this.adapter.list(themeFolder);
          for (const file of themeContents.files) {
            const name = basename(file);
            if (name === "theme.css" || name === "manifest.json") {
              this.scanSpecialFileQueue.push(file);
            }
          }
        }
      }

      // snippets/*.css
      const snippetsDir = `${configDir}/snippets`;
      if (await this.adapter.exists(snippetsDir)) {
        const snippets = await this.adapter.list(snippetsDir);
        for (const file of snippets.files) {
          if (extname(file) === "css") {
            this.scanSpecialFileQueue.push(file);
          }
        }
      }

      // plugins/{name}/{pluginFiles}
      const pluginsDir = `${configDir}/plugins`;
      if (await this.adapter.exists(pluginsDir)) {
        const plugins = await this.adapter.list(pluginsDir);
        for (const pluginFolder of plugins.folders) {
          const pluginContents = await this.adapter.list(pluginFolder);
          for (const file of pluginContents.files) {
            const name = basename(file);
            if (this.filter.isPluginFile(name)) {
              this.scanSpecialFileQueue.push(file);
            }
          }
        }
      }
    } catch {
      // Config directory scan failed; will retry next cycle
    }
  }

  /* ---------------------------------------------------------------- */
  /*  Conflict resolution helpers                                      */
  /* ---------------------------------------------------------------- */

  /**
   * Generate a conflicted copy path for a file.
   *
   * Format: `{dir}/{basename} (Conflicted copy {device} {date}).{ext}`
   *
   * @param filePath - Original vault-relative file path.
   * @returns The conflicted copy path.
   */
  private getConflictedPath(filePath: string): string {
    const dir = dirname(filePath);
    const ext = extname(filePath);
    const name = basename(filePath);
    const nameWithoutExt = ext ? name.slice(0, -(ext.length + 1)) : name;
    const dateStr = new Date().toISOString().replace(/[:.]/g, "-").slice(0, 19);
    const conflictName = `${nameWithoutExt} (Conflicted copy ${this.deviceName} ${dateStr}).${ext || "md"}`;
    return dir && dir !== "." ? `${dir}/${conflictName}` : conflictName;
  }

  /* ---------------------------------------------------------------- */
  /*  Logging                                                          */
  /* ---------------------------------------------------------------- */

  /**
   * Log a sync message to the console.
   *
   * @param message - The message to log.
   */
  log(message: string): void {
    console.log(`[sync-engine] ${message}`);
  }

  /**
   * Log a merge operation.
   *
   * @param filePath - The file that was merged.
   * @param detail   - Description of the merge action taken.
   */
  logMerge(filePath: string, detail: string): void {
    console.log(`[sync-engine] [merge] ${filePath}: ${detail}`);
  }

  /**
   * Log a skipped file with reason.
   *
   * @param filePath - The file that was skipped.
   * @param reason   - Why the file was skipped.
   */
  logSkip(filePath: string, reason: string): void {
    console.log(`[sync-engine] [skip] ${filePath}: ${reason}`);
  }
}
