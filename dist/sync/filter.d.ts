/**
 * @module sync/filter
 *
 * File sync filter that determines which vault files should be synced
 * based on file type, special config files, and ignore rules.
 */
/** All valid file type categories that can be toggled for sync. */
export declare const VALID_FILE_TYPES: string[];
/** Default set of file types synced when no explicit configuration is given. */
export declare const DEFAULT_FILE_TYPES: string[];
/** All valid special file categories within the config directory. */
export declare const VALID_SPECIAL_FILES: string[];
/** Default set of special file categories synced. */
export declare const DEFAULT_SPECIAL_FILES: string[];
export declare class SyncFilter {
    /** Allowed attachment type categories. */
    allowTypes: Set<string>;
    /** Allowed special config-directory file categories. */
    allowSpecialFiles: Set<string>;
    /** Folder names to completely ignore during sync. */
    ignoreFolders: string[];
    /** Cache of path → allowed decision to avoid repeated computation. */
    filterCache: Record<string, boolean>;
    /** The vault's configuration directory name (e.g. ".obsidian"). */
    configDir: string;
    constructor(configDir: string);
    /**
     * Initialise the filter with explicit allow-lists and ignore rules.
     * If parameters are omitted the current values are kept.
     */
    init(allowTypes?: string[], allowSpecialFiles?: string[], ignoreFolders?: string[]): void;
    /** Reset all settings back to defaults. */
    clear(): void;
    /** Clear only the decision cache, leaving settings intact. */
    clearCache(): void;
    /**
     * Determine whether a file or folder at `path` should be synced.
     * Results are cached for repeated lookups on the same path.
     */
    allowSyncFile(path: string, isFolder: boolean): boolean;
    /**
     * Core filter logic (uncached).
     * @internal
     */
    _allowSyncFile(path: string, isFolder: boolean): boolean;
    /**
     * Evaluate whether a file inside the config directory should be synced.
     */
    private _allowConfigFile;
    /**
     * Map a config-relative path to its special file category.
     * Returns null if the file doesn't match any known category.
     */
    private _categorizeConfigFile;
    /**
     * Check whether a file is allowed based on its extension.
     */
    private _allowByExtension;
    /**
     * Check if a filename is a recognised community plugin file.
     */
    isPluginFile(filename: string): boolean;
}
//# sourceMappingURL=filter.d.ts.map