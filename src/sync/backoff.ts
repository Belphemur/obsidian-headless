/**
 * @module sync/backoff
 *
 * Exponential backoff with jitter for reconnection attempts.
 * Used by the sync engine to space out retries after connection failures,
 * preventing thundering-herd problems and server overload.
 */

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
export class ExponentialBackoff {
  /** Number of consecutive failures since last success. */
  private count = 0;

  /** Timestamp (ms since epoch) when the next attempt is allowed. */
  private nextTs = Date.now();

  /**
   * Create a new exponential backoff controller.
   *
   * @param min   - Minimum delay in milliseconds (floor). Defaults to `0`.
   * @param max   - Maximum delay in milliseconds (ceiling). Defaults to `Number.MAX_VALUE`.
   * @param base  - Base delay in milliseconds before exponential growth. Defaults to `1000`.
   * @param jitter - Whether to apply random jitter (±50%) to the timeout. Defaults to `true`.
   */
  constructor(
    private readonly min = 0,
    private readonly max = Number.MAX_VALUE,
    private readonly base = 1000,
    private readonly jitter = true,
  ) {}

  /**
   * Signal a successful connection.
   * Resets the failure counter and schedules the next eligible timestamp
   * using the minimum delay.
   */
  success(): void {
    this.count = 0;
    this.nextTs = Date.now() + this.getTimeout();
  }

  /**
   * Signal a failed connection attempt.
   * Increments the failure counter and schedules the next eligible
   * timestamp using the current exponential timeout.
   */
  fail(): void {
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
  getTimeout(): number {
    if (this.count === 0) return this.min;
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
  getNextTs(): number {
    return this.nextTs;
  }

  /**
   * Check whether the backoff period has elapsed and a retry is allowed.
   *
   * @returns `true` if the current time exceeds the next eligible timestamp.
   */
  isReady(): boolean {
    return Date.now() > this.nextTs;
  }
}
