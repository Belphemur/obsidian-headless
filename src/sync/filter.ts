/**
 * @module sync/filter
 *
 * File sync filter that determines which vault files should be synced
 * based on file type, special config files, and ignore rules.
 */

import {
  extname,
  IMAGE_EXTS,
  AUDIO_EXTS,
  VIDEO_EXTS,
  PDF_EXTS,
} from "../utils/paths.js";

/* ------------------------------------------------------------------ */
/*  Constants                                                          */
/* ------------------------------------------------------------------ */

/** All valid file type categories that can be toggled for sync. */
export const VALID_FILE_TYPES = [
  "image",
  "audio",
  "video",
  "pdf",
  "unsupported",
];

/** Default set of file types synced when no explicit configuration is given. */
export const DEFAULT_FILE_TYPES = ["image", "audio", "pdf", "video"];

/** All valid special file categories within the config directory. */
export const VALID_SPECIAL_FILES = [
  "app",
  "appearance",
  "appearance-data",
  "hotkey",
  "core-plugin",
  "core-plugin-data",
  "community-plugin",
  "community-plugin-data",
];

/** Default set of special file categories synced. */
export const DEFAULT_SPECIAL_FILES = [
  "app",
  "appearance",
  "appearance-data",
  "hotkey",
  "core-plugin",
  "core-plugin-data",
];

/* ------------------------------------------------------------------ */
/*  Plugin file detection                                              */
/* ------------------------------------------------------------------ */

const PLUGIN_FILES = new Set([
  "manifest.json",
  "main.js",
  "styles.css",
  "data.json",
]);

/* ------------------------------------------------------------------ */
/*  SyncFilter                                                         */
/* ------------------------------------------------------------------ */

export class SyncFilter {
  /** Allowed attachment type categories. */
  allowTypes: Set<string> = new Set(DEFAULT_FILE_TYPES);

  /** Allowed special config-directory file categories. */
  allowSpecialFiles: Set<string> = new Set();

  /** Folder names to completely ignore during sync. */
  ignoreFolders: string[] = [];

  /** Cache of path → allowed decision to avoid repeated computation. */
  filterCache: Record<string, boolean> = {};

  /** The vault's configuration directory name (e.g. ".obsidian"). */
  configDir: string;

  constructor(configDir: string) {
    this.configDir = configDir;
  }

  /**
   * Initialise the filter with explicit allow-lists and ignore rules.
   * If parameters are omitted the current values are kept.
   */
  init(
    allowTypes?: string[],
    allowSpecialFiles?: string[],
    ignoreFolders?: string[],
  ): void {
    if (allowTypes !== undefined) {
      this.allowTypes = new Set(allowTypes);
    }
    if (allowSpecialFiles !== undefined) {
      this.allowSpecialFiles = new Set(allowSpecialFiles);
    }
    if (ignoreFolders !== undefined) {
      this.ignoreFolders = ignoreFolders;
    }
    this.clearCache();
  }

  /** Reset all settings back to defaults. */
  clear(): void {
    this.allowTypes = new Set(DEFAULT_FILE_TYPES);
    this.allowSpecialFiles = new Set();
    this.ignoreFolders = [];
    this.filterCache = {};
  }

  /** Clear only the decision cache, leaving settings intact. */
  clearCache(): void {
    this.filterCache = {};
  }

  /**
   * Determine whether a file or folder at `path` should be synced.
   * Results are cached for repeated lookups on the same path.
   */
  allowSyncFile(path: string, isFolder: boolean): boolean {
    const key = path + (isFolder ? "/" : "");
    if (key in this.filterCache) {
      return this.filterCache[key];
    }
    const allowed = this._allowSyncFile(path, isFolder);
    this.filterCache[key] = allowed;
    return allowed;
  }

  /**
   * Core filter logic (uncached).
   * @internal
   */
  _allowSyncFile(path: string, isFolder: boolean): boolean {
    // 1. Check ignoreFolders
    for (const folder of this.ignoreFolders) {
      if (isFolder && path === folder) return false;
      if (path.startsWith(folder + "/")) return false;
    }

    // 2. Config directory special handling (files only)
    if (!isFolder && path.startsWith(this.configDir + "/")) {
      return this._allowConfigFile(path);
    }

    // 3. Hidden paths (start with ".") are not allowed
    if (path.startsWith(".")) return false;

    // 4. Folders are always allowed (if not ignored above)
    if (isFolder) return true;

    // 5. Check extension-based type rules
    return this._allowByExtension(path);
  }

  /**
   * Evaluate whether a file inside the config directory should be synced.
   */
  private _allowConfigFile(path: string): boolean {
    const relative = path.slice(this.configDir.length + 1);
    const segments = relative.split("/");

    // Skip node_modules or segments starting with "."
    for (const seg of segments) {
      if (seg === "node_modules" || seg.startsWith(".")) return false;
    }

    // Skip workspace files
    if (relative === "workspace.json" || relative === "workspace-mobile.json") {
      return false;
    }

    // Determine category
    const category = this._categorizeConfigFile(relative, segments);
    if (category === null) return false;

    return this.allowSpecialFiles.has(category);
  }

  /**
   * Map a config-relative path to its special file category.
   * Returns null if the file doesn't match any known category.
   */
  private _categorizeConfigFile(
    relative: string,
    segments: string[],
  ): string | null {
    // Root-level known files
    if (segments.length === 1) {
      const file = segments[0];
      if (file === "app.json" || file === "types.json") return "app";
      if (file === "appearance.json") return "appearance";
      if (file === "hotkeys.json") return "hotkey";
      if (
        file === "core-plugins.json" ||
        file === "core-plugins-migration.json"
      )
        return "core-plugin";
      if (file === "community-plugins.json") return "community-plugin";
      // Any other .json at root → core-plugin-data
      if (file.endsWith(".json")) return "core-plugin-data";
      return null;
    }

    // themes/{name}/theme.css or themes/{name}/manifest.json
    if (
      segments.length === 3 &&
      segments[0] === "themes" &&
      (segments[2] === "theme.css" || segments[2] === "manifest.json")
    ) {
      return "appearance-data";
    }

    // snippets/{name}.css
    if (
      segments.length === 2 &&
      segments[0] === "snippets" &&
      segments[1].endsWith(".css")
    ) {
      return "appearance-data";
    }

    // plugins/{name}/{file} where file is a known plugin file
    if (
      segments.length === 3 &&
      segments[0] === "plugins" &&
      this.isPluginFile(segments[2])
    ) {
      return "community-plugin-data";
    }

    return null;
  }

  /**
   * Check whether a file is allowed based on its extension.
   */
  private _allowByExtension(path: string): boolean {
    const ext = extname(path);

    // Markdown, canvas, base files are always allowed
    if (ext === "md" || ext === "canvas" || ext === "base") return true;

    // Image extensions
    if (IMAGE_EXTS.includes(ext)) return this.allowTypes.has("image");

    // "webm" can be audio or video
    if (ext === "webm") {
      return this.allowTypes.has("audio") || this.allowTypes.has("video");
    }

    // Audio extensions
    if (AUDIO_EXTS.includes(ext)) return this.allowTypes.has("audio");

    // Video extensions
    if (VIDEO_EXTS.includes(ext)) return this.allowTypes.has("video");

    // PDF extensions
    if (PDF_EXTS.includes(ext)) return this.allowTypes.has("pdf");

    // Everything else requires "unsupported"
    return this.allowTypes.has("unsupported");
  }

  /**
   * Check if a filename is a recognised community plugin file.
   */
  isPluginFile(filename: string): boolean {
    return PLUGIN_FILES.has(filename);
  }
}
