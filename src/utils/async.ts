/**
 * @module utils/async
 *
 * Generic concurrency primitives used across the sync and networking layers.
 */

/* ------------------------------------------------------------------ */
/*  Deferred promise                                                  */
/* ------------------------------------------------------------------ */

/** A promise whose `resolve` / `reject` callbacks are externally accessible. */
export interface Deferred<T> {
  promise: Promise<T>;
  resolve: (value: T | PromiseLike<T>) => void;
  reject: (reason?: unknown) => void;
}

/**
 * Create a {@link Deferred} – an externally-resolvable promise.
 *
 * Uses the ES2024 `Promise.withResolvers()` API available in Node 24+.
 */
export function createDeferred<T>(): Deferred<T> {
  return Promise.withResolvers<T>();
}

/* ------------------------------------------------------------------ */
/*  Serial async queue                                                */
/* ------------------------------------------------------------------ */

/**
 * A simple serial queue that ensures only one async task runs at a time.
 * Tasks are executed in FIFO order.
 */
export class AsyncQueue {
  private tail: Promise<unknown> = Promise.resolve();

  /**
   * Enqueue `fn` so it runs after all previously queued tasks.
   * Returns the result of `fn`.
   */
  queue<T>(fn: () => Promise<T>): Promise<T> {
    const next = this.tail.then(fn, fn);
    this.tail = next;
    return next as Promise<T>;
  }
}

/* ------------------------------------------------------------------ */
/*  Sleep                                                             */
/* ------------------------------------------------------------------ */

/**
 * Return a promise that resolves after `ms` milliseconds.
 */
export function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}
