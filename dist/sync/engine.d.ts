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
import { SyncServerConnection, type ServerPushFile } from "./connection.js";
import { ExponentialBackoff } from "./backoff.js";
import { type FileRecord } from "../storage/state-store.js";
import type { EncryptionProvider } from "../encryption/types.js";
import type { SyncConfig } from "../config/index.js";
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
export declare class SyncEngine {
    /** Active server connection, or null if disconnected. */
    server: SyncServerConnection | null;
    /** Local file state indexed by vault-relative path. */
    localFiles: Record<string, FileRecord>;
    /** Server file state indexed by vault-relative path. */
    serverFiles: Record<string, FileRecord>;
    /** Queue of pending server file changes awaiting processing. */
    newServerFiles: FileRecord[];
    /** Current sync version counter from the server. */
    version: number;
    /** Whether we are in initial sync (first full download). */
    initial: boolean;
    /** Whether the server has signalled readiness. */
    ready: boolean;
    /** Whether a sync operation is currently in progress. */
    syncing: boolean;
    /** Whether the engine has completed its first full load cycle. */
    loaded: boolean;
    /** Backoff controller for server reconnection (0ms min, 5min max, 5s base). */
    backoff: ExponentialBackoff;
    /** Per-file retry tracking for failed sync operations. */
    fileRetry: Record<string, {
        count: number;
        ts: number;
    }>;
    /** Files skipped during sync with a reason string. */
    skippedFiles: Record<string, string>;
    /** Path currently being synced (for re-entrancy detection). */
    syncingPath: string;
    /** Whether the config directory needs re-scanning. */
    scanSpecialFiles: boolean;
    /** Queue of special file paths discovered during config-dir scanning. */
    scanSpecialFileQueue: string[];
    /** Resolver function for the stop signal promise. */
    resolveStop: (() => void) | null;
    /** Sync server hostname. */
    private host;
    /** Authentication token. */
    private token;
    /** Remote vault identifier. */
    private vaultId;
    /** Whether to keep running after initial sync completes. */
    private continuous;
    /** Encryption provider for content and path encoding. */
    private encryption;
    /** SQLite state store for persisting sync metadata. */
    private stateStore;
    /** Conflict resolution strategy: "merge" or "conflict". */
    private conflictStrategy;
    /** Sync mode: "" (bidirectional), "pull", or "mirror". */
    private syncMode;
    /** Device name used in conflict copies and server identification. */
    private deviceName;
    /** File system adapter for vault I/O operations. */
    private adapter;
    /** Filter for determining which files should be synced. */
    private filter;
    /** Handle for the periodic sync interval timer. */
    private syncInterval;
    /** Debounced sync request to coalesce rapid triggers. */
    private debouncedRequestSync;
    /**
     * Create a new sync engine.
     *
     * Initializes the state store, file system adapter, sync filter, and
     * loads persisted state from the SQLite database.
     *
     * @param options - Engine configuration options.
     */
    constructor(options: SyncEngineOptions);
    /**
     * Load persisted sync state from the SQLite database.
     *
     * Restores the version counter, initial sync flag, local file records,
     * server file records, and any pending (unprocessed) server file changes.
     */
    private loadState;
    /**
     * Handle a file change pushed from the server.
     *
     * Updates the sync version, creates a FileRecord from the push data,
     * and either accepts a self-echo (wasJustPushed) immediately or queues
     * the change for processing during the next sync cycle.
     *
     * @param pushFile - The server push notification containing file metadata.
     */
    handlePush(pushFile: ServerPushFile): void;
    /**
     * Start the sync engine.
     *
     * In continuous mode, this sets up file watching and a periodic sync
     * interval, then blocks until {@link stop} is called. In one-shot mode,
     * performs a single sync cycle and returns.
     *
     * @returns A promise that resolves when sync is stopped or completes.
     */
    sync(): Promise<void>;
    /**
     * Stop the sync engine gracefully.
     *
     * Disconnects from the server, closes the state store, stops file
     * watching, cancels pending debounced operations, and resolves the
     * stop promise to unblock {@link sync}.
     */
    stop(): void;
    /**
     * Schedule a sync request via debouncing to coalesce rapid triggers.
     * @internal
     */
    private scheduleSync;
    /**
     * Execute the sync loop if not already running.
     *
     * Repeatedly calls {@link _sync} until no more work remains, with a
     * minimum rate limit between iterations to avoid overwhelming the server.
     * Handles errors by logging and applying backoff (or throwing in one-shot mode).
     */
    requestSync(): Promise<void>;
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
    canSyncPath(timestamp: number, filePath: string): boolean;
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
    canSyncLocalFile(timestamp: number, record: FileRecord): boolean;
    /**
     * Record a failed sync attempt for a path and compute the next retry time.
     *
     * Uses exponential backoff per file: `2^count * 5000` ms, capped at 5 minutes.
     *
     * @param filePath - The vault-relative path that failed.
     */
    failedSync(filePath: string): void;
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
    getServer(): Promise<SyncServerConnection | null>;
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
    private _sync;
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
    onChange(event: string, filePath: string, oldPath?: string, stat?: {
        ctime: number;
        mtime: number;
        size: number;
    }): void;
    /**
     * Download a file from the server and write it to the local vault.
     *
     * Handles both folders (mkdir) and files (pull, decrypt, write with timestamps).
     * Updates the local file record with the new hash and sync metadata.
     *
     * @param server - Active server connection to pull data from.
     * @param record - The server file record describing what to download.
     */
    syncFileDown(server: SyncServerConnection, record: FileRecord): Promise<void>;
    /**
     * Scan the vault's configuration directory for special files that
     * should be included in sync (themes, snippets, plugins, etc.).
     *
     * @param configDir - The config directory name (e.g. ".obsidian").
     */
    private scanConfigDirectory;
    /**
     * Generate a conflicted copy path for a file.
     *
     * Format: `{dir}/{basename} (Conflicted copy {device} {date}).{ext}`
     *
     * @param filePath - Original vault-relative file path.
     * @returns The conflicted copy path.
     */
    private getConflictedPath;
    /**
     * Log a sync message to the console.
     *
     * @param message - The message to log.
     */
    log(message: string): void;
    /**
     * Log a merge operation.
     *
     * @param filePath - The file that was merged.
     * @param detail   - Description of the merge action taken.
     */
    logMerge(filePath: string, detail: string): void;
    /**
     * Log a skipped file with reason.
     *
     * @param filePath - The file that was skipped.
     * @param reason   - Why the file was skipped.
     */
    logSkip(filePath: string, reason: string): void;
}
//# sourceMappingURL=engine.d.ts.map