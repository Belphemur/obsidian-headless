/**
 * @module utils/debounce
 *
 * A debounce implementation that supports both trailing and "keep-alive"
 * semantics.  Used by the file system adapter to batch rapid change events.
 */

type TimerAPI = {
  setTimeout: (fn: () => void, ms: number) => ReturnType<typeof setTimeout>;
  clearTimeout: (id: ReturnType<typeof setTimeout>) => void;
};

function getTimerAPI(): TimerAPI {
  return globalThis;
}

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
export function debounce(
  fn: (...args: unknown[]) => void,
  delayMs: number = 0,
  keepAlive = false,
): DebouncedFn {
  let timerId: ReturnType<typeof setTimeout> | null = null;
  let savedThis: unknown = null;
  let savedArgs: unknown[] | null = null;
  let pendingTime = 0;
  let deadlineTime = 0;
  let api = getTimerAPI();

  const invoke = () => {
    const ctx = savedThis;
    const args = savedArgs;
    savedThis = null;
    savedArgs = null;
    fn.apply(ctx, args!);
  };

  const check = () => {
    if (pendingTime) {
      const now = Date.now();
      if (now < pendingTime) {
        api = getTimerAPI();
        timerId = api.setTimeout(check, pendingTime - now);
        pendingTime = 0;
        return;
      }
    }
    deadlineTime = 0;
    timerId = null;
    invoke();
  };

  const debounced: DebouncedFn = Object.assign(
    function (this: unknown, ...args: unknown[]) {
      savedThis = this;
      savedArgs = args;
      const now = Date.now();

      if (timerId) {
        if (keepAlive) {
          pendingTime = deadlineTime = now + delayMs;
        } else if (api !== getTimerAPI() && deadlineTime <= now) {
          api.clearTimeout(timerId);
          api = getTimerAPI();
          timerId = api.setTimeout(check, 0);
        }
      } else {
        api = getTimerAPI();
        deadlineTime = now + delayMs;
        timerId = api.setTimeout(check, delayMs);
      }
    },
    {
      cancel() {
        if (timerId) {
          api.clearTimeout(timerId);
          timerId = null;
        }
      },
      run() {
        if (timerId) {
          api.clearTimeout(timerId);
          timerId = null;
          invoke();
        }
      },
    },
  );

  return debounced;
}
