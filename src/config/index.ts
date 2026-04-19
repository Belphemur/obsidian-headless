/**
 * @module config
 *
 * Configuration management for the Obsidian Headless CLI.
 * Handles auth tokens, sync vault configs, publish site configs,
 * validation helpers, and log file setup.
 */

import fs from "node:fs";
import path from "node:path";
import os from "node:os";

/* ------------------------------------------------------------------ */
/*  Constants                                                          */
/* ------------------------------------------------------------------ */

/** Application name used for config directory naming. */
export const APP_NAME = "obsidian-headless";

/** Valid file types for sync allow-types. */
export const VALID_FILE_TYPES = [
  "image",
  "audio",
  "video",
  "pdf",
  "unsupported",
] as const;

/** Default file types included in sync. */
export const DEFAULT_FILE_TYPES = ["image", "audio", "pdf", "video"] as const;

/** Valid config categories for sync special files. */
export const VALID_CONFIG_CATEGORIES = [
  "app",
  "appearance",
  "appearance-data",
  "hotkey",
  "core-plugin",
  "core-plugin-data",
  "community-plugin",
  "community-plugin-data",
] as const;

/** Default config categories included in sync. */
export const DEFAULT_CONFIG_CATEGORIES = [
  "app",
  "appearance",
  "appearance-data",
  "hotkey",
  "core-plugin",
  "core-plugin-data",
] as const;

/* ------------------------------------------------------------------ */
/*  Types                                                              */
/* ------------------------------------------------------------------ */

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

/* ------------------------------------------------------------------ */
/*  Base directory                                                     */
/* ------------------------------------------------------------------ */

/**
 * Returns the base configuration directory for this application.
 *
 * - Linux: `$XDG_CONFIG_HOME/obsidian-headless` or `~/.config/obsidian-headless`
 * - macOS/Windows: `~/.obsidian-headless`
 */
export function getBaseDir(): string {
  if (process.platform === "linux") {
    const xdg = process.env["XDG_CONFIG_HOME"];
    const base = xdg || path.join(os.homedir(), ".config");
    return path.join(base, APP_NAME);
  }
  return path.join(os.homedir(), `.${APP_NAME}`);
}

/* ------------------------------------------------------------------ */
/*  Auth token                                                         */
/* ------------------------------------------------------------------ */

const AUTH_TOKEN_FILE = "auth_token";
const AUTH_TOKEN_ENV = "OBSIDIAN_AUTH_TOKEN";

/**
 * Retrieve the auth token. Checks the `OBSIDIAN_AUTH_TOKEN` environment
 * variable first, then falls back to the token file on disk.
 */
export function getAuthToken(): string | null {
  const envToken = process.env[AUTH_TOKEN_ENV];
  if (envToken) return envToken;

  const tokenPath = path.join(getBaseDir(), AUTH_TOKEN_FILE);
  try {
    return fs.readFileSync(tokenPath, "utf-8").trim();
  } catch {
    return null;
  }
}

/**
 * Save the auth token to disk. Creates the config directory (mode 0o700)
 * and writes the token file with restricted permissions (mode 0o600).
 */
export function saveAuthToken(token: string): void {
  const dir = getBaseDir();
  fs.mkdirSync(dir, { recursive: true, mode: 0o700 });
  const tokenPath = path.join(dir, AUTH_TOKEN_FILE);
  fs.writeFileSync(tokenPath, token, { mode: 0o600 });
}

/**
 * Remove the auth token file from disk.
 */
export function clearAuthToken(): void {
  const tokenPath = path.join(getBaseDir(), AUTH_TOKEN_FILE);
  try {
    fs.unlinkSync(tokenPath);
  } catch {
    // Ignore if file does not exist
  }
}

/* ------------------------------------------------------------------ */
/*  Sync config                                                        */
/* ------------------------------------------------------------------ */

/**
 * Returns the sync directory for a given vault ID.
 */
export function getSyncDir(vaultId: string): string {
  return path.join(getBaseDir(), "sync", vaultId);
}

/**
 * Returns the state database path for a given vault ID.
 */
export function getStatePath(vaultId: string): string {
  return path.join(getSyncDir(vaultId), "state.db");
}

/**
 * Returns the sync log file path for a given vault ID.
 */
export function getLogPath(vaultId: string): string {
  return path.join(getSyncDir(vaultId), "sync.log");
}

/**
 * Returns the sync config file path for a given vault ID.
 */
export function getSyncConfigPath(vaultId: string): string {
  return path.join(getSyncDir(vaultId), "config.json");
}

/**
 * Ensures the sync directory exists for a given vault ID.
 */
export function ensureSyncDir(vaultId: string): void {
  fs.mkdirSync(getSyncDir(vaultId), { recursive: true, mode: 0o700 });
}

/**
 * Reads and parses the sync config for a given vault ID.
 * Returns `null` if the config file does not exist or cannot be parsed.
 */
export function readSyncConfig(vaultId: string): SyncConfig | null {
  const configPath = getSyncConfigPath(vaultId);
  try {
    const raw = fs.readFileSync(configPath, "utf-8");
    return JSON.parse(raw) as SyncConfig;
  } catch {
    return null;
  }
}

/**
 * Writes the sync config for a given vault ID to disk.
 */
export function writeSyncConfig(vaultId: string, config: SyncConfig): void {
  ensureSyncDir(vaultId);
  const configPath = getSyncConfigPath(vaultId);
  fs.writeFileSync(configPath, JSON.stringify(config, null, 2), {
    mode: 0o600,
  });
}

/**
 * Returns the lock file path for a sync vault.
 *
 * @param vaultPath - Absolute path to the vault root
 * @param configDir - Config directory name (defaults to ".obsidian")
 */
export function getLockPath(
  vaultPath: string,
  configDir: string = ".obsidian",
): string {
  return path.join(vaultPath, configDir, ".sync.lock");
}

/**
 * Removes the entire sync configuration directory for a given vault ID.
 */
export function removeSyncConfig(vaultId: string): void {
  const dir = getSyncDir(vaultId);
  try {
    fs.rmSync(dir, { recursive: true, force: true });
  } catch {
    // Ignore if directory does not exist
  }
}

/**
 * Lists all vault IDs that have a local sync configuration.
 */
export function listLocalVaults(): string[] {
  const syncRoot = path.join(getBaseDir(), "sync");
  try {
    const entries = fs.readdirSync(syncRoot, { withFileTypes: true });
    return entries
      .filter((e) => e.isDirectory())
      .map((e) => e.name)
      .filter((id) => {
        const configPath = path.join(syncRoot, id, "config.json");
        return fs.existsSync(configPath);
      });
  } catch {
    return [];
  }
}

/**
 * Finds the sync config whose `vaultPath` matches the given local path.
 * Returns `null` if no matching vault is found.
 */
export function findSyncConfigByPath(localPath: string): SyncConfig | null {
  const resolved = path.resolve(localPath);
  const vaultIds = listLocalVaults();
  for (const id of vaultIds) {
    const config = readSyncConfig(id);
    if (config && path.resolve(config.vaultPath) === resolved) {
      return config;
    }
  }
  return null;
}

/**
 * Returns a default device name in the format "hostname (platform)".
 */
export function getDefaultDeviceName(): string {
  const host = os.hostname();
  const plat =
    process.platform.charAt(0).toUpperCase() + process.platform.slice(1);
  return `${host} (${plat})`;
}

/* ------------------------------------------------------------------ */
/*  Publish config                                                     */
/* ------------------------------------------------------------------ */

/**
 * Returns the publish directory for a given site ID.
 */
export function getPublishDir(siteId: string): string {
  return path.join(getBaseDir(), "publish", siteId);
}

/**
 * Returns the publish config file path for a given site ID.
 */
export function getPublishConfigPath(siteId: string): string {
  return path.join(getPublishDir(siteId), "config.json");
}

/**
 * Returns the publish cache file path for a given site ID.
 */
export function getPublishCachePath(siteId: string): string {
  return path.join(getPublishDir(siteId), "cache.json");
}

/**
 * Ensures the publish directory exists for a given site ID.
 */
export function ensurePublishDir(siteId: string): void {
  fs.mkdirSync(getPublishDir(siteId), { recursive: true, mode: 0o700 });
}

/**
 * Reads and parses the publish config for a given site ID.
 * Returns `null` if the config file does not exist or cannot be parsed.
 */
export function readPublishConfig(siteId: string): PublishConfig | null {
  const configPath = getPublishConfigPath(siteId);
  try {
    const raw = fs.readFileSync(configPath, "utf-8");
    return JSON.parse(raw) as PublishConfig;
  } catch {
    return null;
  }
}

/**
 * Writes the publish config for a given site ID to disk.
 */
export function writePublishConfig(
  siteId: string,
  config: PublishConfig,
): void {
  ensurePublishDir(siteId);
  const configPath = getPublishConfigPath(siteId);
  fs.writeFileSync(configPath, JSON.stringify(config, null, 2), {
    mode: 0o600,
  });
}

/**
 * Removes the entire publish configuration directory for a given site ID.
 */
export function removePublishConfig(siteId: string): void {
  const dir = getPublishDir(siteId);
  try {
    fs.rmSync(dir, { recursive: true, force: true });
  } catch {
    // Ignore if directory does not exist
  }
}

/**
 * Lists all site IDs that have a local publish configuration.
 */
export function listPublishSites(): string[] {
  const publishRoot = path.join(getBaseDir(), "publish");
  try {
    const entries = fs.readdirSync(publishRoot, { withFileTypes: true });
    return entries
      .filter((e) => e.isDirectory())
      .map((e) => e.name)
      .filter((id) => {
        const configPath = path.join(publishRoot, id, "config.json");
        return fs.existsSync(configPath);
      });
  } catch {
    return [];
  }
}

/**
 * Finds the publish config whose `vaultPath` matches the given local path.
 * Returns `null` if no matching site is found.
 */
export function findPublishConfigByPath(
  localPath: string,
): PublishConfig | null {
  const resolved = path.resolve(localPath);
  const siteIds = listPublishSites();
  for (const id of siteIds) {
    const config = readPublishConfig(id);
    if (config && path.resolve(config.vaultPath) === resolved) {
      return config;
    }
  }
  return null;
}

/* ------------------------------------------------------------------ */
/*  Validation helpers                                                 */
/* ------------------------------------------------------------------ */

/**
 * Parses a comma-separated list of file types and validates each entry
 * against the known valid file types.
 *
 * @throws {Error} If any entry is not a valid file type.
 */
export function parseFileTypes(input: string): string[] {
  const types = input
    .split(",")
    .map((t) => t.trim())
    .filter((t) => t.length > 0);

  for (const t of types) {
    if (!(VALID_FILE_TYPES as readonly string[]).includes(t)) {
      throw new Error(
        `Invalid file type "${t}". Valid types: ${VALID_FILE_TYPES.join(", ")}`,
      );
    }
  }
  return types;
}

/**
 * Parses a comma-separated list of config categories and validates each
 * entry against the known valid categories.
 *
 * @throws {Error} If any entry is not a valid config category.
 */
export function parseConfigCategories(input: string): string[] {
  const categories = input
    .split(",")
    .map((c) => c.trim())
    .filter((c) => c.length > 0);

  for (const c of categories) {
    if (!(VALID_CONFIG_CATEGORIES as readonly string[]).includes(c)) {
      throw new Error(
        `Invalid config category "${c}". Valid categories: ${VALID_CONFIG_CATEGORIES.join(", ")}`,
      );
    }
  }
  return categories;
}

/**
 * Validates a config directory name. Must start with "." and not contain
 * any path separators.
 *
 * @returns The validated directory name, or `undefined` if input is undefined.
 * @throws {Error} If the directory name is invalid.
 */
export function validateConfigDir(dir?: string): string | undefined {
  if (dir === undefined) return undefined;

  if (!dir.startsWith(".")) {
    throw new Error('Config directory must start with "."');
  }
  if (dir.includes("/") || dir.includes("\\")) {
    throw new Error("Config directory must not contain path separators");
  }
  return dir;
}

/* ------------------------------------------------------------------ */
/*  Log setup                                                          */
/* ------------------------------------------------------------------ */

/**
 * Sets up logging to a file. Wraps `console.log`, `console.warn`,
 * `console.error`, and `console.debug` to also write timestamped
 * entries to the specified log file.
 *
 * @returns A cleanup function that restores the original console methods
 *          and closes the write stream.
 */
export function setupLogFile(logPath: string): () => void {
  const dir = path.dirname(logPath);
  fs.mkdirSync(dir, { recursive: true });

  const stream = fs.createWriteStream(logPath, { flags: "a" });

  const originalLog = console.log;
  const originalWarn = console.warn;
  const originalError = console.error;
  const originalDebug = console.debug;

  function writeToLog(level: string, args: unknown[]): void {
    const timestamp = new Date().toISOString();
    const message = args
      .map((a) => (typeof a === "string" ? a : JSON.stringify(a)))
      .join(" ");
    stream.write(`[${timestamp}] [${level}] ${message}\n`);
  }

  console.log = (...args: unknown[]) => {
    originalLog.apply(console, args);
    writeToLog("INFO", args);
  };

  console.warn = (...args: unknown[]) => {
    originalWarn.apply(console, args);
    writeToLog("WARN", args);
  };

  console.error = (...args: unknown[]) => {
    originalError.apply(console, args);
    writeToLog("ERROR", args);
  };

  console.debug = (...args: unknown[]) => {
    originalDebug.apply(console, args);
    writeToLog("DEBUG", args);
  };

  return () => {
    console.log = originalLog;
    console.warn = originalWarn;
    console.error = originalError;
    console.debug = originalDebug;
    stream.end();
  };
}
