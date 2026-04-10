/**
 * AbortContext — a small wrapper around AbortController that also holds
 * a LIFO stack of cleanup functions to run on abort. Phase 3's orchestrator
 * composes one of these per session and calls `abort()` inside `destroy()`
 * to tear down every registered fetch, listener, and WebSocket leak-free.
 *
 * Contract (LIFECYCLE-01):
 * - `signal` can be passed to fetch/addEventListener/etc. via `{ signal }`.
 * - `abort()` flips `signal.aborted` to true and drains the cleanup stack in LIFO order.
 * - A cleanup that throws does NOT prevent subsequent cleanups from running.
 * - `abort()` is idempotent — a second call is a no-op.
 */
export interface AbortContext {
  readonly signal: AbortSignal;
  abort(reason?: unknown): void;
  addCleanup(fn: () => void): void;
}

export function createAbortContext(): AbortContext {
  const controller = new AbortController();
  const cleanups: Array<() => void> = [];

  controller.signal.addEventListener(
    'abort',
    () => {
      while (cleanups.length > 0) {
        const fn = cleanups.pop();
        try {
          fn?.();
        } catch {
          // Swallow errors during teardown so subsequent cleanups still run.
        }
      }
    },
    { once: true },
  );

  return {
    signal: controller.signal,
    abort: (reason) => {
      if (!controller.signal.aborted) {
        controller.abort(reason);
      }
    },
    addCleanup: (fn) => {
      cleanups.push(fn);
    },
  };
}
