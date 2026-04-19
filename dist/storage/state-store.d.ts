/**
 * @module storage/state-store
 *
 * SQLite-backed state store for sync metadata.  Tracks local file state,
 * server file state, pending sync entries, and key-value metadata using
 * a WAL-mode better-sqlite3 database.
 */
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
/**
 * SQLite state store for sync metadata.
 *
 * Stores local/server file records, pending sync entries, and arbitrary
 * key-value metadata in a WAL-mode SQLite database.
 */
export declare class StateStore {
    private db;
    private getMeta;
    private setMeta;
    private getLocalFile;
    private setLocalFile;
    private deleteLocalFile;
    private getAllLocalFiles;
    private getServerFile;
    private setServerFile;
    private deleteServerFile;
    private getAllServerFiles;
    private addPendingFile;
    private deletePendingFile;
    private getPendingFiles;
    /**
     * Open (or create) the state database at `dbPath`.
     * Creates parent directories if needed and initialises the schema.
     *
     * @param dbPath - Absolute path to the SQLite database file.
     */
    constructor(dbPath: string);
    /**
     * Retrieve a metadata value by key.
     *
     * @param key - The metadata key.
     * @returns The value string, or `null` if not found.
     */
    getMetaValue(key: string): string | null;
    /**
     * Set a metadata value.
     *
     * @param key   - The metadata key.
     * @param value - The value to store.
     */
    setMetaValue(key: string, value: string): void;
    /**
     * Get the current sync version number from metadata.
     * Defaults to `0` if not set.
     */
    getVersion(): number;
    /**
     * Set the sync version number.
     *
     * @param v - The version number to store.
     */
    setVersion(v: number): void;
    /**
     * Check whether the database is in "initial sync" state.
     * Returns `true` unless explicitly set to `"false"`.
     */
    isInitial(): boolean;
    /**
     * Set the initial sync flag.
     *
     * @param v - `true` to mark as initial, `false` otherwise.
     */
    setInitial(v: boolean): void;
    /**
     * Retrieve a local file record by path.
     *
     * @param filePath - The vault-relative file path.
     * @returns The file record, or `null` if not found.
     */
    getLocalFileRecord(filePath: string): FileRecord | null;
    /**
     * Store a local file record.
     *
     * @param record - The file record to store (keyed by `record.path`).
     */
    setLocalFileRecord(record: FileRecord): void;
    /**
     * Delete a local file record by path.
     *
     * @param filePath - The vault-relative file path to remove.
     */
    deleteLocalFileRecord(filePath: string): void;
    /**
     * Retrieve all local file records as an object keyed by path.
     */
    getAllLocalFileRecords(): Record<string, FileRecord>;
    /**
     * Retrieve a server file record by path.
     *
     * @param filePath - The vault-relative file path.
     * @returns The file record, or `null` if not found.
     */
    getServerFileRecord(filePath: string): FileRecord | null;
    /**
     * Store a server file record.
     *
     * @param record - The file record to store (keyed by `record.path`).
     */
    setServerFileRecord(record: FileRecord): void;
    /**
     * Delete a server file record by path.
     *
     * @param filePath - The vault-relative file path to remove.
     */
    deleteServerFileRecord(filePath: string): void;
    /**
     * Retrieve all server file records as an object keyed by path.
     */
    getAllServerFileRecords(): Record<string, FileRecord>;
    /**
     * Add a file record to the pending sync queue.
     *
     * @param record - The file record; uses `record.uid` as the primary key.
     */
    addPendingFileRecord(record: FileRecord): void;
    /**
     * Remove a pending file record by path.
     *
     * @param filePath - The vault-relative path to remove from pending.
     */
    deletePendingFileRecord(filePath: string): void;
    /**
     * Get all pending file records ordered by uid.
     */
    getPendingFileRecords(): FileRecord[];
    /**
     * Close the database connection.
     */
    close(): void;
}
//# sourceMappingURL=state-store.d.ts.map