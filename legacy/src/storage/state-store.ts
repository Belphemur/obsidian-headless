/**
 * @module storage/state-store
 *
 * SQLite-backed state store for sync metadata.  Tracks local file state,
 * server file state, pending sync entries, and key-value metadata using
 * a WAL-mode better-sqlite3 database.
 */

import Database from "better-sqlite3";
import type { Database as DatabaseType, Statement } from "better-sqlite3";
import fs from "node:fs";
import path from "node:path";

/* ------------------------------------------------------------------ */
/*  Types                                                              */
/* ------------------------------------------------------------------ */

/** Represents a file record stored in the sync state database. */
export interface FileRecord {
  path: string;
  previouspath?: string;
  size: number;
  hash: string;
  ctime: number;
  mtime: number;
  folder: boolean;
  deleted?: boolean;
  uid?: number;
  device?: string;
  user?: string;
  initial?: boolean;
  synctime?: number;
  synchash?: string;
}

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
export class StateStore {
  private db: DatabaseType;

  /* Prepared statements */
  private getMeta: Statement;
  private setMeta: Statement;
  private getLocalFile: Statement;
  private setLocalFile: Statement;
  private deleteLocalFile: Statement;
  private getAllLocalFiles: Statement;
  private getServerFile: Statement;
  private setServerFile: Statement;
  private deleteServerFile: Statement;
  private getAllServerFiles: Statement;
  private addPendingFile: Statement;
  private deletePendingFile: Statement;
  private getPendingFiles: Statement;

  /**
   * Open (or create) the state database at `dbPath`.
   * Creates parent directories if needed and initialises the schema.
   *
   * @param dbPath - Absolute path to the SQLite database file.
   */
  constructor(dbPath: string) {
    const dir = path.dirname(dbPath);
    fs.mkdirSync(dir, { recursive: true });

    this.db = new Database(dbPath);
    this.db.pragma("journal_mode = WAL");
    this.db.exec(CREATE_TABLES);

    // Ensure schema version is tracked
    const existing = this.db
      .prepare("SELECT value FROM meta WHERE key = ?")
      .get("schema_version") as { value: string } | undefined;
    if (!existing) {
      this.db
        .prepare("INSERT INTO meta (key, value) VALUES (?, ?)")
        .run("schema_version", SCHEMA_VERSION);
    }

    // Prepare statements
    this.getMeta = this.db.prepare("SELECT value FROM meta WHERE key = ?");
    this.setMeta = this.db.prepare(
      "INSERT OR REPLACE INTO meta (key, value) VALUES (?, ?)",
    );
    this.getLocalFile = this.db.prepare(
      "SELECT data FROM local_files WHERE path = ?",
    );
    this.setLocalFile = this.db.prepare(
      "INSERT OR REPLACE INTO local_files (path, data) VALUES (?, ?)",
    );
    this.deleteLocalFile = this.db.prepare(
      "DELETE FROM local_files WHERE path = ?",
    );
    this.getAllLocalFiles = this.db.prepare("SELECT path, data FROM local_files");
    this.getServerFile = this.db.prepare(
      "SELECT data FROM server_files WHERE path = ?",
    );
    this.setServerFile = this.db.prepare(
      "INSERT OR REPLACE INTO server_files (path, data) VALUES (?, ?)",
    );
    this.deleteServerFile = this.db.prepare(
      "DELETE FROM server_files WHERE path = ?",
    );
    this.getAllServerFiles = this.db.prepare(
      "SELECT path, data FROM server_files",
    );
    this.addPendingFile = this.db.prepare(
      "INSERT OR REPLACE INTO pending_files (uid, path, data) VALUES (?, ?, ?)",
    );
    this.deletePendingFile = this.db.prepare(
      "DELETE FROM pending_files WHERE path = ?",
    );
    this.getPendingFiles = this.db.prepare(
      "SELECT data FROM pending_files ORDER BY uid",
    );
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
  getMetaValue(key: string): string | null {
    const row = this.getMeta.get(key) as { value: string } | undefined;
    return row ? row.value : null;
  }

  /**
   * Set a metadata value.
   *
   * @param key   - The metadata key.
   * @param value - The value to store.
   */
  setMetaValue(key: string, value: string): void {
    this.setMeta.run(key, value);
  }

  /**
   * Get the current sync version number from metadata.
   * Defaults to `0` if not set.
   */
  getVersion(): number {
    const v = this.getMetaValue("version");
    return v ? parseInt(v, 10) : 0;
  }

  /**
   * Set the sync version number.
   *
   * @param v - The version number to store.
   */
  setVersion(v: number): void {
    this.setMetaValue("version", String(v));
  }

  /**
   * Check whether the database is in "initial sync" state.
   * Returns `true` unless explicitly set to `"false"`.
   */
  isInitial(): boolean {
    return this.getMetaValue("initial") !== "false";
  }

  /**
   * Set the initial sync flag.
   *
   * @param v - `true` to mark as initial, `false` otherwise.
   */
  setInitial(v: boolean): void {
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
  getLocalFileRecord(filePath: string): FileRecord | null {
    const row = this.getLocalFile.get(filePath) as { data: string } | undefined;
    return row ? (JSON.parse(row.data) as FileRecord) : null;
  }

  /**
   * Store a local file record.
   *
   * @param record - The file record to store (keyed by `record.path`).
   */
  setLocalFileRecord(record: FileRecord): void {
    this.setLocalFile.run(record.path, JSON.stringify(record));
  }

  /**
   * Delete a local file record by path.
   *
   * @param filePath - The vault-relative file path to remove.
   */
  deleteLocalFileRecord(filePath: string): void {
    this.deleteLocalFile.run(filePath);
  }

  /**
   * Retrieve all local file records as an object keyed by path.
   */
  getAllLocalFileRecords(): Record<string, FileRecord> {
    const rows = this.getAllLocalFiles.all() as { path: string; data: string }[];
    const result: Record<string, FileRecord> = {};
    for (const row of rows) {
      result[row.path] = JSON.parse(row.data) as FileRecord;
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
  getServerFileRecord(filePath: string): FileRecord | null {
    const row = this.getServerFile.get(filePath) as
      | { data: string }
      | undefined;
    return row ? (JSON.parse(row.data) as FileRecord) : null;
  }

  /**
   * Store a server file record.
   *
   * @param record - The file record to store (keyed by `record.path`).
   */
  setServerFileRecord(record: FileRecord): void {
    this.setServerFile.run(record.path, JSON.stringify(record));
  }

  /**
   * Delete a server file record by path.
   *
   * @param filePath - The vault-relative file path to remove.
   */
  deleteServerFileRecord(filePath: string): void {
    this.deleteServerFile.run(filePath);
  }

  /**
   * Retrieve all server file records as an object keyed by path.
   */
  getAllServerFileRecords(): Record<string, FileRecord> {
    const rows = this.getAllServerFiles.all() as {
      path: string;
      data: string;
    }[];
    const result: Record<string, FileRecord> = {};
    for (const row of rows) {
      result[row.path] = JSON.parse(row.data) as FileRecord;
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
  addPendingFileRecord(record: FileRecord): void {
    this.addPendingFile.run(record.uid, record.path, JSON.stringify(record));
  }

  /**
   * Remove a pending file record by path.
   *
   * @param filePath - The vault-relative path to remove from pending.
   */
  deletePendingFileRecord(filePath: string): void {
    this.deletePendingFile.run(filePath);
  }

  /**
   * Get all pending file records ordered by uid.
   */
  getPendingFileRecords(): FileRecord[] {
    const rows = this.getPendingFiles.all() as { data: string }[];
    return rows.map((row) => JSON.parse(row.data) as FileRecord);
  }

  /* ---------------------------------------------------------------- */
  /*  Lifecycle                                                        */
  /* ---------------------------------------------------------------- */

  /**
   * Close the database connection.
   */
  close(): void {
    this.db.close();
  }
}
