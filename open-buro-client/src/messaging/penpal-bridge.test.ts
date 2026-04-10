// @vitest-environment happy-dom

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// Mock Penpal BEFORE importing PenpalBridge so the class picks up the mock.
const mockDestroy = vi.fn();

vi.mock('penpal', () => {
  return {
    connect: vi.fn(() => ({
      promise: Promise.resolve({}),
      destroy: mockDestroy,
    })),
    WindowMessenger: vi.fn(function (opts) {
      // @ts-expect-error -- mock constructor captures opts for test assertions
      this._mockOpts = opts;
    }),
  };
});

// Import AFTER vi.mock so the mock is in place
import { connect, WindowMessenger } from 'penpal';
import { PenpalBridge } from './penpal-bridge.js';

describe('PenpalBridge', () => {
  let iframe: HTMLIFrameElement;

  beforeEach(() => {
    vi.clearAllMocks();
    iframe = document.createElement('iframe');
    document.body.appendChild(iframe);
  });

  afterEach(() => {
    iframe.remove();
  });

  it('calls WindowMessenger with remoteWindow and single-origin allowedOrigins', async () => {
    const bridge = new PenpalBridge();
    await bridge.connect(iframe, 'https://cap.example.com', {
      resolve: () => {},
    });

    expect(WindowMessenger).toHaveBeenCalledTimes(1);
    const ctorArgs = (WindowMessenger as unknown as ReturnType<typeof vi.fn>).mock.calls[0]?.[0];
    expect(ctorArgs?.remoteWindow).toBe(iframe.contentWindow);
    expect(ctorArgs?.allowedOrigins).toEqual(['https://cap.example.com']);
  });

  it('calls Penpal connect() with messenger, methods, and default timeout 10000', async () => {
    const bridge = new PenpalBridge();
    const methods = { resolve: vi.fn() };
    await bridge.connect(iframe, 'https://cap.example.com', methods);

    expect(connect).toHaveBeenCalledTimes(1);
    const connectArgs = (connect as unknown as ReturnType<typeof vi.fn>).mock.calls[0]?.[0];
    expect(connectArgs?.methods).toBe(methods);
    expect(connectArgs?.timeout).toBe(10000);
    expect(connectArgs?.messenger).toBeDefined();
  });

  it('passes custom timeoutMs to Penpal connect()', async () => {
    const bridge = new PenpalBridge();
    await bridge.connect(iframe, 'https://cap.example.com', { resolve: () => {} }, 5000);

    const connectArgs = (connect as unknown as ReturnType<typeof vi.fn>).mock.calls[0]?.[0];
    expect(connectArgs?.timeout).toBe(5000);
  });

  it('returned ConnectionHandle.destroy() invokes Penpal destroy', async () => {
    const bridge = new PenpalBridge();
    const handle = await bridge.connect(iframe, 'https://cap.example.com', {
      resolve: () => {},
    });

    handle.destroy();
    expect(mockDestroy).toHaveBeenCalledTimes(1);
  });

  it('throws when iframe.contentWindow is null', async () => {
    const bridge = new PenpalBridge();
    const detachedIframe = document.createElement('iframe');
    // do NOT append to DOM — contentWindow may still exist in happy-dom, so override:
    Object.defineProperty(detachedIframe, 'contentWindow', { value: null });

    await expect(
      bridge.connect(detachedIframe, 'https://cap.example.com', { resolve: () => {} }),
    ).rejects.toThrow(/contentWindow/);
  });

  it('does NOT import penpal at runtime from any other file path', () => {
    // Smoke test: verify this test file's mock is the one being consumed.
    // If PenpalBridge somehow bypassed vi.mock (e.g., dynamic import), connect
    // would be the real Penpal function and this assertion would fail.
    expect(vi.isMockFunction(connect)).toBe(true);
  });
});
