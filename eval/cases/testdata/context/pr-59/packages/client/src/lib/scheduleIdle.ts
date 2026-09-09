/**
 * requestIdleCallback with a setTimeout fallback for browsers that
 * don't support the API (Safari < 16). Returns an opaque numeric
 * handle that can be passed to `cancelIdle` for cleanup, so React
 * effect cleanup paths can abort pending work on unmount.
 *
 * `timeoutMs` caps how long the browser is allowed to wait before
 * forcing the callback even if it's still busy — prevents indefinite
 * deferral on busy main threads. Default 200ms is a reasonable
 * "soon-ish" upper bound for non-critical background work.
 */
const hasIdleApi = typeof globalThis.requestIdleCallback === "function";

export function scheduleIdle(cb: () => void, timeoutMs = 200): number {
  if (hasIdleApi) {
    return globalThis.requestIdleCallback!(cb, { timeout: timeoutMs });
  }
  return setTimeout(cb, 1) as unknown as number;
}

export function cancelIdle(handle: number): void {
  if (hasIdleApi) {
    globalThis.cancelIdleCallback!(handle);
  } else {
    clearTimeout(handle);
  }
}
