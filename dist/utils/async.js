"use strict";
/**
 * @module utils/async
 *
 * Generic concurrency primitives used across the sync and networking layers.
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.AsyncQueue = void 0;
exports.createDeferred = createDeferred;
exports.sleep = sleep;
/**
 * Create a {@link Deferred} – an externally-resolvable promise.
 */
function createDeferred() {
    let resolve;
    let reject;
    const promise = new Promise((res, rej) => {
        resolve = res;
        reject = rej;
    });
    return { promise, resolve, reject };
}
/* ------------------------------------------------------------------ */
/*  Serial async queue                                                */
/* ------------------------------------------------------------------ */
/**
 * A simple serial queue that ensures only one async task runs at a time.
 * Tasks are executed in FIFO order.
 */
class AsyncQueue {
    tail = Promise.resolve();
    /**
     * Enqueue `fn` so it runs after all previously queued tasks.
     * Returns the result of `fn`.
     */
    queue(fn) {
        const next = this.tail.then(fn, fn);
        this.tail = next;
        return next;
    }
}
exports.AsyncQueue = AsyncQueue;
/* ------------------------------------------------------------------ */
/*  Sleep                                                             */
/* ------------------------------------------------------------------ */
/**
 * Return a promise that resolves after `ms` milliseconds.
 */
function sleep(ms) {
    return new Promise((resolve) => setTimeout(resolve, ms));
}
//# sourceMappingURL=async.js.map