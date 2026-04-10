import { describe, expect, it, vi } from 'vitest';
import { createAbortContext } from './abort-context.js';

describe('createAbortContext', () => {
  it('initial state: signal.aborted is false before abort() is called', () => {
    const ctx = createAbortContext();
    expect(ctx.signal.aborted).toBe(false);
  });

  it('abort() flips signal.aborted to true', () => {
    const ctx = createAbortContext();
    ctx.abort();
    expect(ctx.signal.aborted).toBe(true);
  });

  it('cleanups run in LIFO order when abort() is called', () => {
    const ctx = createAbortContext();
    const order: string[] = [];
    ctx.addCleanup(() => order.push('a'));
    ctx.addCleanup(() => order.push('b'));
    ctx.addCleanup(() => order.push('c'));
    ctx.abort();
    expect(order).toEqual(['c', 'b', 'a']);
  });

  it('cleanup that throws does NOT prevent subsequent cleanups from running', () => {
    const ctx = createAbortContext();
    const firstRan = vi.fn();
    const lastRan = vi.fn();
    ctx.addCleanup(firstRan);
    ctx.addCleanup(() => {
      throw new Error('boom');
    });
    ctx.addCleanup(lastRan);
    // abort() must not throw even if a cleanup throws
    expect(() => ctx.abort()).not.toThrow();
    // Both the first and last cleanups must have run
    expect(firstRan).toHaveBeenCalledTimes(1);
    expect(lastRan).toHaveBeenCalledTimes(1);
  });

  it('abort() is idempotent — second call is a no-op, cleanups run exactly once', () => {
    const ctx = createAbortContext();
    const counter = vi.fn();
    ctx.addCleanup(counter);
    ctx.abort();
    ctx.abort(); // second call
    expect(counter).toHaveBeenCalledTimes(1);
    expect(ctx.signal.aborted).toBe(true);
  });

  it('addCleanup() called after abort() is a no-op for the current context', () => {
    const ctx = createAbortContext();
    ctx.abort();
    // Registering after abort — cleanup should not run (stack already drained)
    const lateCleanup = vi.fn();
    ctx.addCleanup(lateCleanup);
    // Even though we add a cleanup after abort, it should not run
    // (the once-listener has already fired and the stack has been drained)
    expect(lateCleanup).not.toHaveBeenCalled();
  });
});
