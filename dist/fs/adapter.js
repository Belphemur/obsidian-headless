"use strict";
/**
 * @module fs/adapter
 *
 * File system adapter for vault operations.  Wraps Node.js `fs` to provide
 * a high-level interface for reading, writing, watching, and indexing vault
 * files.  Emits change events for consumption by the sync engine.
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.FileSystemAdapter = void 0;
const debounce_js_1 = require("../utils/debounce.js");
/* ------------------------------------------------------------------ */
/*  FileSystemAdapter                                                  */
/* ------------------------------------------------------------------ */
/**
 * File system adapter for vault operations.
 *
 * Provides a unified interface for reading, writing, listing, watching,
 * and indexing files within an Obsidian vault directory.
 */
class FileSystemAdapter {
    /** In-memory file index mapping vault-relative paths to stat info. */
    files = {};
    /** Debounced function that triggers a kill timeout (60 seconds). */
    thingsHappening;
    /** Whether the underlying file system is case-insensitive. */
    insensitive = false;
    /** Event handler callback, set via {@link watch}. */
    handler = null;
    fsModule;
    pathModule;
    urlModule;
    trash;
    btime;
    resourcePathPrefix;
    basePath;
    watchers = [];
    kill;
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
    constructor(fsModule, pathModule, urlModule, trash, btime, resourcePathPrefix, basePath) {
        this.fsModule = fsModule;
        this.pathModule = pathModule;
        this.urlModule = urlModule;
        this.trash = trash;
        this.btime = btime;
        this.resourcePathPrefix = resourcePathPrefix;
        this.basePath = pathModule.resolve(basePath);
        // Kill function placeholder (no-op; can be overridden externally)
        this.kill = () => { };
        this.thingsHappening = (0, debounce_js_1.debounce)(() => {
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
    getName() {
        return this.pathModule.basename(this.basePath);
    }
    /**
     * Get the absolute path to the vault root.
     */
    getBasePath() {
        return this.basePath;
    }
    /**
     * Resolve a vault-relative path to an absolute file system path.
     *
     * @param relativePath - Vault-relative path.
     * @returns Absolute path on disk.
     */
    getFullRealPath(relativePath) {
        return this.pathModule.join(this.basePath, relativePath);
    }
    /* ---------------------------------------------------------------- */
    /*  Listing                                                          */
    /* ---------------------------------------------------------------- */
    /**
     * Recursively scan all files in the vault, populating {@link files}.
     * Skips hidden files and directories (names starting with `.`).
     */
    async listAll() {
        this.files = {};
        await this.listRecursive("");
        this.thingsHappening();
    }
    /**
     * Recursively scan a directory, adding entries to {@link files}.
     *
     * @param dir - Vault-relative directory path to scan.
     */
    async listRecursive(dir) {
        const fullPath = dir
            ? this.pathModule.join(this.basePath, dir)
            : this.basePath;
        let entries;
        try {
            entries = this.fsModule.readdirSync(fullPath, { withFileTypes: true });
        }
        catch {
            return;
        }
        for (const entry of entries) {
            // Skip hidden files/directories
            if (entry.name.startsWith("."))
                continue;
            const relativePath = dir
                ? `${dir}/${entry.name}`
                : entry.name;
            const entryFullPath = this.pathModule.join(fullPath, entry.name);
            if (entry.isDirectory()) {
                this.files[relativePath] = {
                    type: "folder",
                    realpath: entryFullPath,
                };
                await this.listRecursiveChild(relativePath);
            }
            else if (entry.isFile()) {
                try {
                    const stat = this.fsModule.statSync(entryFullPath);
                    this.files[relativePath] = {
                        type: "file",
                        realpath: entryFullPath,
                        ctime: stat.ctimeMs,
                        mtime: stat.mtimeMs,
                        size: stat.size,
                    };
                }
                catch {
                    // File may have been removed between readdir and stat
                }
            }
        }
        this.thingsHappening();
    }
    /**
     * Recursively scan a child directory (same as listRecursive but used
     * internally to avoid redundant root-level checks).
     */
    async listRecursiveChild(dir) {
        const fullPath = this.pathModule.join(this.basePath, dir);
        let entries;
        try {
            entries = this.fsModule.readdirSync(fullPath, { withFileTypes: true });
        }
        catch {
            return;
        }
        for (const entry of entries) {
            // Skip hidden files/directories
            if (entry.name.startsWith("."))
                continue;
            const relativePath = `${dir}/${entry.name}`;
            const entryFullPath = this.pathModule.join(fullPath, entry.name);
            if (entry.isDirectory()) {
                this.files[relativePath] = {
                    type: "folder",
                    realpath: entryFullPath,
                };
                await this.listRecursiveChild(relativePath);
            }
            else if (entry.isFile()) {
                try {
                    const stat = this.fsModule.statSync(entryFullPath);
                    this.files[relativePath] = {
                        type: "file",
                        realpath: entryFullPath,
                        ctime: stat.ctimeMs,
                        mtime: stat.mtimeMs,
                        size: stat.size,
                    };
                }
                catch {
                    // File may have been removed between readdir and stat
                }
            }
        }
        this.thingsHappening();
    }
    /**
     * List the immediate contents of a directory.
     *
     * @param dir - Vault-relative directory path.
     * @returns Object containing `files` and `folders` arrays of relative paths.
     */
    async list(dir) {
        const fullPath = dir
            ? this.pathModule.join(this.basePath, dir)
            : this.basePath;
        const files = [];
        const folders = [];
        let entries;
        try {
            entries = this.fsModule.readdirSync(fullPath, { withFileTypes: true });
        }
        catch {
            return { files, folders };
        }
        for (const entry of entries) {
            if (entry.name.startsWith("."))
                continue;
            const relativePath = dir ? `${dir}/${entry.name}` : entry.name;
            if (entry.isDirectory()) {
                folders.push(relativePath);
            }
            else if (entry.isFile()) {
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
     *
     * @param filePath - Vault-relative path.
     */
    async exists(filePath) {
        const fullPath = this.getFullRealPath(filePath);
        return this.fsModule.existsSync(fullPath);
    }
    /**
     * Get stat information for a file or directory.
     *
     * @param filePath - Vault-relative path.
     * @returns Stat object with type, realpath, ctime, mtime, and size.
     */
    async stat(filePath) {
        const fullPath = this.getFullRealPath(filePath);
        try {
            const s = this.fsModule.statSync(fullPath);
            return {
                type: s.isDirectory() ? "folder" : "file",
                realpath: fullPath,
                ctime: s.ctimeMs,
                mtime: s.mtimeMs,
                size: s.size,
            };
        }
        catch {
            return null;
        }
    }
    /**
     * Read a file as UTF-8 text.
     *
     * @param filePath - Vault-relative path.
     * @returns File contents as a string.
     */
    async read(filePath) {
        const fullPath = this.getFullRealPath(filePath);
        return this.fsModule.readFileSync(fullPath, "utf-8");
    }
    /**
     * Read a file as binary data.
     *
     * @param filePath - Vault-relative path.
     * @returns File contents as an ArrayBuffer.
     */
    async readBinary(filePath) {
        const fullPath = this.getFullRealPath(filePath);
        const buffer = this.fsModule.readFileSync(fullPath);
        return buffer.buffer.slice(buffer.byteOffset, buffer.byteOffset + buffer.byteLength);
    }
    /**
     * Write text content to a file, creating parent directories as needed.
     * Applies ctime/mtime from options when provided.
     *
     * @param filePath - Vault-relative path.
     * @param content  - UTF-8 text content to write.
     * @param options  - Optional timestamp overrides.
     */
    async write(filePath, content, options) {
        const fullPath = this.getFullRealPath(filePath);
        const dir = this.pathModule.dirname(fullPath);
        this.fsModule.mkdirSync(dir, { recursive: true });
        this.fsModule.writeFileSync(fullPath, content, "utf-8");
        this.applyTimestamps(fullPath, options);
    }
    /**
     * Write binary data to a file, creating parent directories as needed.
     * Applies ctime/mtime from options when provided.
     *
     * @param filePath - Vault-relative path.
     * @param data     - Binary data to write.
     * @param options  - Optional timestamp overrides.
     */
    async writeBinary(filePath, data, options) {
        const fullPath = this.getFullRealPath(filePath);
        const dir = this.pathModule.dirname(fullPath);
        this.fsModule.mkdirSync(dir, { recursive: true });
        this.fsModule.writeFileSync(fullPath, Buffer.from(data));
        this.applyTimestamps(fullPath, options);
    }
    /**
     * Append text content to a file.
     *
     * @param filePath - Vault-relative path.
     * @param content  - Text to append.
     */
    async append(filePath, content) {
        const fullPath = this.getFullRealPath(filePath);
        this.fsModule.appendFileSync(fullPath, content, "utf-8");
    }
    /**
     * Create a directory (recursively).
     *
     * @param dirPath - Vault-relative directory path.
     */
    async mkdir(dirPath) {
        const fullPath = this.getFullRealPath(dirPath);
        this.fsModule.mkdirSync(fullPath, { recursive: true });
    }
    /**
     * Remove (unlink) a file.
     *
     * @param filePath - Vault-relative path.
     */
    async remove(filePath) {
        const fullPath = this.getFullRealPath(filePath);
        this.fsModule.unlinkSync(fullPath);
    }
    /**
     * Remove a directory.
     *
     * @param dirPath   - Vault-relative directory path.
     * @param recursive - Whether to remove contents recursively.
     */
    async rmdir(dirPath, recursive) {
        const fullPath = this.getFullRealPath(dirPath);
        this.fsModule.rmSync(fullPath, { recursive: recursive ?? false });
    }
    /**
     * Rename (move) a file or directory.
     *
     * @param oldPath - Current vault-relative path.
     * @param newPath - New vault-relative path.
     */
    async rename(oldPath, newPath) {
        const fullOld = this.getFullRealPath(oldPath);
        const fullNew = this.getFullRealPath(newPath);
        const dir = this.pathModule.dirname(fullNew);
        this.fsModule.mkdirSync(dir, { recursive: true });
        this.fsModule.renameSync(fullOld, fullNew);
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
    async watch(handler) {
        this.handler = handler;
        await this.listAll();
        this.watchDirectory(this.basePath);
    }
    /**
     * Stop all file system watchers.
     */
    stopWatch() {
        for (const watcher of this.watchers) {
            watcher.close();
        }
        this.watchers = [];
        this.handler = null;
    }
    /**
     * Trigger a file event via the registered handler.
     *
     * @param event   - Event name (e.g. "file-created", "modified").
     * @param filePath - Vault-relative path of the affected file.
     * @param oldPath - Previous path (for rename events).
     * @param stat    - Stat info for the file.
     */
    trigger(event, filePath, oldPath, stat) {
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
    reconcileDeletion(filePath) {
        const entry = this.files[filePath];
        if (!entry)
            return;
        const fullPath = this.getFullRealPath(filePath);
        if (!this.fsModule.existsSync(fullPath)) {
            const type = entry.type;
            delete this.files[filePath];
            if (type === "folder") {
                this.trigger("folder-removed", filePath);
            }
            else {
                this.trigger("file-removed", filePath);
            }
        }
    }
    /* ---------------------------------------------------------------- */
    /*  Private helpers                                                  */
    /* ---------------------------------------------------------------- */
    /**
     * Detect whether the file system is case-insensitive by creating a
     * test file and checking if its lowercase variant exists.
     */
    detectCaseSensitivity() {
        const testFile = this.pathModule.join(this.basePath, ".OBSIDIANTEST");
        try {
            this.fsModule.writeFileSync(testFile, "");
            const lowerPath = this.pathModule.join(this.basePath, ".obsidiantest");
            this.insensitive = this.fsModule.existsSync(lowerPath);
            this.fsModule.unlinkSync(testFile);
        }
        catch {
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
    applyTimestamps(fullPath, options) {
        if (!options)
            return;
        if (options.ctime && this.btime) {
            // Set birth time using native function (macOS/Windows)
            this.btime(fullPath, options.ctime);
        }
        if (options.mtime) {
            const mtime = new Date(options.mtime);
            const atime = new Date(); // access time = now
            this.fsModule.utimesSync(fullPath, atime, mtime);
        }
    }
    /**
     * Set up a file system watcher on a directory.
     *
     * @param dirPath - Absolute path to the directory to watch.
     */
    watchDirectory(dirPath) {
        try {
            const watcher = this.fsModule.watch(dirPath, { recursive: true }, (eventType, filename) => {
                if (!filename)
                    return;
                // Skip hidden files
                if (filename.startsWith(".") ||
                    filename.includes(`${this.pathModule.sep}.`)) {
                    return;
                }
                // Normalise path separators to forward slashes
                const relativePath = filename.replace(/\\/g, "/");
                this.handleWatchEvent(eventType, relativePath);
            });
            this.watchers.push(watcher);
        }
        catch {
            // Directory may not exist or permissions may be insufficient
        }
    }
    /**
     * Process a raw watch event, determining if it's a creation, modification,
     * rename, or deletion and emitting the correct event.
     */
    handleWatchEvent(eventType, relativePath) {
        const fullPath = this.getFullRealPath(relativePath);
        const existed = relativePath in this.files;
        // Emit raw event
        this.trigger("raw", relativePath);
        if (!this.fsModule.existsSync(fullPath)) {
            // File/folder was deleted
            if (existed) {
                const entry = this.files[relativePath];
                delete this.files[relativePath];
                if (entry.type === "folder") {
                    this.trigger("folder-removed", relativePath);
                }
                else {
                    this.trigger("file-removed", relativePath);
                }
            }
            return;
        }
        try {
            const s = this.fsModule.statSync(fullPath);
            const stat = { ctime: s.ctimeMs, mtime: s.mtimeMs, size: s.size };
            if (s.isDirectory()) {
                if (!existed) {
                    this.files[relativePath] = {
                        type: "folder",
                        realpath: fullPath,
                    };
                    this.trigger("folder-created", relativePath);
                }
            }
            else {
                if (!existed) {
                    this.files[relativePath] = {
                        type: "file",
                        realpath: fullPath,
                        ctime: stat.ctime,
                        mtime: stat.mtime,
                        size: stat.size,
                    };
                    this.trigger("file-created", relativePath, undefined, stat);
                }
                else {
                    this.files[relativePath] = {
                        type: "file",
                        realpath: fullPath,
                        ctime: stat.ctime,
                        mtime: stat.mtime,
                        size: stat.size,
                    };
                    this.trigger("modified", relativePath, undefined, stat);
                }
            }
        }
        catch {
            // Stat failed; file may have been removed immediately
        }
    }
}
exports.FileSystemAdapter = FileSystemAdapter;
//# sourceMappingURL=adapter.js.map