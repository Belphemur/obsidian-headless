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

/* ------------------------------------------------------------------ */
/*  Constants                                                          */
/* ------------------------------------------------------------------ */

/** Maximum age (in ms) before a lock is considered stale. */
const STALE_THRESHOLD_MS = 5_000;

/** Interval (in ms) between lock heartbeat refreshes. */
const REFRESH_INTERVAL_MS = 1_000;

/* ------------------------------------------------------------------ */
/*  LockError                                                          */
/* ------------------------------------------------------------------ */

/** Thrown when a lock cannot be acquired because another instance holds it. */
export class LockError extends Error {
  constructor() {
    super("Failed to acquire lock.");
  }
}

/* ------------------------------------------------------------------ */
/*  FileLock                                                            */
/* ------------------------------------------------------------------ */

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
export class FileLock {
  private fs: typeof fs;
  private path: typeof path;
  private lockPath: string;
  private refreshTimer: ReturnType<typeof setInterval> | null = null;
  private lockTime: number = 0;
  private isMs: boolean = true;

  constructor(fsModule: typeof fs, pathModule: typeof path, lockPath: string) {
    this.fs = fsModule;
    this.path = pathModule;
    this.lockPath = lockPath;
  }

  /**
   * Attempts to acquire the lock.
   *
   * If an existing lock directory is found but its mtime is older than
   * 5 seconds, it is considered stale and removed. If the lock is fresh,
   * a {@link LockError} is thrown.
   *
   * @throws {LockError} If the lock is held by another active instance.
   */
  acquire(): void {
    // Ensure parent directory exists
    const parentDir = this.path.dirname(this.lockPath);
    this.fs.mkdirSync(parentDir, { recursive: true });

    try {
      this.fs.mkdirSync(this.lockPath);
    } catch (err: unknown) {
      const code = (err as NodeJS.ErrnoException).code;

      if (code === "EEXIST") {
        // Check if existing lock is stale
        let stat: fs.Stats;
        try {
          stat = this.fs.statSync(this.lockPath);
        } catch (statErr: unknown) {
          if ((statErr as NodeJS.ErrnoException).code === "ENOENT") {
            // Lock was released between our mkdir and stat — retry
            return this.acquire();
          }
          throw statErr;
        }

        const age = Date.now() - Math.round(stat.mtimeMs);
        if (age < STALE_THRESHOLD_MS) {
          throw new LockError();
        }

        // Stale lock — remove and retry
        try {
          this.fs.rmdirSync(this.lockPath);
        } catch {
          // Another process may have already cleaned it up
        }
        return this.acquire();
      }

      throw err;
    }

    // Set initial lock time (ensure not divisible by 1000 for ms detection)
    this.lockTime = Date.now();
    if (this.lockTime % 1000 === 0) {
      this.lockTime += 1;
    }

    this.touch();

    // Check if filesystem supports millisecond timestamps
    const mtime = this.get();
    if (mtime % 1000 === 0) {
      this.isMs = false;
      // Re-set with second granularity
      this.set();
    }

    // Verify we actually own the lock
    if (!this.verify()) {
      throw new LockError();
    }

    // Start heartbeat refresh
    this.refreshTimer = setInterval(() => {
      this.set();
    }, REFRESH_INTERVAL_MS);
  }

  /**
   * Releases the lock.
   *
   * Clears the refresh interval and removes the lock directory, but only
   * if this instance still owns the lock (verified via timestamp).
   */
  release(): void {
    if (this.refreshTimer !== null) {
      clearInterval(this.refreshTimer);
      this.refreshTimer = null;
    }

    if (this.verify()) {
      try {
        this.fs.rmdirSync(this.lockPath);
      } catch {
        // Lock directory may already be gone
      }
    }
  }

  /**
   * Updates the lock timestamp to the current time (heartbeat).
   */
  set(): void {
    let now = Date.now();
    if (!this.isMs) {
      now = Math.ceil(now / 1000) * 1000;
    }
    this.lockTime = now;
    this.touch();
  }

  /**
   * Writes the current lockTime to the lock directory's atime and mtime.
   */
  private touch(): void {
    const timeInSeconds = this.lockTime / 1000;
    try {
      this.fs.utimesSync(this.lockPath, timeInSeconds, timeInSeconds);
    } catch {
      // Ignore errors if lock was removed
    }
  }

  /**
   * Reads the current mtime of the lock directory.
   *
   * @returns The rounded mtime in milliseconds, or 0 on error.
   */
  get(): number {
    try {
      return Math.round(this.fs.statSync(this.lockPath).mtimeMs);
    } catch {
      return 0;
    }
  }

  /**
   * Verifies that this instance still owns the lock by comparing the
   * stored lockTime against the directory's mtime.
   */
  verify(): boolean {
    return this.lockTime === this.get();
  }
}
