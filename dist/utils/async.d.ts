/**
 * @module utils/async
 *
 * Generic concurrency primitives used across the sync and networking layers.
 */
/** A promise whose `resolve` / `reject` callbacks are externally accessible. */
export interface Deferred<T> {
    promise: Promise<T>;
    resolve: (value: T | PromiseLike<T>) => void;
    reject: (reason?: unknown) => void;
}
/**
 * Create a {@link Deferred} – an externally-resolvable promise.
 */
export declare function createDeferred<T>(): Deferred<T>;
/**
 * A simple serial queue that ensures only one async task runs at a time.
 * Tasks are executed in FIFO order.
 */
export declare class AsyncQueue {
    private tail;
    /**
     * Enqueue `fn` so it runs after all previously queued tasks.
     * Returns the result of `fn`.
     */
    queue<T>(fn: () => Promise<T>): Promise<T>;
}
/**
 * Return a promise that resolves after `ms` milliseconds.
 */
export declare function sleep(ms: number): Promise<void>;
//# sourceMappingURL=async.d.ts.map