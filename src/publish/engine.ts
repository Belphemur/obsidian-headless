/**
 * @module publish/engine
 *
 * The publish engine scans for local file changes and uploads/removes files
 * from the Obsidian Publish service.  It maintains a local cache of file
 * hashes and metadata to enable efficient incremental publishes.
 */

import fs from "node:fs";
import path from "node:path";
import { parse as parseYaml } from "yaml";
import { sha256Hex } from "../utils/crypto.js";
import { toArrayBuffer } from "../utils/encoding.js";
import { isKnownExtension } from "../utils/paths.js";
import {
  listPublishFiles,
  uploadPublishFile,
  removePublishFile,
} from "../api/client.js";
import {
  getPublishCachePath,
  ensurePublishDir,
  type PublishConfig,
} from "../config/index.js";

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
/*  Types                                                              */
/* ------------------------------------------------------------------ */

/** Describes a single file change detected during a scan. */
export interface PublishChange {
  path: string;
  type: "new" | "changed" | "deleted";
}

/** A single entry in the local publish cache. */
interface CacheEntry {
  mtime: number;
  size: number;
  hash: string;
  publish: boolean | null;
}

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
export class PublishEngine {
  private token: string;
  private config: PublishConfig;
  private cache: Record<string, CacheEntry>;

  constructor(token: string, config: PublishConfig) {
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
  async scanForChanges(includeAll?: boolean): Promise<PublishChange[]> {
    const serverResponse = await listPublishFiles(
      this.token,
      this.config.host,
      this.config.siteId,
    );

    const serverFiles = (serverResponse.files ?? []) as Array<{
      path: string;
      hash: string;
    }>;

    const serverMap = new Map<string, string>();
    for (const sf of serverFiles) {
      serverMap.set(sf.path, sf.hash);
    }

    const localFiles = this.walkLocalFiles();
    const changes: PublishChange[] = [];
    const localPaths = new Set<string>();

    for (const file of localFiles) {
      localPaths.add(file.path);

      // Check cache
      const cached = this.cache[file.path];
      let hash: string;
      let publishFlag: boolean | null;

      if (cached && cached.mtime === file.mtime && cached.size === file.size) {
        hash = cached.hash;
        publishFlag = cached.publish;
      } else {
        const fullPath = path.join(this.config.vaultPath, file.path);
        const content = fs.readFileSync(fullPath);
        hash = await sha256Hex(toArrayBuffer(content));

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

      if (!shouldPublish) continue;

      // Compare with server
      const serverHash = serverMap.get(file.path);
      if (serverHash === hash) continue;
      if (serverHash === undefined) {
        changes.push({ path: file.path, type: "new" });
      } else {
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
  async uploadFile(filePath: string): Promise<void> {
    const fullPath = path.join(this.config.vaultPath, filePath);
    const content = fs.readFileSync(fullPath);

    if (content.byteLength > MAX_UPLOAD_SIZE) {
      throw new Error(
        `File exceeds 50 MB upload limit: ${filePath} (${content.byteLength} bytes)`,
      );
    }

    const hash = await sha256Hex(toArrayBuffer(content));

    await uploadPublishFile(
      this.token,
      this.config.host,
      this.config.siteId,
      filePath,
      hash,
      content,
    );
  }

  /**
   * Removes a file from the publish site.
   *
   * @param filePath - Vault-relative path to the file.
   */
  async removeFile(filePath: string): Promise<void> {
    await removePublishFile(
      this.token,
      this.config.host,
      this.config.siteId,
      filePath,
    );
  }

  /**
   * Applies a list of changes by uploading or removing files.
   *
   * @param changes - The list of changes to apply.
   * @param onProgress - Optional callback invoked after each file is processed.
   */
  async publish(
    changes: PublishChange[],
    onProgress?: (current: number, total: number) => void,
  ): Promise<void> {
    const total = changes.length;

    for (let i = 0; i < total; i++) {
      const change = changes[i];

      if (change.type === "deleted") {
        await this.removeFile(change.path);
      } else {
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
   *
   * Uses `readdirSync({ recursive: true, withFileTypes: true })` for a
   * single kernel call that returns the entire tree, avoiding per-directory
   * recursion overhead.
   */
  walkLocalFiles(): Array<{ path: string; mtime: number; size: number }> {
    const results: Array<{ path: string; mtime: number; size: number }> = [];

    let entries: fs.Dirent[];
    try {
      entries = fs.readdirSync(this.config.vaultPath, {
        withFileTypes: true,
        recursive: true,
      });
    } catch {
      return results;
    }

    for (const entry of entries) {
      if (!entry.isFile()) continue;

      const parentDir =
        (entry as any).parentPath ?? (entry as any).path ?? "";
      const fullPath = path.join(parentDir, entry.name);
      const relativePath = path
        .relative(this.config.vaultPath, fullPath)
        .replace(/\\/g, "/");

      // Skip hidden files/directories
      if (relativePath.split("/").some((s) => s.startsWith("."))) continue;

      if (!this.isFileSupported(entry.name, relativePath)) continue;

      try {
        const stat = fs.statSync(fullPath);
        results.push({
          path: relativePath,
          mtime: Math.round(stat.mtimeMs),
          size: stat.size,
        });
      } catch {
        // File may have disappeared between readdir and stat
      }
    }

    return results;
  }

  /**
   * Checks if a file is eligible for publishing based on its name or extension.
   */
  isFileSupported(filename: string, filePath: string): boolean {
    if (PUBLISH_SPECIAL_FILES.includes(filename)) return true;

    const dotIdx = filePath.lastIndexOf(".");
    if (dotIdx === -1) return false;
    const ext = filePath.slice(dotIdx + 1).toLowerCase();
    return isKnownExtension(ext);
  }

  /* ---------------------------------------------------------------- */
  /*  Publish flag resolution                                          */
  /* ---------------------------------------------------------------- */

  /**
   * Determines the publish flag based on folder include/exclude rules.
   *
   * @returns `false` if excluded, `true` if included, `null` if no rule matches.
   */
  getFolderPublishFlag(filePath: string): boolean | null {
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
  parseFrontmatterPublishFlag(content: string): boolean | null {
    if (!content.startsWith("---")) return null;

    const endIdx = content.indexOf("\n---", 3);
    if (endIdx === -1) return null;

    const yamlBlock = content.slice(4, endIdx);

    let frontmatter: Record<string, unknown>;
    try {
      frontmatter = parseYaml(yamlBlock) as Record<string, unknown>;
    } catch {
      return null;
    }

    if (frontmatter == null || typeof frontmatter !== "object") return null;

    const value = frontmatter["publish"];
    if (value === undefined || value === null) return null;

    if (typeof value === "string") {
      const lower = value.toLowerCase();
      if (lower === "false" || lower === "no") return false;
      if (lower === "true" || lower === "yes") return true;
    }

    if (typeof value === "boolean") return value;

    return value ? true : null;
  }

  /* ---------------------------------------------------------------- */
  /*  Cache persistence                                                */
  /* ---------------------------------------------------------------- */

  /** Loads the publish cache from disk. */
  private loadCache(): void {
    try {
      const cachePath = getPublishCachePath(this.config.siteId);
      const raw = fs.readFileSync(cachePath, "utf-8");
      this.cache = JSON.parse(raw) as Record<string, CacheEntry>;
    } catch {
      this.cache = {};
    }
  }

  /** Saves the publish cache to disk. */
  private saveCache(): void {
    ensurePublishDir(this.config.siteId);
    const cachePath = getPublishCachePath(this.config.siteId);
    fs.writeFileSync(cachePath, JSON.stringify(this.cache), { mode: 0o600 });
  }
}
