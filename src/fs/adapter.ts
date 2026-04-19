/**
 * @module fs/adapter
 *
 * File system adapter for vault operations.  Wraps Node.js `fs` to provide
 * a high-level interface for reading, writing, watching, and indexing vault
 * files.  Emits change events for consumption by the sync engine.
 *
 * Optimised for minimal filesystem I/O:
 * - Uses async `fs.promises` for all non-blocking operations
 * - Single recursive scan using `readdir({ recursive: true })`
 * - Debounced watch events to batch rapid changes
 * - In-memory index avoids redundant stat calls
 */

import fs from "node:fs";
import fsp from "node:fs/promises";
import path from "node:path";
import url from "node:url";

import { debounce, type DebouncedFn } from "../utils/debounce.js";

/* ------------------------------------------------------------------ */
/*  Types                                                              */
/* ------------------------------------------------------------------ */

/** Options for write operations that allow setting file timestamps. */
export interface WriteOptions {
  ctime?: number;
  mtime?: number;
}

/** Stat info stored in the in-memory file index. */
export interface FileStat {
  type: "file" | "folder";
  realpath: string;
  ctime?: number;
  mtime?: number;
  size?: number;
}

/** Handler callback signature for file change events. */
export type FileEventHandler = (
  event: string,
  filePath: string,
  oldPath?: string,
  stat?: { ctime: number; mtime: number; size: number },
) => void;

/* ------------------------------------------------------------------ */
/*  FileSystemAdapter                                                  */
/* ------------------------------------------------------------------ */

/**
 * File system adapter for vault operations.
 *
 * Provides a unified interface for reading, writing, listing, watching,
 * and indexing files within an Obsidian vault directory.
 */
export class FileSystemAdapter {
  /** In-memory file index mapping vault-relative paths to stat info. */
  files: Record<string, FileStat> = {};

  /** Debounced function that triggers a kill timeout (60 seconds). */
  thingsHappening: DebouncedFn;

  /** Whether the underlying file system is case-insensitive. */
  insensitive = false;

  /** Event handler callback, set via {@link watch}. */
  handler: FileEventHandler | null = null;

  private fsModule: typeof fs;
  private pathModule: typeof path;
  private urlModule: typeof url;
  private trash: (filePath: string) => boolean;
  private btime: ((filePath: string, time: number) => void) | null;
  private resourcePathPrefix: string;
  private basePath: string;
  private watchers: fs.FSWatcher[] = [];
  private kill: () => void;

  /**
   * Pending watch events keyed by relative path. Events arriving within a
   * short window are coalesced so we only stat the file once per batch.
   */
  private pendingWatchEvents: Map<string, ReturnType<typeof setTimeout>> =
    new Map();

  /** Minimum ms between processing watch events for the same path. */
  private static readonly WATCH_DEBOUNCE_MS = 50;

  /**
   * Create a new file system adapter.
   *
   * @param fsModule            - Node.js `fs` module.
   * @param pathModule          - Node.js `path` module.
   * @param urlModule           - Node.js `url` module.
   * @param trash               - Trash function (returns false; no-op).
   * @param btime               - Native birthtime setter, or null.
   * @param resourcePathPrefix  - URI prefix for resources (e.g. "file:///").
   * @param basePath            - Absolute path to the vault root.
   */
  constructor(
    fsModule: typeof fs,
    pathModule: typeof path,
    urlModule: typeof url,
    trash: (filePath: string) => boolean,
    btime: ((filePath: string, time: number) => void) | null,
    resourcePathPrefix: string,
    basePath: string,
  ) {
    this.fsModule = fsModule;
    this.pathModule = pathModule;
    this.urlModule = urlModule;
    this.trash = trash;
    this.btime = btime;
    this.resourcePathPrefix = resourcePathPrefix;
    this.basePath = pathModule.resolve(basePath);

    // Kill function placeholder (no-op; can be overridden externally)
    this.kill = () => {};

    this.thingsHappening = debounce(() => {
      this.kill();
    }, 60_000);

    // Detect case-insensitive file system
    this.detectCaseSensitivity();
  }

  /* ---------------------------------------------------------------- */
  /*  Path helpers                                                     */
  /* ---------------------------------------------------------------- */

  /**
   * Get the vault name (base directory name).
   */
  getName(): string {
    return this.pathModule.basename(this.basePath);
  }

  /**
   * Get the absolute path to the vault root.
   */
  getBasePath(): string {
    return this.basePath;
  }

  /**
   * Resolve a vault-relative path to an absolute file system path.
   *
   * @param relativePath - Vault-relative path.
   * @returns Absolute path on disk.
   */
  getFullRealPath(relativePath: string): string {
    return this.pathModule.join(this.basePath, relativePath);
  }

  /* ---------------------------------------------------------------- */
  /*  Listing                                                          */
  /* ---------------------------------------------------------------- */

  /**
   * Recursively scan all files in the vault, populating {@link files}.
   * Skips hidden files and directories (names starting with `.`).
   *
   * Uses `fs.promises.readdir` with `{ recursive: true, withFileTypes: true }`
   * for a single kernel call that returns the entire tree, then stats only
   * regular files (directories don't need size/mtime).
   */
  async listAll(): Promise<void> {
    this.files = {};

    let entries: fs.Dirent[];
    try {
      entries = await fsp.readdir(this.basePath, {
        withFileTypes: true,
        recursive: true,
      });
    } catch {
      this.thingsHappening();
      return;
    }

    // Batch stat promises for files — directories don't need stat
    const fileStatJobs: Array<{
      relativePath: string;
      fullPath: string;
    }> = [];

    for (const entry of entries) {
      // Build the vault-relative path from parentPath (Node 20+) or name
      const parentDir = (entry as any).parentPath ?? (entry as any).path ?? "";
      const fullPath = this.pathModule.join(parentDir, entry.name);
      const relativePath = this.pathModule
        .relative(this.basePath, fullPath)
        .replace(/\\/g, "/");

      // Skip hidden files/directories (any segment starting with ".")
      if (this.isHiddenRelative(relativePath)) continue;

      if (entry.isDirectory()) {
        this.files[relativePath] = {
          type: "folder",
          realpath: fullPath,
        };
      } else if (entry.isFile()) {
        fileStatJobs.push({ relativePath, fullPath });
      }
    }

    // Stat all files concurrently in batches of 64 to avoid fd exhaustion
    const BATCH_SIZE = 64;
    for (let i = 0; i < fileStatJobs.length; i += BATCH_SIZE) {
      const batch = fileStatJobs.slice(i, i + BATCH_SIZE);
      const results = await Promise.allSettled(
        batch.map(async ({ relativePath, fullPath }) => {
          const s = await fsp.stat(fullPath);
          this.files[relativePath] = {
            type: "file",
            realpath: fullPath,
            ctime: s.ctimeMs,
            mtime: s.mtimeMs,
            size: s.size,
          };
        }),
      );
      // Silently skip files that disappeared between readdir and stat
    }

    this.thingsHappening();
  }

  /**
   * Recursively scan a directory, adding entries to {@link files}.
   * Falls back to the single-dir approach for targeted rescans.
   *
   * @param dir - Vault-relative directory path to scan.
   */
  async listRecursive(dir: string): Promise<void> {
    const fullPath = dir
      ? this.pathModule.join(this.basePath, dir)
      : this.basePath;

    let entries: fs.Dirent[];
    try {
      entries = await fsp.readdir(fullPath, {
        withFileTypes: true,
        recursive: true,
      });
    } catch {
      return;
    }

    const fileStatJobs: Array<{
      relativePath: string;
      entryFullPath: string;
    }> = [];

    for (const entry of entries) {
      const parentDir = (entry as any).parentPath ?? (entry as any).path ?? "";
      const entryFullPath = this.pathModule.join(parentDir, entry.name);
      const relativePath = this.pathModule
        .relative(this.basePath, entryFullPath)
        .replace(/\\/g, "/");

      if (this.isHiddenRelative(relativePath)) continue;

      if (entry.isDirectory()) {
        this.files[relativePath] = {
          type: "folder",
          realpath: entryFullPath,
        };
      } else if (entry.isFile()) {
        fileStatJobs.push({ relativePath, entryFullPath });
      }
    }

    const BATCH_SIZE = 64;
    for (let i = 0; i < fileStatJobs.length; i += BATCH_SIZE) {
      const batch = fileStatJobs.slice(i, i + BATCH_SIZE);
      await Promise.allSettled(
        batch.map(async ({ relativePath, entryFullPath }) => {
          const s = await fsp.stat(entryFullPath);
          this.files[relativePath] = {
            type: "file",
            realpath: entryFullPath,
            ctime: s.ctimeMs,
            mtime: s.mtimeMs,
            size: s.size,
          };
        }),
      );
    }

    this.thingsHappening();
  }

  /**
   * List the immediate contents of a directory.
   *
   * @param dir - Vault-relative directory path.
   * @returns Object containing `files` and `folders` arrays of relative paths.
   */
  async list(dir: string): Promise<{ files: string[]; folders: string[] }> {
    const fullPath = dir
      ? this.pathModule.join(this.basePath, dir)
      : this.basePath;
    const files: string[] = [];
    const folders: string[] = [];

    let entries: fs.Dirent[];
    try {
      entries = await fsp.readdir(fullPath, { withFileTypes: true });
    } catch {
      return { files, folders };
    }

    for (const entry of entries) {
      if (entry.name.startsWith(".")) continue;
      const relativePath = dir ? `${dir}/${entry.name}` : entry.name;
      if (entry.isDirectory()) {
        folders.push(relativePath);
      } else if (entry.isFile()) {
        files.push(relativePath);
      }
    }

    return { files, folders };
  }

  /* ---------------------------------------------------------------- */
  /*  File operations                                                  */
  /* ---------------------------------------------------------------- */

  /**
   * Check whether a file or directory exists at the given path.
   * Uses the in-memory index first, falling back to disk only if needed.
   *
   * @param filePath - Vault-relative path.
   */
  async exists(filePath: string): Promise<boolean> {
    // Fast path: check in-memory index
    if (filePath in this.files) return true;
    // Slow path: check disk
    try {
      await fsp.access(this.getFullRealPath(filePath));
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Get stat information for a file or directory.
   * Uses the in-memory index if available, otherwise stats from disk.
   *
   * @param filePath - Vault-relative path.
   * @returns Stat object with type, realpath, ctime, mtime, and size.
   */
  async stat(filePath: string): Promise<FileStat | null> {
    // Fast path: return from in-memory index if we have full info
    const cached = this.files[filePath];
    if (cached && cached.mtime !== undefined) return cached;

    const fullPath = this.getFullRealPath(filePath);
    try {
      const s = await fsp.stat(fullPath);
      return {
        type: s.isDirectory() ? "folder" : "file",
        realpath: fullPath,
        ctime: s.ctimeMs,
        mtime: s.mtimeMs,
        size: s.size,
      };
    } catch {
      return null;
    }
  }

  /**
   * Read a file as UTF-8 text.
   *
   * @param filePath - Vault-relative path.
   * @returns File contents as a string.
   */
  async read(filePath: string): Promise<string> {
    const fullPath = this.getFullRealPath(filePath);
    return fsp.readFile(fullPath, "utf-8");
  }

  /**
   * Read a file as binary data.
   *
   * @param filePath - Vault-relative path.
   * @returns File contents as an ArrayBuffer.
   */
  async readBinary(filePath: string): Promise<ArrayBuffer> {
    const fullPath = this.getFullRealPath(filePath);
    const buffer = await fsp.readFile(fullPath);
    return buffer.buffer.slice(
      buffer.byteOffset,
      buffer.byteOffset + buffer.byteLength,
    );
  }

  /**
   * Write text content to a file, creating parent directories as needed.
   * Applies ctime/mtime from options when provided.
   *
   * @param filePath - Vault-relative path.
   * @param content  - UTF-8 text content to write.
   * @param options  - Optional timestamp overrides.
   */
  async write(
    filePath: string,
    content: string,
    options?: WriteOptions,
  ): Promise<void> {
    const fullPath = this.getFullRealPath(filePath);
    const dir = this.pathModule.dirname(fullPath);
    await fsp.mkdir(dir, { recursive: true });
    await fsp.writeFile(fullPath, content, "utf-8");
    await this.applyTimestamps(fullPath, options);
  }

  /**
   * Write binary data to a file, creating parent directories as needed.
   * Applies ctime/mtime from options when provided.
   *
   * @param filePath - Vault-relative path.
   * @param data     - Binary data to write.
   * @param options  - Optional timestamp overrides.
   */
  async writeBinary(
    filePath: string,
    data: ArrayBuffer,
    options?: WriteOptions,
  ): Promise<void> {
    const fullPath = this.getFullRealPath(filePath);
    const dir = this.pathModule.dirname(fullPath);
    await fsp.mkdir(dir, { recursive: true });
    await fsp.writeFile(fullPath, Buffer.from(data));
    await this.applyTimestamps(fullPath, options);
  }

  /**
   * Append text content to a file.
   *
   * @param filePath - Vault-relative path.
   * @param content  - Text to append.
   */
  async append(filePath: string, content: string): Promise<void> {
    const fullPath = this.getFullRealPath(filePath);
    await fsp.appendFile(fullPath, content, "utf-8");
  }

  /**
   * Create a directory (recursively).
   *
   * @param dirPath - Vault-relative directory path.
   */
  async mkdir(dirPath: string): Promise<void> {
    const fullPath = this.getFullRealPath(dirPath);
    await fsp.mkdir(fullPath, { recursive: true });
  }

  /**
   * Remove (unlink) a file.
   *
   * @param filePath - Vault-relative path.
   */
  async remove(filePath: string): Promise<void> {
    const fullPath = this.getFullRealPath(filePath);
    await fsp.unlink(fullPath);
  }

  /**
   * Remove a directory.
   *
   * @param dirPath   - Vault-relative directory path.
   * @param recursive - Whether to remove contents recursively.
   */
  async rmdir(dirPath: string, recursive?: boolean): Promise<void> {
    const fullPath = this.getFullRealPath(dirPath);
    await fsp.rm(fullPath, { recursive: recursive ?? false });
  }

  /**
   * Rename (move) a file or directory.
   *
   * @param oldPath - Current vault-relative path.
   * @param newPath - New vault-relative path.
   */
  async rename(oldPath: string, newPath: string): Promise<void> {
    const fullOld = this.getFullRealPath(oldPath);
    const fullNew = this.getFullRealPath(newPath);
    const dir = this.pathModule.dirname(fullNew);
    await fsp.mkdir(dir, { recursive: true });
    await fsp.rename(fullOld, fullNew);
  }

  /* ---------------------------------------------------------------- */
  /*  Watching                                                         */
  /* ---------------------------------------------------------------- */

  /**
   * Start watching the vault for file system changes.
   * Performs an initial full scan, then sets up recursive watchers.
   *
   * @param handler - Callback for file change events.
   */
  async watch(handler: FileEventHandler): Promise<void> {
    this.handler = handler;
    await this.listAll();

    this.watchDirectory(this.basePath);
  }

  /**
   * Stop all file system watchers and cancel pending debounced events.
   */
  stopWatch(): void {
    for (const watcher of this.watchers) {
      watcher.close();
    }
    this.watchers = [];
    this.handler = null;

    // Cancel all pending debounced watch events
    for (const timer of this.pendingWatchEvents.values()) {
      clearTimeout(timer);
    }
    this.pendingWatchEvents.clear();
  }

  /**
   * Trigger a file event via the registered handler.
   *
   * @param event   - Event name (e.g. "file-created", "modified").
   * @param filePath - Vault-relative path of the affected file.
   * @param oldPath - Previous path (for rename events).
   * @param stat    - Stat info for the file.
   */
  trigger(
    event: string,
    filePath: string,
    oldPath?: string,
    stat?: { ctime: number; mtime: number; size: number },
  ): void {
    if (this.handler) {
      this.handler(event, filePath, oldPath, stat);
    }
  }

  /**
   * Reconcile a deletion: if a previously-indexed path no longer exists
   * on disk, remove it from the file index and emit the appropriate
   * removal event.
   *
   * @param filePath - Vault-relative path to check.
   */
  async reconcileDeletion(filePath: string): Promise<void> {
    const entry = this.files[filePath];
    if (!entry) return;

    const fullPath = this.getFullRealPath(filePath);
    try {
      await fsp.access(fullPath);
    } catch {
      // File doesn't exist on disk — emit removal
      const type = entry.type;
      delete this.files[filePath];
      if (type === "folder") {
        this.trigger("folder-removed", filePath);
      } else {
        this.trigger("file-removed", filePath);
      }
    }
  }

  /* ---------------------------------------------------------------- */
  /*  Private helpers                                                  */
  /* ---------------------------------------------------------------- */

  /**
   * Check if a vault-relative path is hidden (any segment starts with ".").
   */
  private isHiddenRelative(relativePath: string): boolean {
    const segments = relativePath.split("/");
    return segments.some((s) => s.startsWith("."));
  }

  /**
   * Detect whether the file system is case-insensitive by creating a
   * test file and checking if its lowercase variant exists.
   */
  private detectCaseSensitivity(): void {
    const testFile = this.pathModule.join(this.basePath, ".OBSIDIANTEST");
    try {
      this.fsModule.writeFileSync(testFile, "");
      const lowerPath = this.pathModule.join(this.basePath, ".obsidiantest");
      this.insensitive = this.fsModule.existsSync(lowerPath);
      this.fsModule.unlinkSync(testFile);
    } catch {
      // If we can't create the test file, assume case-sensitive
      this.insensitive = false;
    }
  }

  /**
   * Apply timestamp overrides to a file after writing.
   *
   * @param fullPath - Absolute file path.
   * @param options  - Write options with optional ctime/mtime.
   */
  private async applyTimestamps(
    fullPath: string,
    options?: WriteOptions,
  ): Promise<void> {
    if (!options) return;

    if (options.ctime && this.btime) {
      // Set birth time using native function (macOS/Windows)
      this.btime(fullPath, options.ctime);
    }

    if (options.mtime) {
      const mtime = new Date(options.mtime);
      const atime = new Date(); // access time = now
      await fsp.utimes(fullPath, atime, mtime);
    }
  }

  /**
   * Set up a file system watcher on a directory.
   *
   * @param dirPath - Absolute path to the directory to watch.
   */
  private watchDirectory(dirPath: string): void {
    try {
      const watcher = this.fsModule.watch(
        dirPath,
        { recursive: true },
        (eventType, filename) => {
          if (!filename) return;

          // Skip hidden files
          if (
            filename.startsWith(".") ||
            filename.includes(`${this.pathModule.sep}.`)
          ) {
            return;
          }

          // Normalise path separators to forward slashes
          const relativePath = filename.replace(/\\/g, "/");

          // Debounce: if we already have a pending event for this path,
          // cancel it and reschedule. This prevents multiple rapid stat
          // calls for the same file (e.g. editor save-then-rename).
          const existing = this.pendingWatchEvents.get(relativePath);
          if (existing) {
            clearTimeout(existing);
          }

          const timer = setTimeout(() => {
            this.pendingWatchEvents.delete(relativePath);
            this.handleWatchEvent(eventType, relativePath);
          }, FileSystemAdapter.WATCH_DEBOUNCE_MS);

          this.pendingWatchEvents.set(relativePath, timer);
        },
      );
      this.watchers.push(watcher);
    } catch {
      // Directory may not exist or permissions may be insufficient
    }
  }

  /**
   * Process a raw watch event, determining if it's a creation, modification,
   * rename, or deletion and emitting the correct event.
   */
  private async handleWatchEvent(
    eventType: string,
    relativePath: string,
  ): Promise<void> {
    const fullPath = this.getFullRealPath(relativePath);
    const existed = relativePath in this.files;

    // Emit raw event
    this.trigger("raw", relativePath);

    let s: fs.Stats;
    try {
      s = await fsp.stat(fullPath);
    } catch {
      // File/folder was deleted
      if (existed) {
        const entry = this.files[relativePath];
        delete this.files[relativePath];
        if (entry.type === "folder") {
          this.trigger("folder-removed", relativePath);
        } else {
          this.trigger("file-removed", relativePath);
        }
      }
      return;
    }

    const stat = { ctime: s.ctimeMs, mtime: s.mtimeMs, size: s.size };

    if (s.isDirectory()) {
      if (!existed) {
        this.files[relativePath] = {
          type: "folder",
          realpath: fullPath,
        };
        this.trigger("folder-created", relativePath);
      }
    } else {
      const newEntry: FileStat = {
        type: "file",
        realpath: fullPath,
        ctime: stat.ctime,
        mtime: stat.mtime,
        size: stat.size,
      };

      if (!existed) {
        this.files[relativePath] = newEntry;
        this.trigger("file-created", relativePath, undefined, stat);
      } else {
        // Only emit "modified" if stat actually changed
        const old = this.files[relativePath];
        if (old.mtime !== stat.mtime || old.size !== stat.size) {
          this.files[relativePath] = newEntry;
          this.trigger("modified", relativePath, undefined, stat);
        }
      }
    }
  }
}
