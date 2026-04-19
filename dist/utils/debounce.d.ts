/**
 * @module utils/debounce
 *
 * A debounce implementation that supports both trailing and "keep-alive"
 * semantics.  Used by the file system adapter to batch rapid change events.
 */
export interface DebouncedFn {
    (...args: unknown[]): void;
    /** Cancel any pending invocation. */
    cancel(): void;
    /** Immediately invoke if a call is pending. */
    run(): void;
}
/**
 * Create a debounced wrapper around `fn`.
 *
 * @param fn       The function to debounce.
 * @param delayMs  Delay in milliseconds.
 * @param keepAlive When `true`, subsequent calls during the wait period reset
 *                  the timer so the function fires `delayMs` after the *last*
 *                  call.  When `false` (default) it fires `delayMs` after the
 *                  *first* call.
 */
export declare function debounce(fn: (...args: unknown[]) => void, delayMs?: number, keepAlive?: boolean): DebouncedFn;
//# sourceMappingURL=debounce.d.ts.map