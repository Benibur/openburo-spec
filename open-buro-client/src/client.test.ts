// @vitest-environment happy-dom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { WsListener } from './capabilities/ws-listener';
import { OpenBuroClient } from './client';
import { OBCError } from './errors';
import { MockBridge } from './messaging/mock-bridge';
import type { Capability } from './types';

const VALID_URL = 'https://api.example.com/capabilities';
const SAMPLE_CAPS: Capability[] = [
  {
    id: 'pick-1',
    appName: 'Sample Picker',
    action: 'PICK',
    path: 'https://files.example.com/pick',
    properties: { mimeTypes: ['*/*'] },
  },
];

function mockFetchOk(caps: Capability[], isOpenBuroServer = true) {
  return vi.fn(
    async () =>
      new Response(JSON.stringify(caps), {
        status: 200,
        headers: isOpenBuroServer ? { 'X-OpenBuro-Server': 'true' } : {},
      }),
  );
}

function makeCap(id: string, action: string, mimes: string[] = ['*/*']): Capability {
  return {
    id,
    appName: `App ${id}`,
    action,
    path: `https://apps.example.com/${id}`,
    properties: { mimeTypes: mimes },
  };
}

beforeEach(() => {
  vi.unstubAllGlobals();
  // Disable child frame navigation so happy-dom does not make real network requests
  // when iframes are appended to the shadow DOM during orchestrator tests.
  // Casting through unknown because the vitest-env.d.ts uses an index signature.
  const settings = window.happyDOM.settings as unknown as {
    disableIframePageLoading: boolean;
    navigation: { disableChildFrameNavigation: boolean };
  };
  settings.disableIframePageLoading = true;
  settings.navigation.disableChildFrameNavigation = true;
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  document.body.innerHTML = '';
});

// ---------------------------------------------------------------------------
// Task 2: Constructor and capability access
// ---------------------------------------------------------------------------
describe('OpenBuroClient - constructor and capability access', () => {
  it('constructs with https URL without calling fetch', () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);
    const obc = new OpenBuroClient({ capabilitiesUrl: VALID_URL });
    expect(fetchMock).not.toHaveBeenCalled();
    obc.destroy();
  });

  it('throws CAPABILITIES_FETCH_FAILED on non-HTTPS non-localhost url', () => {
    expect(() => new OpenBuroClient({ capabilitiesUrl: 'http://example.com/caps' })).toThrow(
      OBCError,
    );
    expect(() => new OpenBuroClient({ capabilitiesUrl: 'http://example.com/caps' })).toThrow(
      expect.objectContaining({ code: 'CAPABILITIES_FETCH_FAILED' }),
    );
  });

  it('does NOT throw for http://localhost capabilitiesUrl', () => {
    expect(
      () => new OpenBuroClient({ capabilitiesUrl: 'http://localhost:3000/capabilities' }),
    ).not.toThrow();
  });

  it('throws WS_CONNECTION_FAILED on insecure wsUrl', () => {
    expect(
      () =>
        new OpenBuroClient({
          capabilitiesUrl: VALID_URL,
          wsUrl: 'ws://evil.com',
        }),
    ).toThrow(expect.objectContaining({ code: 'WS_CONNECTION_FAILED' }));
  });

  it('does NOT throw for ws://localhost wsUrl', () => {
    expect(
      () =>
        new OpenBuroClient({
          capabilitiesUrl: VALID_URL,
          wsUrl: 'ws://localhost:3000/ws',
        }),
    ).not.toThrow();
  });

  it('getCapabilities() returns [] synchronously before any fetch', () => {
    const obc = new OpenBuroClient({ capabilitiesUrl: VALID_URL });
    expect(obc.getCapabilities()).toEqual([]);
    obc.destroy();
  });

  it('refreshCapabilities() resolves with fetched capabilities', async () => {
    vi.stubGlobal('fetch', mockFetchOk(SAMPLE_CAPS, false));
    const obc = new OpenBuroClient({ capabilitiesUrl: VALID_URL });
    const caps = await obc.refreshCapabilities();
    expect(caps).toEqual(SAMPLE_CAPS);
    obc.destroy();
  });

  it('two concurrent refreshCapabilities() share a single in-flight Promise (fetch called once)', async () => {
    const fetchMock = mockFetchOk(SAMPLE_CAPS, false);
    vi.stubGlobal('fetch', fetchMock);
    const obc = new OpenBuroClient({ capabilitiesUrl: VALID_URL });
    const [a, b] = await Promise.all([obc.refreshCapabilities(), obc.refreshCapabilities()]);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(a).toEqual(SAMPLE_CAPS);
    expect(b).toEqual(SAMPLE_CAPS);
    obc.destroy();
  });

  it('after successful refreshCapabilities, getCapabilities() returns the list synchronously', async () => {
    vi.stubGlobal('fetch', mockFetchOk(SAMPLE_CAPS, false));
    const obc = new OpenBuroClient({ capabilitiesUrl: VALID_URL });
    await obc.refreshCapabilities();
    expect(obc.getCapabilities()).toEqual(SAMPLE_CAPS);
    obc.destroy();
  });

  it('failed refreshCapabilities calls onError and rejects; next call retries (fetch called twice)', async () => {
    const onError = vi.fn();
    let callCount = 0;
    const fetchMock = vi.fn(async () => {
      callCount += 1;
      if (callCount === 1) {
        throw new Error('network failure');
      }
      return new Response(JSON.stringify(SAMPLE_CAPS), { status: 200 });
    });
    vi.stubGlobal('fetch', fetchMock);
    const obc = new OpenBuroClient({ capabilitiesUrl: VALID_URL, onError });

    await expect(obc.refreshCapabilities()).rejects.toMatchObject({
      code: 'CAPABILITIES_FETCH_FAILED',
    });
    expect(onError).toHaveBeenCalledTimes(1);

    // Second call should retry
    const caps = await obc.refreshCapabilities();
    expect(caps).toEqual(SAMPLE_CAPS);
    expect(fetchMock).toHaveBeenCalledTimes(2);
    obc.destroy();
  });

  it('two separate OpenBuroClient instances maintain isolated capability state', async () => {
    const caps1: Capability[] = [makeCap('cap-a', 'PICK')];
    const caps2: Capability[] = [makeCap('cap-b', 'SAVE')];
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValueOnce(new Response(JSON.stringify(caps1), { status: 200 }))
        .mockResolvedValueOnce(new Response(JSON.stringify(caps2), { status: 200 })),
    );
    const obc1 = new OpenBuroClient({ capabilitiesUrl: 'https://a.example.com/caps' });
    const obc2 = new OpenBuroClient({ capabilitiesUrl: 'https://b.example.com/caps' });
    await obc1.refreshCapabilities();
    await obc2.refreshCapabilities();
    expect(obc1.getCapabilities()).toEqual(caps1);
    expect(obc2.getCapabilities()).toEqual(caps2);
    obc1.destroy();
    obc2.destroy();
  });
});

// ---------------------------------------------------------------------------
// Task 3: castIntent orchestration
// ---------------------------------------------------------------------------
describe('OpenBuroClient - castIntent orchestration', () => {
  const PICK_ACTION = 'PICK';
  const SAVE_ACTION = 'SAVE';

  it('happy-path: one match → direct iframe → resolve routes to callback', async () => {
    vi.stubGlobal('fetch', mockFetchOk([makeCap('a', PICK_ACTION)]));
    const bridge = new MockBridge();
    const obc = new OpenBuroClient({
      capabilitiesUrl: VALID_URL,
      bridgeFactory: bridge,
    });

    const cb = vi.fn();
    const castPromise = obc.castIntent({ action: PICK_ACTION, args: {} }, cb);

    // Wait a microtask for single-flight fetch + iframe sync block + await connect
    await new Promise<void>((r) => setTimeout(r, 0));
    expect(bridge.connectCallCount).toBe(1);
    expect(bridge.lastMethods).not.toBeNull();

    // Get session id from debug helper
    const ids = obc.__debugGetActiveSessionIds();
    expect(ids.length).toBe(1);
    const activeId = ids[0];
    expect(activeId).toBeDefined();

    // Simulate child iframe resolving
    bridge.lastMethods?.resolve({
      id: activeId as string,
      status: 'done',
      results: [{ name: 'a.txt', type: 'text/plain', size: 1, url: 'blob:x' }],
    });

    const result = await castPromise;
    expect(result.status).toBe('done');
    expect(cb).toHaveBeenCalledTimes(1);
    const cbArg = cb.mock.calls[0]?.[0];
    expect(cbArg?.status).toBe('done');
    expect(bridge.destroyCallCount).toBe(1);

    obc.destroy();
  });

  it('no-match: fires NO_MATCHING_CAPABILITY + cancel callback', async () => {
    vi.stubGlobal('fetch', mockFetchOk([makeCap('a', PICK_ACTION)]));
    const onError = vi.fn();
    const obc = new OpenBuroClient({
      capabilitiesUrl: VALID_URL,
      bridgeFactory: new MockBridge(),
      onError,
    });
    const cb = vi.fn();
    const result = await obc.castIntent({ action: SAVE_ACTION, args: {} }, cb);
    expect(result.status).toBe('cancel');
    expect(cb).toHaveBeenCalledTimes(1);
    expect(onError).toHaveBeenCalledTimes(1);
    const errArg = onError.mock.calls[0]?.[0];
    expect(errArg?.code).toBe('NO_MATCHING_CAPABILITY');
    obc.destroy();
  });

  it('select: >1 matches → modal rendered with role=dialog, selecting capability opens iframe', async () => {
    vi.stubGlobal('fetch', mockFetchOk([makeCap('a', PICK_ACTION), makeCap('b', PICK_ACTION)]));
    const bridge = new MockBridge();
    const obc = new OpenBuroClient({
      capabilitiesUrl: VALID_URL,
      bridgeFactory: bridge,
    });

    const cb = vi.fn();
    const castPromise = obc.castIntent({ action: PICK_ACTION, args: {} }, cb);

    // Wait for fetch to resolve and modal to render
    await new Promise<void>((r) => setTimeout(r, 0));

    // Modal should be in the shadow DOM
    const host = document.querySelector('[data-obc-host]');
    expect(host).not.toBeNull();
    const shadowRoot = host?.shadowRoot;
    const dialog = shadowRoot?.querySelector('[role="dialog"]');
    expect(dialog).not.toBeNull();

    // Select the second capability (cap 'b')
    const options = shadowRoot?.querySelectorAll('[role="option"]');
    expect(options?.length).toBe(2);
    (options?.[1] as HTMLElement)?.click();

    // Wait for iframe to open
    await new Promise<void>((r) => setTimeout(r, 0));
    expect(bridge.connectCallCount).toBe(1);

    // Resolve from bridge
    const ids = obc.__debugGetActiveSessionIds();
    bridge.lastMethods?.resolve({
      id: ids[0] as string,
      status: 'done',
      results: [],
    });

    const result = await castPromise;
    expect(result.status).toBe('done');
    obc.destroy();
  });

  it('select cancel: clicking cancel fires cancel callback, no iframe opened', async () => {
    vi.stubGlobal('fetch', mockFetchOk([makeCap('a', PICK_ACTION), makeCap('b', PICK_ACTION)]));
    const bridge = new MockBridge();
    const cb = vi.fn();
    const obc = new OpenBuroClient({
      capabilitiesUrl: VALID_URL,
      bridgeFactory: bridge,
    });

    const castPromise = obc.castIntent({ action: PICK_ACTION, args: {} }, cb);
    await new Promise<void>((r) => setTimeout(r, 0));

    // Click cancel button inside shadow DOM
    const host = document.querySelector('[data-obc-host]');
    const shadowRoot = host?.shadowRoot;
    const cancelBtn = shadowRoot?.querySelector('button') as HTMLButtonElement | null;
    cancelBtn?.click();

    const result = await castPromise;
    expect(result.status).toBe('cancel');
    expect(cb).toHaveBeenCalledWith(expect.objectContaining({ status: 'cancel' }));
    expect(bridge.connectCallCount).toBe(0);
    obc.destroy();
  });

  it('two concurrent sessions route resolve to the correct callback by id', async () => {
    vi.stubGlobal('fetch', mockFetchOk([makeCap('a', PICK_ACTION)]));
    const bridge = new MockBridge();
    const obc = new OpenBuroClient({
      capabilitiesUrl: VALID_URL,
      bridgeFactory: bridge,
    });

    const cb1 = vi.fn();
    const cb2 = vi.fn();
    const promise1 = obc.castIntent({ action: PICK_ACTION, args: {} }, cb1);
    const promise2 = obc.castIntent({ action: PICK_ACTION, args: {} }, cb2);

    // Wait for both sessions to be established
    await new Promise<void>((r) => setTimeout(r, 20));

    const ids = obc.__debugGetActiveSessionIds();
    expect(ids.length).toBe(2);

    const id1 = ids[0] as string;
    const id2 = ids[1] as string;

    // Resolve session 2 first
    bridge.lastMethods?.resolve({ id: id2, status: 'done', results: [] });
    await new Promise<void>((r) => setTimeout(r, 0));

    expect(cb1).not.toHaveBeenCalled();
    expect(cb2).toHaveBeenCalledTimes(1);
    expect(cb2.mock.calls[0]?.[0]?.status).toBe('done');

    // Then resolve session 1
    bridge.lastMethods?.resolve({ id: id1, status: 'done', results: [] });
    const r1 = await promise1;
    await promise2;

    expect(cb1).toHaveBeenCalledTimes(1);
    expect(r1.status).toBe('done');
    obc.destroy();
  });

  it('unknown/stale session id is silently ignored — no throw, no callback invoked', async () => {
    vi.stubGlobal('fetch', mockFetchOk([makeCap('a', PICK_ACTION)]));
    const bridge = new MockBridge();
    const obc = new OpenBuroClient({
      capabilitiesUrl: VALID_URL,
      bridgeFactory: bridge,
    });

    const cb = vi.fn();
    void obc.castIntent({ action: PICK_ACTION, args: {} }, cb);
    await new Promise<void>((r) => setTimeout(r, 0));

    // INT-09: unknown id — must be silently ignored
    expect(() => {
      bridge.lastMethods?.resolve({ id: 'bogus-session-id', status: 'done', results: [] });
    }).not.toThrow();

    expect(cb).not.toHaveBeenCalled();
    obc.destroy();
  });

  it('same-origin capability: castIntent fires cancel callback and SAME_ORIGIN_CAPABILITY error', async () => {
    // In happy-dom, location.origin is typically about:blank or http://localhost
    // We need a capability whose path matches location.origin
    const sameOriginPath = `${location.origin}/pick`;
    const sameOriginCap: Capability = {
      id: 'same',
      appName: 'SameOriginApp',
      action: PICK_ACTION,
      path: sameOriginPath,
      properties: { mimeTypes: ['*/*'] },
    };
    vi.stubGlobal('fetch', mockFetchOk([sameOriginCap]));
    const onError = vi.fn();
    const obc = new OpenBuroClient({
      capabilitiesUrl: VALID_URL,
      bridgeFactory: new MockBridge(),
      onError,
    });

    const cb = vi.fn();
    const result = await obc.castIntent({ action: PICK_ACTION, args: {} }, cb);
    expect(result.status).toBe('cancel');
    expect(cb).toHaveBeenCalledWith(expect.objectContaining({ status: 'cancel' }));
    expect(onError).toHaveBeenCalledWith(
      expect.objectContaining({ code: 'SAME_ORIGIN_CAPABILITY' }),
    );
    obc.destroy();
  });

  it('single-flight cold start: two concurrent castIntents share one fetch call', async () => {
    const fetchMock = mockFetchOk([makeCap('a', PICK_ACTION)]);
    vi.stubGlobal('fetch', fetchMock);
    const bridge = new MockBridge();
    const obc = new OpenBuroClient({
      capabilitiesUrl: VALID_URL,
      bridgeFactory: bridge,
    });

    const cb = vi.fn();
    const p1 = obc.castIntent({ action: PICK_ACTION, args: {} }, cb);
    const p2 = obc.castIntent({ action: PICK_ACTION, args: {} }, cb);

    await new Promise<void>((r) => setTimeout(r, 20));

    expect(fetchMock).toHaveBeenCalledTimes(1);

    // Clean up: resolve sessions
    const ids = obc.__debugGetActiveSessionIds();
    for (const id of ids) {
      bridge.lastMethods?.resolve({ id, status: 'done', results: [] });
    }
    await Promise.allSettled([p1, p2]);
    obc.destroy();
  });

  it('watchdog: session times out after sessionTimeoutMs and fires IFRAME_TIMEOUT cancel', async () => {
    // Use real timers with a very short timeout (50ms) to avoid fake-timer complexity
    vi.stubGlobal('fetch', mockFetchOk([makeCap('a', PICK_ACTION)]));
    const onError = vi.fn();
    const bridge = new MockBridge();
    const obc = new OpenBuroClient({
      capabilitiesUrl: VALID_URL,
      bridgeFactory: bridge,
      onError,
      sessionTimeoutMs: 50,
    });

    const cb = vi.fn();
    const castPromise = obc.castIntent({ action: PICK_ACTION, args: {} }, cb);

    // Wait for session to be established (fetch + connect)
    await new Promise<void>((r) => setTimeout(r, 20));
    expect(obc.__debugGetActiveSessionIds().length).toBe(1);

    // Wait for the 50ms watchdog to fire
    const result = await castPromise;
    expect(result.status).toBe('cancel');
    expect(cb).toHaveBeenCalledWith(expect.objectContaining({ status: 'cancel' }));
    expect(onError).toHaveBeenCalledWith(expect.objectContaining({ code: 'IFRAME_TIMEOUT' }));

    obc.destroy();
  }, 1000);

  it('two instances are isolated: destroy on instance A does not affect instance B sessions', async () => {
    const caps = [makeCap('a', PICK_ACTION)];
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify(caps), {
          status: 200,
          headers: { 'X-OpenBuro-Server': 'true' },
        }),
      ),
    );
    const bridgeA = new MockBridge();
    const bridgeB = new MockBridge();

    const obcA = new OpenBuroClient({
      capabilitiesUrl: 'https://a.example.com/caps',
      bridgeFactory: bridgeA,
    });
    const obcB = new OpenBuroClient({
      capabilitiesUrl: 'https://b.example.com/caps',
      bridgeFactory: bridgeB,
    });

    const cbA = vi.fn();
    const cbB = vi.fn();
    const promiseB = obcB.castIntent({ action: PICK_ACTION, args: {} }, cbB);

    await new Promise<void>((r) => setTimeout(r, 20));

    // Destroy A — should not affect B
    obcA.destroy();

    const idsB = obcB.__debugGetActiveSessionIds();
    expect(idsB.length).toBe(1);

    bridgeB.lastMethods?.resolve({ id: idsB[0] as string, status: 'done', results: [] });
    const resultB = await promiseB;
    expect(resultB.status).toBe('done');
    expect(cbA).not.toHaveBeenCalled();
    expect(cbB).toHaveBeenCalledTimes(1);

    obcB.destroy();
  });
});

// ---------------------------------------------------------------------------
// Task 4: destroy and post-destroy semantics
// ---------------------------------------------------------------------------
describe('OpenBuroClient - destroy and post-destroy', () => {
  it('destroy during active session cancels callback and removes DOM nodes', async () => {
    vi.stubGlobal('fetch', mockFetchOk([makeCap('a', 'PICK')]));
    const bridge = new MockBridge();
    const obc = new OpenBuroClient({
      capabilitiesUrl: VALID_URL,
      bridgeFactory: bridge,
    });
    const cb = vi.fn();
    void obc.castIntent({ action: 'PICK', args: {} }, cb);
    await new Promise<void>((r) => setTimeout(r, 0));
    expect(document.querySelectorAll('[data-obc-host]').length).toBe(1);

    obc.destroy();

    expect(cb).toHaveBeenCalledTimes(1);
    const cbArg = cb.mock.calls[0]?.[0];
    expect(cbArg?.status).toBe('cancel');
    expect(bridge.destroyCallCount).toBe(1);
    expect(document.querySelectorAll('[data-obc-host]').length).toBe(0);
  });

  it('destroy is idempotent — second call does not throw', () => {
    const obc = new OpenBuroClient({ capabilitiesUrl: VALID_URL });
    expect(() => {
      obc.destroy();
      obc.destroy();
    }).not.toThrow();
  });

  it('destroy aborts in-flight fetch (signal.aborted is true)', async () => {
    let capturedSignal: AbortSignal | undefined;
    let resolveFetch!: (v: Response) => void;
    vi.stubGlobal(
      'fetch',
      vi.fn((_url: string, opts?: RequestInit) => {
        capturedSignal = opts?.signal as AbortSignal | undefined;
        return new Promise<Response>((resolve) => {
          resolveFetch = resolve;
        });
      }),
    );
    const obc = new OpenBuroClient({ capabilitiesUrl: VALID_URL });
    void obc.refreshCapabilities().catch(() => {});
    await new Promise<void>((r) => setTimeout(r, 0));
    obc.destroy();
    expect(capturedSignal?.aborted).toBe(true);
    // Unblock the hanging fetch so happy-dom doesn't time out on teardown
    resolveFetch(new Response('[]', { status: 200 }));
  });

  it('destroy stops WsListener when one is active', async () => {
    const stopSpy = vi.spyOn(WsListener.prototype, 'stop');
    const startSpy = vi.spyOn(WsListener.prototype, 'start').mockImplementation(() => {
      // no-op: don't open real WebSocket connections in happy-dom
    });
    vi.stubGlobal('fetch', mockFetchOk([makeCap('a', 'PICK')], true));
    const obc = new OpenBuroClient({
      capabilitiesUrl: VALID_URL,
      bridgeFactory: new MockBridge(),
    });
    await obc.refreshCapabilities();
    expect(startSpy).toHaveBeenCalled();
    obc.destroy();
    expect(stopSpy).toHaveBeenCalled();
  });

  it('post-destroy castIntent rejects with DESTROYED', async () => {
    const obc = new OpenBuroClient({ capabilitiesUrl: VALID_URL });
    obc.destroy();
    await expect(obc.castIntent({ action: 'PICK', args: {} }, () => {})).rejects.toMatchObject({
      code: 'DESTROYED',
    });
  });

  it('post-destroy getCapabilities() returns []', () => {
    const obc = new OpenBuroClient({ capabilitiesUrl: VALID_URL });
    obc.destroy();
    expect(obc.getCapabilities()).toEqual([]);
  });

  it('post-destroy refreshCapabilities() returns rejected Promise with DESTROYED', async () => {
    const obc = new OpenBuroClient({ capabilitiesUrl: VALID_URL });
    obc.destroy();
    await expect(obc.refreshCapabilities()).rejects.toMatchObject({ code: 'DESTROYED' });
  });

  it('destroy removes all [data-obc-host] nodes from document.body', async () => {
    vi.stubGlobal('fetch', mockFetchOk([makeCap('a', 'PICK'), makeCap('b', 'PICK')]));
    const obc = new OpenBuroClient({
      capabilitiesUrl: VALID_URL,
      bridgeFactory: new MockBridge(),
    });
    // Trigger a castIntent so DOM is created
    const cb = vi.fn();
    void obc.castIntent({ action: 'PICK', args: {} }, cb);
    await new Promise<void>((r) => setTimeout(r, 0));
    // Should have shadow host(s) in DOM
    expect(document.querySelectorAll('[data-obc-host]').length).toBeGreaterThan(0);

    obc.destroy();
    expect(document.querySelectorAll('[data-obc-host]').length).toBe(0);
  });

  it('body scroll restored after destroy during a live modal', async () => {
    vi.stubGlobal('fetch', mockFetchOk([makeCap('a', 'PICK'), makeCap('b', 'PICK')]));
    const obc = new OpenBuroClient({
      capabilitiesUrl: VALID_URL,
      bridgeFactory: new MockBridge(),
    });
    const cb = vi.fn();
    void obc.castIntent({ action: 'PICK', args: {} }, cb);
    await new Promise<void>((r) => setTimeout(r, 0));

    // Scroll is locked by the modal
    obc.destroy();

    // After destroy the scroll lock should be removed
    expect(document.body.style.overflow).not.toBe('hidden');
  });

  it('destroy on instance A does not cancel instance B sessions', async () => {
    const caps = [makeCap('a', 'PICK')];
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(new Response(JSON.stringify(caps), { status: 200 })),
    );
    const bridgeA = new MockBridge();
    const bridgeB = new MockBridge();

    const obcA = new OpenBuroClient({
      capabilitiesUrl: 'https://a.example.com/caps',
      bridgeFactory: bridgeA,
    });
    const obcB = new OpenBuroClient({
      capabilitiesUrl: 'https://b.example.com/caps',
      bridgeFactory: bridgeB,
    });

    const cbB = vi.fn();
    const promiseB = obcB.castIntent({ action: 'PICK', args: {} }, cbB);
    await new Promise<void>((r) => setTimeout(r, 20));

    // Destroy A — B's sessions must remain intact
    obcA.destroy();

    const idsB = obcB.__debugGetActiveSessionIds();
    expect(idsB.length).toBe(1);
    bridgeB.lastMethods?.resolve({ id: idsB[0] as string, status: 'done', results: [] });
    const resultB = await promiseB;
    expect(resultB.status).toBe('done');
    expect(cbB).toHaveBeenCalledTimes(1);
    obcB.destroy();
  });
});
