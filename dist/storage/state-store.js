"use strict";
/**
 * @module storage/state-store
 *
 * SQLite-backed state store for sync metadata.  Tracks local file state,
 * server file state, pending sync entries, and key-value metadata using
 * a WAL-mode better-sqlite3 database.
 */
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
exports.StateStore = void 0;
const better_sqlite3_1 = __importDefault(require("better-sqlite3"));
const node_fs_1 = __importDefault(require("node:fs"));
const node_path_1 = __importDefault(require("node:path"));
/* ------------------------------------------------------------------ */
/*  Schema                                                             */
/* ------------------------------------------------------------------ */
const SCHEMA_VERSION = "1";
const CREATE_TABLES = `
  CREATE TABLE IF NOT EXISTS meta (
    key TEXT PRIMARY KEY,
    value TEXT
  );
  CREATE TABLE IF NOT EXISTS local_files (
    path TEXT PRIMARY KEY,
    data TEXT NOT NULL
  );
  CREATE TABLE IF NOT EXISTS server_files (
    path TEXT PRIMARY KEY,
    data TEXT NOT NULL
  );
  CREATE TABLE IF NOT EXISTS pending_files (
    uid INTEGER PRIMARY KEY,
    path TEXT,
    data TEXT NOT NULL
  );
`;
/* ------------------------------------------------------------------ */
/*  StateStore                                                         */
/* ------------------------------------------------------------------ */
/**
 * SQLite state store for sync metadata.
 *
 * Stores local/server file records, pending sync entries, and arbitrary
 * key-value metadata in a WAL-mode SQLite database.
 */
class StateStore {
    db;
    /* Prepared statements */
    getMeta;
    setMeta;
    getLocalFile;
    setLocalFile;
    deleteLocalFile;
    getAllLocalFiles;
    getServerFile;
    setServerFile;
    deleteServerFile;
    getAllServerFiles;
    addPendingFile;
    deletePendingFile;
    getPendingFiles;
    /**
     * Open (or create) the state database at `dbPath`.
     * Creates parent directories if needed and initialises the schema.
     *
     * @param dbPath - Absolute path to the SQLite database file.
     */
    constructor(dbPath) {
        const dir = node_path_1.default.dirname(dbPath);
        node_fs_1.default.mkdirSync(dir, { recursive: true });
        this.db = new better_sqlite3_1.default(dbPath);
        this.db.pragma("journal_mode = WAL");
        this.db.exec(CREATE_TABLES);
        // Ensure schema version is tracked
        const existing = this.db
            .prepare("SELECT value FROM meta WHERE key = ?")
            .get("schema_version");
        if (!existing) {
            this.db
                .prepare("INSERT INTO meta (key, value) VALUES (?, ?)")
                .run("schema_version", SCHEMA_VERSION);
        }
        // Prepare statements
        this.getMeta = this.db.prepare("SELECT value FROM meta WHERE key = ?");
        this.setMeta = this.db.prepare("INSERT OR REPLACE INTO meta (key, value) VALUES (?, ?)");
        this.getLocalFile = this.db.prepare("SELECT data FROM local_files WHERE path = ?");
        this.setLocalFile = this.db.prepare("INSERT OR REPLACE INTO local_files (path, data) VALUES (?, ?)");
        this.deleteLocalFile = this.db.prepare("DELETE FROM local_files WHERE path = ?");
        this.getAllLocalFiles = this.db.prepare("SELECT path, data FROM local_files");
        this.getServerFile = this.db.prepare("SELECT data FROM server_files WHERE path = ?");
        this.setServerFile = this.db.prepare("INSERT OR REPLACE INTO server_files (path, data) VALUES (?, ?)");
        this.deleteServerFile = this.db.prepare("DELETE FROM server_files WHERE path = ?");
        this.getAllServerFiles = this.db.prepare("SELECT path, data FROM server_files");
        this.addPendingFile = this.db.prepare("INSERT OR REPLACE INTO pending_files (uid, path, data) VALUES (?, ?, ?)");
        this.deletePendingFile = this.db.prepare("DELETE FROM pending_files WHERE path = ?");
        this.getPendingFiles = this.db.prepare("SELECT data FROM pending_files ORDER BY uid");
    }
    /* ---------------------------------------------------------------- */
    /*  Meta helpers                                                     */
    /* ---------------------------------------------------------------- */
    /**
     * Retrieve a metadata value by key.
     *
     * @param key - The metadata key.
     * @returns The value string, or `null` if not found.
     */
    getMetaValue(key) {
        const row = this.getMeta.get(key);
        return row ? row.value : null;
    }
    /**
     * Set a metadata value.
     *
     * @param key   - The metadata key.
     * @param value - The value to store.
     */
    setMetaValue(key, value) {
        this.setMeta.run(key, value);
    }
    /**
     * Get the current sync version number from metadata.
     * Defaults to `0` if not set.
     */
    getVersion() {
        const v = this.getMetaValue("version");
        return v ? parseInt(v, 10) : 0;
    }
    /**
     * Set the sync version number.
     *
     * @param v - The version number to store.
     */
    setVersion(v) {
        this.setMetaValue("version", String(v));
    }
    /**
     * Check whether the database is in "initial sync" state.
     * Returns `true` unless explicitly set to `"false"`.
     */
    isInitial() {
        return this.getMetaValue("initial") !== "false";
    }
    /**
     * Set the initial sync flag.
     *
     * @param v - `true` to mark as initial, `false` otherwise.
     */
    setInitial(v) {
        this.setMetaValue("initial", v ? "true" : "false");
    }
    /* ---------------------------------------------------------------- */
    /*  Local files                                                      */
    /* ---------------------------------------------------------------- */
    /**
     * Retrieve a local file record by path.
     *
     * @param filePath - The vault-relative file path.
     * @returns The file record, or `null` if not found.
     */
    getLocalFileRecord(filePath) {
        const row = this.getLocalFile.get(filePath);
        return row ? JSON.parse(row.data) : null;
    }
    /**
     * Store a local file record.
     *
     * @param record - The file record to store (keyed by `record.path`).
     */
    setLocalFileRecord(record) {
        this.setLocalFile.run(record.path, JSON.stringify(record));
    }
    /**
     * Delete a local file record by path.
     *
     * @param filePath - The vault-relative file path to remove.
     */
    deleteLocalFileRecord(filePath) {
        this.deleteLocalFile.run(filePath);
    }
    /**
     * Retrieve all local file records as an object keyed by path.
     */
    getAllLocalFileRecords() {
        const rows = this.getAllLocalFiles.all();
        const result = {};
        for (const row of rows) {
            result[row.path] = JSON.parse(row.data);
        }
        return result;
    }
    /* ---------------------------------------------------------------- */
    /*  Server files                                                     */
    /* ---------------------------------------------------------------- */
    /**
     * Retrieve a server file record by path.
     *
     * @param filePath - The vault-relative file path.
     * @returns The file record, or `null` if not found.
     */
    getServerFileRecord(filePath) {
        const row = this.getServerFile.get(filePath);
        return row ? JSON.parse(row.data) : null;
    }
    /**
     * Store a server file record.
     *
     * @param record - The file record to store (keyed by `record.path`).
     */
    setServerFileRecord(record) {
        this.setServerFile.run(record.path, JSON.stringify(record));
    }
    /**
     * Delete a server file record by path.
     *
     * @param filePath - The vault-relative file path to remove.
     */
    deleteServerFileRecord(filePath) {
        this.deleteServerFile.run(filePath);
    }
    /**
     * Retrieve all server file records as an object keyed by path.
     */
    getAllServerFileRecords() {
        const rows = this.getAllServerFiles.all();
        const result = {};
        for (const row of rows) {
            result[row.path] = JSON.parse(row.data);
        }
        return result;
    }
    /* ---------------------------------------------------------------- */
    /*  Pending files                                                    */
    /* ---------------------------------------------------------------- */
    /**
     * Add a file record to the pending sync queue.
     *
     * @param record - The file record; uses `record.uid` as the primary key.
     */
    addPendingFileRecord(record) {
        this.addPendingFile.run(record.uid, record.path, JSON.stringify(record));
    }
    /**
     * Remove a pending file record by path.
     *
     * @param filePath - The vault-relative path to remove from pending.
     */
    deletePendingFileRecord(filePath) {
        this.deletePendingFile.run(filePath);
    }
    /**
     * Get all pending file records ordered by uid.
     */
    getPendingFileRecords() {
        const rows = this.getPendingFiles.all();
        return rows.map((row) => JSON.parse(row.data));
    }
    /* ---------------------------------------------------------------- */
    /*  Lifecycle                                                        */
    /* ---------------------------------------------------------------- */
    /**
     * Close the database connection.
     */
    close() {
        this.db.close();
    }
}
exports.StateStore = StateStore;
//# sourceMappingURL=state-store.js.map