/**
 * @module fs/adapter
 *
 * File system adapter for vault operations.  Wraps Node.js `fs` to provide
 * a high-level interface for reading, writing, watching, and indexing vault
 * files.  Emits change events for consumption by the sync engine.
 */
import fs from "node:fs";
import path from "node:path";
import url from "node:url";
import { type DebouncedFn } from "../utils/debounce.js";
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
export type FileEventHandler = (event: string, filePath: string, oldPath?: string, stat?: {
    ctime: number;
    mtime: number;
    size: number;
}) => void;
/**
 * File system adapter for vault operations.
 *
 * Provides a unified interface for reading, writing, listing, watching,
 * and indexing files within an Obsidian vault directory.
 */
export declare class FileSystemAdapter {
    /** In-memory file index mapping vault-relative paths to stat info. */
    files: Record<string, FileStat>;
    /** Debounced function that triggers a kill timeout (60 seconds). */
    thingsHappening: DebouncedFn;
    /** Whether the underlying file system is case-insensitive. */
    insensitive: boolean;
    /** Event handler callback, set via {@link watch}. */
    handler: FileEventHandler | null;
    private fsModule;
    private pathModule;
    private urlModule;
    private trash;
    private btime;
    private resourcePathPrefix;
    private basePath;
    private watchers;
    private kill;
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
    constructor(fsModule: typeof fs, pathModule: typeof path, urlModule: typeof url, trash: (filePath: string) => boolean, btime: ((filePath: string, time: number) => void) | null, resourcePathPrefix: string, basePath: string);
    /**
     * Get the vault name (base directory name).
     */
    getName(): string;
    /**
     * Get the absolute path to the vault root.
     */
    getBasePath(): string;
    /**
     * Resolve a vault-relative path to an absolute file system path.
     *
     * @param relativePath - Vault-relative path.
     * @returns Absolute path on disk.
     */
    getFullRealPath(relativePath: string): string;
    /**
     * Recursively scan all files in the vault, populating {@link files}.
     * Skips hidden files and directories (names starting with `.`).
     */
    listAll(): Promise<void>;
    /**
     * Recursively scan a directory, adding entries to {@link files}.
     *
     * @param dir - Vault-relative directory path to scan.
     */
    listRecursive(dir: string): Promise<void>;
    /**
     * Recursively scan a child directory (same as listRecursive but used
     * internally to avoid redundant root-level checks).
     */
    private listRecursiveChild;
    /**
     * List the immediate contents of a directory.
     *
     * @param dir - Vault-relative directory path.
     * @returns Object containing `files` and `folders` arrays of relative paths.
     */
    list(dir: string): Promise<{
        files: string[];
        folders: string[];
    }>;
    /**
     * Check whether a file or directory exists at the given path.
     *
     * @param filePath - Vault-relative path.
     */
    exists(filePath: string): Promise<boolean>;
    /**
     * Get stat information for a file or directory.
     *
     * @param filePath - Vault-relative path.
     * @returns Stat object with type, realpath, ctime, mtime, and size.
     */
    stat(filePath: string): Promise<FileStat | null>;
    /**
     * Read a file as UTF-8 text.
     *
     * @param filePath - Vault-relative path.
     * @returns File contents as a string.
     */
    read(filePath: string): Promise<string>;
    /**
     * Read a file as binary data.
     *
     * @param filePath - Vault-relative path.
     * @returns File contents as an ArrayBuffer.
     */
    readBinary(filePath: string): Promise<ArrayBuffer>;
    /**
     * Write text content to a file, creating parent directories as needed.
     * Applies ctime/mtime from options when provided.
     *
     * @param filePath - Vault-relative path.
     * @param content  - UTF-8 text content to write.
     * @param options  - Optional timestamp overrides.
     */
    write(filePath: string, content: string, options?: WriteOptions): Promise<void>;
    /**
     * Write binary data to a file, creating parent directories as needed.
     * Applies ctime/mtime from options when provided.
     *
     * @param filePath - Vault-relative path.
     * @param data     - Binary data to write.
     * @param options  - Optional timestamp overrides.
     */
    writeBinary(filePath: string, data: ArrayBuffer, options?: WriteOptions): Promise<void>;
    /**
     * Append text content to a file.
     *
     * @param filePath - Vault-relative path.
     * @param content  - Text to append.
     */
    append(filePath: string, content: string): Promise<void>;
    /**
     * Create a directory (recursively).
     *
     * @param dirPath - Vault-relative directory path.
     */
    mkdir(dirPath: string): Promise<void>;
    /**
     * Remove (unlink) a file.
     *
     * @param filePath - Vault-relative path.
     */
    remove(filePath: string): Promise<void>;
    /**
     * Remove a directory.
     *
     * @param dirPath   - Vault-relative directory path.
     * @param recursive - Whether to remove contents recursively.
     */
    rmdir(dirPath: string, recursive?: boolean): Promise<void>;
    /**
     * Rename (move) a file or directory.
     *
     * @param oldPath - Current vault-relative path.
     * @param newPath - New vault-relative path.
     */
    rename(oldPath: string, newPath: string): Promise<void>;
    /**
     * Start watching the vault for file system changes.
     * Performs an initial full scan, then sets up recursive watchers.
     *
     * @param handler - Callback for file change events.
     */
    watch(handler: FileEventHandler): Promise<void>;
    /**
     * Stop all file system watchers.
     */
    stopWatch(): void;
    /**
     * Trigger a file event via the registered handler.
     *
     * @param event   - Event name (e.g. "file-created", "modified").
     * @param filePath - Vault-relative path of the affected file.
     * @param oldPath - Previous path (for rename events).
     * @param stat    - Stat info for the file.
     */
    trigger(event: string, filePath: string, oldPath?: string, stat?: {
        ctime: number;
        mtime: number;
        size: number;
    }): void;
    /**
     * Reconcile a deletion: if a previously-indexed path no longer exists
     * on disk, remove it from the file index and emit the appropriate
     * removal event.
     *
     * @param filePath - Vault-relative path to check.
     */
    reconcileDeletion(filePath: string): void;
    /**
     * Detect whether the file system is case-insensitive by creating a
     * test file and checking if its lowercase variant exists.
     */
    private detectCaseSensitivity;
    /**
     * Apply timestamp overrides to a file after writing.
     *
     * @param fullPath - Absolute file path.
     * @param options  - Write options with optional ctime/mtime.
     */
    private applyTimestamps;
    /**
     * Set up a file system watcher on a directory.
     *
     * @param dirPath - Absolute path to the directory to watch.
     */
    private watchDirectory;
    /**
     * Process a raw watch event, determining if it's a creation, modification,
     * rename, or deletion and emitting the correct event.
     */
    private handleWatchEvent;
}
//# sourceMappingURL=adapter.d.ts.map