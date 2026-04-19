"use strict";
/**
 * @module publish/engine
 *
 * The publish engine scans for local file changes and uploads/removes files
 * from the Obsidian Publish service.  It maintains a local cache of file
 * hashes and metadata to enable efficient incremental publishes.
 */
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
exports.PublishEngine = void 0;
const node_fs_1 = __importDefault(require("node:fs"));
const node_path_1 = __importDefault(require("node:path"));
const yaml_1 = require("yaml");
const crypto_js_1 = require("../utils/crypto.js");
const encoding_js_1 = require("../utils/encoding.js");
const paths_js_1 = require("../utils/paths.js");
const client_js_1 = require("../api/client.js");
const index_js_1 = require("../config/index.js");
/* ------------------------------------------------------------------ */
/*  Constants                                                          */
/* ------------------------------------------------------------------ */
/** Maximum upload size in bytes (50 MB). */
const MAX_UPLOAD_SIZE = 50 * 1024 * 1024;
/** Special files that are always eligible for publishing. */
const PUBLISH_SPECIAL_FILES = [
    "obsidian.css",
    "publish.css",
    "favicon.ico",
    "publish.js",
];
/* ------------------------------------------------------------------ */
/*  PublishEngine                                                       */
/* ------------------------------------------------------------------ */
/**
 * Coordinates scanning, uploading, and removing files for Obsidian Publish.
 *
 * @example
 * ```ts
 * const engine = new PublishEngine(token, config);
 * const changes = await engine.scanForChanges();
 * await engine.publish(changes, (cur, total) => console.log(`${cur}/${total}`));
 * ```
 */
class PublishEngine {
    token;
    config;
    cache;
    constructor(token, config) {
        this.token = token;
        this.config = config;
        this.cache = {};
        this.loadCache();
    }
    /* ---------------------------------------------------------------- */
    /*  Public methods                                                   */
    /* ---------------------------------------------------------------- */
    /**
     * Scans the local vault and compares it with the server file list to
     * determine which files need to be published, updated, or removed.
     *
     * @param includeAll - When true, publish all supported files regardless
     *   of frontmatter or folder rules.
     * @returns A list of changes to apply.
     */
    async scanForChanges(includeAll) {
        const serverResponse = await (0, client_js_1.listPublishFiles)(this.token, this.config.host, this.config.siteId);
        const serverFiles = (serverResponse.files ?? []);
        const serverMap = new Map();
        for (const sf of serverFiles) {
            serverMap.set(sf.path, sf.hash);
        }
        const localFiles = this.walkLocalFiles();
        const changes = [];
        const localPaths = new Set();
        for (const file of localFiles) {
            localPaths.add(file.path);
            // Check cache
            const cached = this.cache[file.path];
            let hash;
            let publishFlag;
            if (cached && cached.mtime === file.mtime && cached.size === file.size) {
                hash = cached.hash;
                publishFlag = cached.publish;
            }
            else {
                const fullPath = node_path_1.default.join(this.config.vaultPath, file.path);
                const content = node_fs_1.default.readFileSync(fullPath);
                hash = await (0, crypto_js_1.sha256Hex)((0, encoding_js_1.toArrayBuffer)(content));
                publishFlag = null;
                if (file.path.endsWith(".md")) {
                    publishFlag = this.parseFrontmatterPublishFlag(content.toString("utf-8"));
                }
                this.cache[file.path] = {
                    mtime: file.mtime,
                    size: file.size,
                    hash,
                    publish: publishFlag,
                };
            }
            // Determine if file should be published
            const shouldPublish = includeAll
                ? true
                : publishFlag ?? this.getFolderPublishFlag(file.path) ?? false;
            if (!shouldPublish)
                continue;
            // Compare with server
            const serverHash = serverMap.get(file.path);
            if (serverHash === hash)
                continue;
            if (serverHash === undefined) {
                changes.push({ path: file.path, type: "new" });
            }
            else {
                changes.push({ path: file.path, type: "changed" });
            }
        }
        // Detect deleted files (on server but not local)
        for (const serverPath of serverMap.keys()) {
            if (!localPaths.has(serverPath)) {
                changes.push({ path: serverPath, type: "deleted" });
            }
        }
        this.saveCache();
        return changes;
    }
    /**
     * Uploads a single file to the publish site.
     *
     * @param filePath - Vault-relative path to the file.
     * @throws If the file exceeds the 50 MB upload limit.
     */
    async uploadFile(filePath) {
        const fullPath = node_path_1.default.join(this.config.vaultPath, filePath);
        const content = node_fs_1.default.readFileSync(fullPath);
        if (content.byteLength > MAX_UPLOAD_SIZE) {
            throw new Error(`File exceeds 50 MB upload limit: ${filePath} (${content.byteLength} bytes)`);
        }
        const hash = await (0, crypto_js_1.sha256Hex)((0, encoding_js_1.toArrayBuffer)(content));
        await (0, client_js_1.uploadPublishFile)(this.token, this.config.host, this.config.siteId, filePath, hash, content);
    }
    /**
     * Removes a file from the publish site.
     *
     * @param filePath - Vault-relative path to the file.
     */
    async removeFile(filePath) {
        await (0, client_js_1.removePublishFile)(this.token, this.config.host, this.config.siteId, filePath);
    }
    /**
     * Applies a list of changes by uploading or removing files.
     *
     * @param changes - The list of changes to apply.
     * @param onProgress - Optional callback invoked after each file is processed.
     */
    async publish(changes, onProgress) {
        const total = changes.length;
        for (let i = 0; i < total; i++) {
            const change = changes[i];
            if (change.type === "deleted") {
                await this.removeFile(change.path);
            }
            else {
                await this.uploadFile(change.path);
            }
            if (onProgress) {
                onProgress(i + 1, total);
            }
        }
    }
    /* ---------------------------------------------------------------- */
    /*  File system helpers                                              */
    /* ---------------------------------------------------------------- */
    /**
     * Recursively walks the vault directory and returns all publishable files
     * with their metadata.
     */
    walkLocalFiles() {
        const results = [];
        this.walkDir(this.config.vaultPath, "", results);
        return results;
    }
    walkDir(root, relative, results) {
        const dirPath = relative ? node_path_1.default.join(root, relative) : root;
        const entries = node_fs_1.default.readdirSync(dirPath, { withFileTypes: true });
        for (const entry of entries) {
            // Skip hidden files and directories
            if (entry.name.startsWith("."))
                continue;
            const entryRelative = relative
                ? `${relative}/${entry.name}`
                : entry.name;
            if (entry.isDirectory()) {
                this.walkDir(root, entryRelative, results);
            }
            else if (entry.isFile()) {
                if (!this.isFileSupported(entry.name, entryRelative))
                    continue;
                const stat = node_fs_1.default.statSync(node_path_1.default.join(dirPath, entry.name));
                results.push({
                    path: entryRelative,
                    mtime: Math.round(stat.mtimeMs),
                    size: stat.size,
                });
            }
        }
    }
    /**
     * Checks if a file is eligible for publishing based on its name or extension.
     */
    isFileSupported(filename, filePath) {
        if (PUBLISH_SPECIAL_FILES.includes(filename))
            return true;
        const dotIdx = filePath.lastIndexOf(".");
        if (dotIdx === -1)
            return false;
        const ext = filePath.slice(dotIdx + 1).toLowerCase();
        return (0, paths_js_1.isKnownExtension)(ext);
    }
    /* ---------------------------------------------------------------- */
    /*  Publish flag resolution                                          */
    /* ---------------------------------------------------------------- */
    /**
     * Determines the publish flag based on folder include/exclude rules.
     *
     * @returns `false` if excluded, `true` if included, `null` if no rule matches.
     */
    getFolderPublishFlag(filePath) {
        const excludes = this.config.excludes ?? [];
        for (const folder of excludes) {
            if (filePath.startsWith(folder + "/") || filePath === folder) {
                return false;
            }
        }
        const includes = this.config.includes ?? [];
        for (const folder of includes) {
            if (filePath.startsWith(folder + "/") || filePath === folder) {
                return true;
            }
        }
        return null;
    }
    /**
     * Parses the YAML frontmatter of a Markdown file to extract the `publish`
     * flag.
     *
     * @returns `true` if publish is enabled, `false` if explicitly disabled,
     *   or `null` if no frontmatter or no publish key.
     */
    parseFrontmatterPublishFlag(content) {
        if (!content.startsWith("---"))
            return null;
        const endIdx = content.indexOf("\n---", 3);
        if (endIdx === -1)
            return null;
        const yamlBlock = content.slice(4, endIdx);
        let frontmatter;
        try {
            frontmatter = (0, yaml_1.parse)(yamlBlock);
        }
        catch {
            return null;
        }
        if (frontmatter == null || typeof frontmatter !== "object")
            return null;
        const value = frontmatter["publish"];
        if (value === undefined || value === null)
            return null;
        if (typeof value === "string") {
            const lower = value.toLowerCase();
            if (lower === "false" || lower === "no")
                return false;
            if (lower === "true" || lower === "yes")
                return true;
        }
        if (typeof value === "boolean")
            return value;
        return value ? true : null;
    }
    /* ---------------------------------------------------------------- */
    /*  Cache persistence                                                */
    /* ---------------------------------------------------------------- */
    /** Loads the publish cache from disk. */
    loadCache() {
        try {
            const cachePath = (0, index_js_1.getPublishCachePath)(this.config.siteId);
            const raw = node_fs_1.default.readFileSync(cachePath, "utf-8");
            this.cache = JSON.parse(raw);
        }
        catch {
            this.cache = {};
        }
    }
    /** Saves the publish cache to disk. */
    saveCache() {
        (0, index_js_1.ensurePublishDir)(this.config.siteId);
        const cachePath = (0, index_js_1.getPublishCachePath)(this.config.siteId);
        node_fs_1.default.writeFileSync(cachePath, JSON.stringify(this.cache), { mode: 0o600 });
    }
}
exports.PublishEngine = PublishEngine;
//# sourceMappingURL=engine.js.map