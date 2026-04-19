"use strict";
/**
 * @module config
 *
 * Configuration management for the Obsidian Headless CLI.
 * Handles auth tokens, sync vault configs, publish site configs,
 * validation helpers, and log file setup.
 */
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
exports.DEFAULT_CONFIG_CATEGORIES = exports.VALID_CONFIG_CATEGORIES = exports.DEFAULT_FILE_TYPES = exports.VALID_FILE_TYPES = exports.APP_NAME = void 0;
exports.getBaseDir = getBaseDir;
exports.getAuthToken = getAuthToken;
exports.saveAuthToken = saveAuthToken;
exports.clearAuthToken = clearAuthToken;
exports.getSyncDir = getSyncDir;
exports.getStatePath = getStatePath;
exports.getLogPath = getLogPath;
exports.getSyncConfigPath = getSyncConfigPath;
exports.ensureSyncDir = ensureSyncDir;
exports.readSyncConfig = readSyncConfig;
exports.writeSyncConfig = writeSyncConfig;
exports.getLockPath = getLockPath;
exports.removeSyncConfig = removeSyncConfig;
exports.listLocalVaults = listLocalVaults;
exports.findSyncConfigByPath = findSyncConfigByPath;
exports.getDefaultDeviceName = getDefaultDeviceName;
exports.getPublishDir = getPublishDir;
exports.getPublishConfigPath = getPublishConfigPath;
exports.getPublishCachePath = getPublishCachePath;
exports.ensurePublishDir = ensurePublishDir;
exports.readPublishConfig = readPublishConfig;
exports.writePublishConfig = writePublishConfig;
exports.removePublishConfig = removePublishConfig;
exports.listPublishSites = listPublishSites;
exports.findPublishConfigByPath = findPublishConfigByPath;
exports.parseFileTypes = parseFileTypes;
exports.parseConfigCategories = parseConfigCategories;
exports.validateConfigDir = validateConfigDir;
exports.setupLogFile = setupLogFile;
const node_fs_1 = __importDefault(require("node:fs"));
const node_path_1 = __importDefault(require("node:path"));
const node_os_1 = __importDefault(require("node:os"));
/* ------------------------------------------------------------------ */
/*  Constants                                                          */
/* ------------------------------------------------------------------ */
/** Application name used for config directory naming. */
exports.APP_NAME = "obsidian-headless";
/** Valid file types for sync allow-types. */
exports.VALID_FILE_TYPES = [
    "image",
    "audio",
    "video",
    "pdf",
    "unsupported",
];
/** Default file types included in sync. */
exports.DEFAULT_FILE_TYPES = ["image", "audio", "pdf", "video"];
/** Valid config categories for sync special files. */
exports.VALID_CONFIG_CATEGORIES = [
    "app",
    "appearance",
    "appearance-data",
    "hotkey",
    "core-plugin",
    "core-plugin-data",
    "community-plugin",
    "community-plugin-data",
];
/** Default config categories included in sync. */
exports.DEFAULT_CONFIG_CATEGORIES = [
    "app",
    "appearance",
    "appearance-data",
    "hotkey",
    "core-plugin",
    "core-plugin-data",
];
/* ------------------------------------------------------------------ */
/*  Base directory                                                     */
/* ------------------------------------------------------------------ */
/**
 * Returns the base configuration directory for this application.
 *
 * - Linux: `$XDG_CONFIG_HOME/obsidian-headless` or `~/.config/obsidian-headless`
 * - macOS/Windows: `~/.obsidian-headless`
 */
function getBaseDir() {
    if (process.platform === "linux") {
        const xdg = process.env["XDG_CONFIG_HOME"];
        const base = xdg || node_path_1.default.join(node_os_1.default.homedir(), ".config");
        return node_path_1.default.join(base, exports.APP_NAME);
    }
    return node_path_1.default.join(node_os_1.default.homedir(), `.${exports.APP_NAME}`);
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
function getAuthToken() {
    const envToken = process.env[AUTH_TOKEN_ENV];
    if (envToken)
        return envToken;
    const tokenPath = node_path_1.default.join(getBaseDir(), AUTH_TOKEN_FILE);
    try {
        return node_fs_1.default.readFileSync(tokenPath, "utf-8").trim();
    }
    catch {
        return null;
    }
}
/**
 * Save the auth token to disk. Creates the config directory (mode 0o700)
 * and writes the token file with restricted permissions (mode 0o600).
 */
function saveAuthToken(token) {
    const dir = getBaseDir();
    node_fs_1.default.mkdirSync(dir, { recursive: true, mode: 0o700 });
    const tokenPath = node_path_1.default.join(dir, AUTH_TOKEN_FILE);
    node_fs_1.default.writeFileSync(tokenPath, token, { mode: 0o600 });
}
/**
 * Remove the auth token file from disk.
 */
function clearAuthToken() {
    const tokenPath = node_path_1.default.join(getBaseDir(), AUTH_TOKEN_FILE);
    try {
        node_fs_1.default.unlinkSync(tokenPath);
    }
    catch {
        // Ignore if file does not exist
    }
}
/* ------------------------------------------------------------------ */
/*  Sync config                                                        */
/* ------------------------------------------------------------------ */
/**
 * Returns the sync directory for a given vault ID.
 */
function getSyncDir(vaultId) {
    return node_path_1.default.join(getBaseDir(), "sync", vaultId);
}
/**
 * Returns the state database path for a given vault ID.
 */
function getStatePath(vaultId) {
    return node_path_1.default.join(getSyncDir(vaultId), "state.db");
}
/**
 * Returns the sync log file path for a given vault ID.
 */
function getLogPath(vaultId) {
    return node_path_1.default.join(getSyncDir(vaultId), "sync.log");
}
/**
 * Returns the sync config file path for a given vault ID.
 */
function getSyncConfigPath(vaultId) {
    return node_path_1.default.join(getSyncDir(vaultId), "config.json");
}
/**
 * Ensures the sync directory exists for a given vault ID.
 */
function ensureSyncDir(vaultId) {
    node_fs_1.default.mkdirSync(getSyncDir(vaultId), { recursive: true, mode: 0o700 });
}
/**
 * Reads and parses the sync config for a given vault ID.
 * Returns `null` if the config file does not exist or cannot be parsed.
 */
function readSyncConfig(vaultId) {
    const configPath = getSyncConfigPath(vaultId);
    try {
        const raw = node_fs_1.default.readFileSync(configPath, "utf-8");
        return JSON.parse(raw);
    }
    catch {
        return null;
    }
}
/**
 * Writes the sync config for a given vault ID to disk.
 */
function writeSyncConfig(vaultId, config) {
    ensureSyncDir(vaultId);
    const configPath = getSyncConfigPath(vaultId);
    node_fs_1.default.writeFileSync(configPath, JSON.stringify(config, null, 2), {
        mode: 0o600,
    });
}
/**
 * Returns the lock file path for a sync vault.
 *
 * @param vaultPath - Absolute path to the vault root
 * @param configDir - Config directory name (defaults to ".obsidian")
 */
function getLockPath(vaultPath, configDir = ".obsidian") {
    return node_path_1.default.join(vaultPath, configDir, ".sync.lock");
}
/**
 * Removes the entire sync configuration directory for a given vault ID.
 */
function removeSyncConfig(vaultId) {
    const dir = getSyncDir(vaultId);
    try {
        node_fs_1.default.rmSync(dir, { recursive: true, force: true });
    }
    catch {
        // Ignore if directory does not exist
    }
}
/**
 * Lists all vault IDs that have a local sync configuration.
 */
function listLocalVaults() {
    const syncRoot = node_path_1.default.join(getBaseDir(), "sync");
    try {
        const entries = node_fs_1.default.readdirSync(syncRoot, { withFileTypes: true });
        return entries
            .filter((e) => e.isDirectory())
            .map((e) => e.name)
            .filter((id) => {
            const configPath = node_path_1.default.join(syncRoot, id, "config.json");
            return node_fs_1.default.existsSync(configPath);
        });
    }
    catch {
        return [];
    }
}
/**
 * Finds the sync config whose `vaultPath` matches the given local path.
 * Returns `null` if no matching vault is found.
 */
function findSyncConfigByPath(localPath) {
    const resolved = node_path_1.default.resolve(localPath);
    const vaultIds = listLocalVaults();
    for (const id of vaultIds) {
        const config = readSyncConfig(id);
        if (config && node_path_1.default.resolve(config.vaultPath) === resolved) {
            return config;
        }
    }
    return null;
}
/**
 * Returns a default device name in the format "hostname (platform)".
 */
function getDefaultDeviceName() {
    const host = node_os_1.default.hostname();
    const plat = process.platform.charAt(0).toUpperCase() + process.platform.slice(1);
    return `${host} (${plat})`;
}
/* ------------------------------------------------------------------ */
/*  Publish config                                                     */
/* ------------------------------------------------------------------ */
/**
 * Returns the publish directory for a given site ID.
 */
function getPublishDir(siteId) {
    return node_path_1.default.join(getBaseDir(), "publish", siteId);
}
/**
 * Returns the publish config file path for a given site ID.
 */
function getPublishConfigPath(siteId) {
    return node_path_1.default.join(getPublishDir(siteId), "config.json");
}
/**
 * Returns the publish cache file path for a given site ID.
 */
function getPublishCachePath(siteId) {
    return node_path_1.default.join(getPublishDir(siteId), "cache.json");
}
/**
 * Ensures the publish directory exists for a given site ID.
 */
function ensurePublishDir(siteId) {
    node_fs_1.default.mkdirSync(getPublishDir(siteId), { recursive: true, mode: 0o700 });
}
/**
 * Reads and parses the publish config for a given site ID.
 * Returns `null` if the config file does not exist or cannot be parsed.
 */
function readPublishConfig(siteId) {
    const configPath = getPublishConfigPath(siteId);
    try {
        const raw = node_fs_1.default.readFileSync(configPath, "utf-8");
        return JSON.parse(raw);
    }
    catch {
        return null;
    }
}
/**
 * Writes the publish config for a given site ID to disk.
 */
function writePublishConfig(siteId, config) {
    ensurePublishDir(siteId);
    const configPath = getPublishConfigPath(siteId);
    node_fs_1.default.writeFileSync(configPath, JSON.stringify(config, null, 2), {
        mode: 0o600,
    });
}
/**
 * Removes the entire publish configuration directory for a given site ID.
 */
function removePublishConfig(siteId) {
    const dir = getPublishDir(siteId);
    try {
        node_fs_1.default.rmSync(dir, { recursive: true, force: true });
    }
    catch {
        // Ignore if directory does not exist
    }
}
/**
 * Lists all site IDs that have a local publish configuration.
 */
function listPublishSites() {
    const publishRoot = node_path_1.default.join(getBaseDir(), "publish");
    try {
        const entries = node_fs_1.default.readdirSync(publishRoot, { withFileTypes: true });
        return entries
            .filter((e) => e.isDirectory())
            .map((e) => e.name)
            .filter((id) => {
            const configPath = node_path_1.default.join(publishRoot, id, "config.json");
            return node_fs_1.default.existsSync(configPath);
        });
    }
    catch {
        return [];
    }
}
/**
 * Finds the publish config whose `vaultPath` matches the given local path.
 * Returns `null` if no matching site is found.
 */
function findPublishConfigByPath(localPath) {
    const resolved = node_path_1.default.resolve(localPath);
    const siteIds = listPublishSites();
    for (const id of siteIds) {
        const config = readPublishConfig(id);
        if (config && node_path_1.default.resolve(config.vaultPath) === resolved) {
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
function parseFileTypes(input) {
    const types = input
        .split(",")
        .map((t) => t.trim())
        .filter((t) => t.length > 0);
    for (const t of types) {
        if (!exports.VALID_FILE_TYPES.includes(t)) {
            throw new Error(`Invalid file type "${t}". Valid types: ${exports.VALID_FILE_TYPES.join(", ")}`);
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
function parseConfigCategories(input) {
    const categories = input
        .split(",")
        .map((c) => c.trim())
        .filter((c) => c.length > 0);
    for (const c of categories) {
        if (!exports.VALID_CONFIG_CATEGORIES.includes(c)) {
            throw new Error(`Invalid config category "${c}". Valid categories: ${exports.VALID_CONFIG_CATEGORIES.join(", ")}`);
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
function validateConfigDir(dir) {
    if (dir === undefined)
        return undefined;
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
function setupLogFile(logPath) {
    const dir = node_path_1.default.dirname(logPath);
    node_fs_1.default.mkdirSync(dir, { recursive: true });
    const stream = node_fs_1.default.createWriteStream(logPath, { flags: "a" });
    const originalLog = console.log;
    const originalWarn = console.warn;
    const originalError = console.error;
    const originalDebug = console.debug;
    function writeToLog(level, args) {
        const timestamp = new Date().toISOString();
        const message = args
            .map((a) => (typeof a === "string" ? a : JSON.stringify(a)))
            .join(" ");
        stream.write(`[${timestamp}] [${level}] ${message}\n`);
    }
    console.log = (...args) => {
        originalLog.apply(console, args);
        writeToLog("INFO", args);
    };
    console.warn = (...args) => {
        originalWarn.apply(console, args);
        writeToLog("WARN", args);
    };
    console.error = (...args) => {
        originalError.apply(console, args);
        writeToLog("ERROR", args);
    };
    console.debug = (...args) => {
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
//# sourceMappingURL=index.js.map