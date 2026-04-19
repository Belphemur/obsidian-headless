"use strict";
/**
 * @module sync/backoff
 *
 * Exponential backoff with jitter for reconnection attempts.
 * Used by the sync engine to space out retries after connection failures,
 * preventing thundering-herd problems and server overload.
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.ExponentialBackoff = void 0;
/**
 * Implements exponential backoff with optional jitter for controlling
 * reconnection timing after failures.
 *
 * The timeout grows exponentially with each consecutive failure:
 * `base * 2^(count-1)`, clamped between `min` and `max`, with optional
 * random jitter to decorrelate multiple clients reconnecting simultaneously.
 *
 * @example
 * ```ts
 * const backoff = new ExponentialBackoff(0, 300_000, 5000, true);
 * backoff.fail();           // schedule next retry
 * if (backoff.isReady()) {  // check if enough time has elapsed
 *   // attempt reconnection
 * }
 * backoff.success();        // reset on successful connection
 * ```
 */
class ExponentialBackoff {
    min;
    max;
    base;
    jitter;
    /** Number of consecutive failures since last success. */
    count = 0;
    /** Timestamp (ms since epoch) when the next attempt is allowed. */
    nextTs = Date.now();
    /**
     * Create a new exponential backoff controller.
     *
     * @param min   - Minimum delay in milliseconds (floor). Defaults to `0`.
     * @param max   - Maximum delay in milliseconds (ceiling). Defaults to `Number.MAX_VALUE`.
     * @param base  - Base delay in milliseconds before exponential growth. Defaults to `1000`.
     * @param jitter - Whether to apply random jitter (±50%) to the timeout. Defaults to `true`.
     */
    constructor(min = 0, max = Number.MAX_VALUE, base = 1000, jitter = true) {
        this.min = min;
        this.max = max;
        this.base = base;
        this.jitter = jitter;
    }
    /**
     * Signal a successful connection.
     * Resets the failure counter and schedules the next eligible timestamp
     * using the minimum delay.
     */
    success() {
        this.count = 0;
        this.nextTs = Date.now() + this.getTimeout();
    }
    /**
     * Signal a failed connection attempt.
     * Increments the failure counter and schedules the next eligible
     * timestamp using the current exponential timeout.
     */
    fail() {
        this.count++;
        this.nextTs = Date.now() + this.getTimeout();
    }
    /**
     * Calculate the current timeout value based on the failure count.
     *
     * When `count` is 0 (no failures), returns `min`.
     * Otherwise computes `base * 2^(count-1)` with optional jitter,
     * clamped to `[min, max]`.
     *
     * @returns The timeout duration in milliseconds (floored to integer).
     */
    getTimeout() {
        if (this.count === 0)
            return this.min;
        const exp = this.count - 1;
        let timeout = this.base * Math.pow(2, exp);
        if (this.jitter) {
            const jitterFactor = 0.5;
            timeout = timeout * (1 - jitterFactor + jitterFactor * Math.random());
        }
        return Math.floor(Math.min(this.max, this.min + timeout));
    }
    /**
     * Get the timestamp (ms since epoch) when the next attempt is allowed.
     *
     * @returns Unix timestamp in milliseconds.
     */
    getNextTs() {
        return this.nextTs;
    }
    /**
     * Check whether the backoff period has elapsed and a retry is allowed.
     *
     * @returns `true` if the current time exceeds the next eligible timestamp.
     */
    isReady() {
        return Date.now() > this.nextTs;
    }
}
exports.ExponentialBackoff = ExponentialBackoff;
//# sourceMappingURL=backoff.js.map