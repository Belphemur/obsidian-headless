/**
 * @module sync/lock
 *
 * File lock manager using directory-based locking with timestamp verification.
 * Uses `mkdir` atomicity to provide cooperative locking between processes.
 * The lock is a directory whose mtime acts as a heartbeat — a stale lock
 * (older than 5 seconds) is considered abandoned and can be reclaimed.
 */
import fs from "node:fs";
import path from "node:path";
/** Thrown when a lock cannot be acquired because another instance holds it. */
export declare class LockError extends Error {
    constructor();
}
/**
 * Directory-based cooperative file lock with heartbeat refresh.
 *
 * The lock is represented as a directory on disk. Its modification time
 * is used as a heartbeat to distinguish active locks from stale ones.
 *
 * @example
 * ```ts
 * const lock = new FileLock(fs, path, "/path/to/.lock");
 * lock.acquire();
 * try {
 *   // ... critical section ...
 * } finally {
 *   lock.release();
 * }
 * ```
 */
export declare class FileLock {
    private fs;
    private path;
    private lockPath;
    private refreshTimer;
    private lockTime;
    private isMs;
    constructor(fsModule: typeof fs, pathModule: typeof path, lockPath: string);
    /**
     * Attempts to acquire the lock.
     *
     * If an existing lock directory is found but its mtime is older than
     * 5 seconds, it is considered stale and removed. If the lock is fresh,
     * a {@link LockError} is thrown.
     *
     * @throws {LockError} If the lock is held by another active instance.
     */
    acquire(): void;
    /**
     * Releases the lock.
     *
     * Clears the refresh interval and removes the lock directory, but only
     * if this instance still owns the lock (verified via timestamp).
     */
    release(): void;
    /**
     * Updates the lock timestamp to the current time (heartbeat).
     */
    set(): void;
    /**
     * Writes the current lockTime to the lock directory's atime and mtime.
     */
    private touch;
    /**
     * Reads the current mtime of the lock directory.
     *
     * @returns The rounded mtime in milliseconds, or 0 on error.
     */
    get(): number;
    /**
     * Verifies that this instance still owns the lock by comparing the
     * stored lockTime against the directory's mtime.
     */
    verify(): boolean;
}
//# sourceMappingURL=lock.d.ts.map