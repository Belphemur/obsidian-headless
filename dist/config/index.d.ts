/**
 * @module config
 *
 * Configuration management for the Obsidian Headless CLI.
 * Handles auth tokens, sync vault configs, publish site configs,
 * validation helpers, and log file setup.
 */
/** Application name used for config directory naming. */
export declare const APP_NAME = "obsidian-headless";
/** Valid file types for sync allow-types. */
export declare const VALID_FILE_TYPES: readonly ["image", "audio", "video", "pdf", "unsupported"];
/** Default file types included in sync. */
export declare const DEFAULT_FILE_TYPES: readonly ["image", "audio", "pdf", "video"];
/** Valid config categories for sync special files. */
export declare const VALID_CONFIG_CATEGORIES: readonly ["app", "appearance", "appearance-data", "hotkey", "core-plugin", "core-plugin-data", "community-plugin", "community-plugin-data"];
/** Default config categories included in sync. */
export declare const DEFAULT_CONFIG_CATEGORIES: readonly ["app", "appearance", "appearance-data", "hotkey", "core-plugin", "core-plugin-data"];
/** Configuration for a synced vault. */
export interface SyncConfig {
    vaultId: string;
    vaultName: string;
    vaultPath: string;
    host: string;
    encryptionVersion: number;
    encryptionKey: string;
    encryptionSalt: string;
    conflictStrategy: string;
    syncMode?: string;
    deviceName?: string;
    configDir?: string;
    allowTypes?: string[];
    allowSpecialFiles?: string[];
    ignoreFolders?: string[];
}
/** Configuration for a publish site. */
export interface PublishConfig {
    siteId: string;
    host: string;
    vaultPath: string;
    includes?: string[];
    excludes?: string[];
}
/**
 * Returns the base configuration directory for this application.
 *
 * - Linux: `$XDG_CONFIG_HOME/obsidian-headless` or `~/.config/obsidian-headless`
 * - macOS/Windows: `~/.obsidian-headless`
 */
export declare function getBaseDir(): string;
/**
 * Retrieve the auth token. Checks the `OBSIDIAN_AUTH_TOKEN` environment
 * variable first, then falls back to the token file on disk.
 */
export declare function getAuthToken(): string | null;
/**
 * Save the auth token to disk. Creates the config directory (mode 0o700)
 * and writes the token file with restricted permissions (mode 0o600).
 */
export declare function saveAuthToken(token: string): void;
/**
 * Remove the auth token file from disk.
 */
export declare function clearAuthToken(): void;
/**
 * Returns the sync directory for a given vault ID.
 */
export declare function getSyncDir(vaultId: string): string;
/**
 * Returns the state database path for a given vault ID.
 */
export declare function getStatePath(vaultId: string): string;
/**
 * Returns the sync log file path for a given vault ID.
 */
export declare function getLogPath(vaultId: string): string;
/**
 * Returns the sync config file path for a given vault ID.
 */
export declare function getSyncConfigPath(vaultId: string): string;
/**
 * Ensures the sync directory exists for a given vault ID.
 */
export declare function ensureSyncDir(vaultId: string): void;
/**
 * Reads and parses the sync config for a given vault ID.
 * Returns `null` if the config file does not exist or cannot be parsed.
 */
export declare function readSyncConfig(vaultId: string): SyncConfig | null;
/**
 * Writes the sync config for a given vault ID to disk.
 */
export declare function writeSyncConfig(vaultId: string, config: SyncConfig): void;
/**
 * Returns the lock file path for a sync vault.
 *
 * @param vaultPath - Absolute path to the vault root
 * @param configDir - Config directory name (defaults to ".obsidian")
 */
export declare function getLockPath(vaultPath: string, configDir?: string): string;
/**
 * Removes the entire sync configuration directory for a given vault ID.
 */
export declare function removeSyncConfig(vaultId: string): void;
/**
 * Lists all vault IDs that have a local sync configuration.
 */
export declare function listLocalVaults(): string[];
/**
 * Finds the sync config whose `vaultPath` matches the given local path.
 * Returns `null` if no matching vault is found.
 */
export declare function findSyncConfigByPath(localPath: string): SyncConfig | null;
/**
 * Returns a default device name in the format "hostname (platform)".
 */
export declare function getDefaultDeviceName(): string;
/**
 * Returns the publish directory for a given site ID.
 */
export declare function getPublishDir(siteId: string): string;
/**
 * Returns the publish config file path for a given site ID.
 */
export declare function getPublishConfigPath(siteId: string): string;
/**
 * Returns the publish cache file path for a given site ID.
 */
export declare function getPublishCachePath(siteId: string): string;
/**
 * Ensures the publish directory exists for a given site ID.
 */
export declare function ensurePublishDir(siteId: string): void;
/**
 * Reads and parses the publish config for a given site ID.
 * Returns `null` if the config file does not exist or cannot be parsed.
 */
export declare function readPublishConfig(siteId: string): PublishConfig | null;
/**
 * Writes the publish config for a given site ID to disk.
 */
export declare function writePublishConfig(siteId: string, config: PublishConfig): void;
/**
 * Removes the entire publish configuration directory for a given site ID.
 */
export declare function removePublishConfig(siteId: string): void;
/**
 * Lists all site IDs that have a local publish configuration.
 */
export declare function listPublishSites(): string[];
/**
 * Finds the publish config whose `vaultPath` matches the given local path.
 * Returns `null` if no matching site is found.
 */
export declare function findPublishConfigByPath(localPath: string): PublishConfig | null;
/**
 * Parses a comma-separated list of file types and validates each entry
 * against the known valid file types.
 *
 * @throws {Error} If any entry is not a valid file type.
 */
export declare function parseFileTypes(input: string): string[];
/**
 * Parses a comma-separated list of config categories and validates each
 * entry against the known valid categories.
 *
 * @throws {Error} If any entry is not a valid config category.
 */
export declare function parseConfigCategories(input: string): string[];
/**
 * Validates a config directory name. Must start with "." and not contain
 * any path separators.
 *
 * @returns The validated directory name, or `undefined` if input is undefined.
 * @throws {Error} If the directory name is invalid.
 */
export declare function validateConfigDir(dir?: string): string | undefined;
/**
 * Sets up logging to a file. Wraps `console.log`, `console.warn`,
 * `console.error`, and `console.debug` to also write timestamped
 * entries to the specified log file.
 *
 * @returns A cleanup function that restores the original console methods
 *          and closes the write stream.
 */
export declare function setupLogFile(logPath: string): () => void;
//# sourceMappingURL=index.d.ts.map