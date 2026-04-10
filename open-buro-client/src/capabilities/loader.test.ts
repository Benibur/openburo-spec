import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Capability } from '../types.js';
import { fetchCapabilities } from './loader.js';

const fakeCap: Capability = {
  id: 'test-cap',
  appName: 'Test App',
  action: 'PICK',
  path: 'https://testapp.example.com/cap',
  properties: { mimeTypes: ['image/png'] },
};

function makeResponse(opts: {
  ok: boolean;
  status: number;
  xOpenBuroServer?: string;
  body?: unknown;
}) {
  return {
    ok: opts.ok,
    status: opts.status,
    headers: {
      get: (name: string) => (name === 'X-OpenBuro-Server' ? (opts.xOpenBuroServer ?? null) : null),
    },
    json: async () => opts.body ?? [],
  };
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('fetchCapabilities', () => {
  it('happy path: returns capabilities and isOpenBuroServer=true when header is "true"', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          makeResponse({ ok: true, status: 200, xOpenBuroServer: 'true', body: [fakeCap] }),
        ),
    );
    const result = await fetchCapabilities('https://api.example.com/capabilities');
    expect(result.isOpenBuroServer).toBe(true);
    expect(result.capabilities).toHaveLength(1);
    expect(result.capabilities[0]).toEqual(fakeCap);
  });

  it('header absent: returns isOpenBuroServer=false when X-OpenBuro-Server header is not present', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(makeResponse({ ok: true, status: 200, body: [fakeCap] })),
    );
    const result = await fetchCapabilities('https://api.example.com/capabilities');
    expect(result.isOpenBuroServer).toBe(false);
  });

  it('non-200: rejects with OBCError(CAPABILITIES_FETCH_FAILED) on 500 response', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(makeResponse({ ok: false, status: 500 })));
    await expect(fetchCapabilities('https://api.example.com/capabilities')).rejects.toMatchObject({
      code: 'CAPABILITIES_FETCH_FAILED',
    });
  });

  it('fetch throws (network error): rejects with OBCError and cause is original thrown value', async () => {
    const networkError = new Error('Network failure');
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(networkError));
    const err = await fetchCapabilities('https://api.example.com/capabilities').catch(
      (e: unknown) => e,
    );
    expect(err).toMatchObject({ code: 'CAPABILITIES_FETCH_FAILED' });
    // @ts-expect-error accessing cause on unknown error
    expect(err.cause).toBe(networkError);
  });

  it('HTTPS guard: rejects before calling fetch when URL is http:// and location is https:', async () => {
    vi.stubGlobal('location', { protocol: 'https:' });
    const fetchSpy = vi.fn();
    vi.stubGlobal('fetch', fetchSpy);
    await expect(fetchCapabilities('http://example.com/capabilities')).rejects.toMatchObject({
      code: 'CAPABILITIES_FETCH_FAILED',
    });
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it('AbortController: rejects when signal is aborted during fetch', async () => {
    const controller = new AbortController();
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation(
        (_url: string, opts?: RequestInit) =>
          new Promise((_resolve, reject) => {
            const signal = opts?.signal;
            if (signal?.aborted) {
              reject(new DOMException('aborted', 'AbortError'));
              return;
            }
            signal?.addEventListener('abort', () => {
              reject(new DOMException('aborted', 'AbortError'));
            });
          }),
      ),
    );
    const promise = fetchCapabilities('https://api.example.com/capabilities', controller.signal);
    controller.abort();
    await expect(promise).rejects.toMatchObject({ code: 'CAPABILITIES_FETCH_FAILED' });
  });
});
