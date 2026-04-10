import { describe, expect, it } from 'vitest';
import {
  type Capability,
  type CastPlan,
  type FileResult,
  generateSessionId,
  type IntentCallback,
  type IntentRequest,
  type IntentResult,
  OBCError,
  type OBCErrorCode,
  type OBCOptions,
} from './index';

describe('public API surface (FOUND-04)', () => {
  it('exports OBCError as a constructor', () => {
    expect(typeof OBCError).toBe('function');
    expect(new OBCError('INTENT_CANCELLED', 'x')).toBeInstanceOf(Error);
  });

  it('exports generateSessionId as a function returning a UUID-shaped string', () => {
    expect(typeof generateSessionId).toBe('function');
    expect(generateSessionId()).toMatch(/^[0-9a-f-]{36}$/i);
  });

  it('all shared types are assignable (compile-time check)', () => {
    const cap: Capability = {
      id: 'x',
      appName: 'X',
      action: 'PICK',
      path: 'https://x/',
      properties: { mimeTypes: ['*/*'] },
    };
    const req: IntentRequest = { action: 'PICK', args: {} };
    const file: FileResult = { name: 'f', type: 't', size: 0, url: 'u' };
    const res: IntentResult = { id: 'id', status: 'done', results: [file] };
    const cb: IntentCallback = (_r) => {};
    const opts: OBCOptions = { capabilitiesUrl: 'https://x/' };
    const plan: CastPlan = { kind: 'no-match' };
    const code: OBCErrorCode = 'CAPABILITIES_FETCH_FAILED';
    expect([cap, req, file, res, cb, opts, plan, code]).toBeDefined();
  });
});

describe('public API surface — Phase 2', () => {
  it('re-exports capabilities layer', async () => {
    const mod = await import('./index');
    expect(typeof mod.resolve).toBe('function');
    expect(typeof mod.fetchCapabilities).toBe('function');
    expect(typeof mod.WsListener).toBe('function'); // class
    expect(typeof mod.deriveWsUrl).toBe('function');
  });

  it('re-exports lifecycle layer', async () => {
    const mod = await import('./index');
    expect(typeof mod.createAbortContext).toBe('function');
    const ctx = mod.createAbortContext();
    expect(ctx.signal.aborted).toBe(false);
    ctx.abort();
    expect(ctx.signal.aborted).toBe(true);
  });

  it('re-exports intent layer', async () => {
    const mod = await import('./index');
    expect(typeof mod.planCast).toBe('function');
    // ActiveSession is a type; compile-time only
  });

  it('re-exports UI layer', async () => {
    const mod = await import('./index');
    expect(typeof mod.createShadowHost).toBe('function');
    expect(typeof mod.createSpinnerOverlay).toBe('function');
    expect(typeof mod.lockBodyScroll).toBe('function');
    expect(typeof mod.buildIframe).toBe('function');
    expect(typeof mod.trapFocus).toBe('function');
    expect(typeof mod.buildModal).toBe('function');
  });

  it('re-exports messaging layer', async () => {
    const mod = await import('./index');
    expect(typeof mod.MockBridge).toBe('function'); // class
    expect(typeof mod.PenpalBridge).toBe('function'); // class
  });

  it('planCast returns the no-match branch for empty input', async () => {
    const { planCast } = await import('./index');
    const result = planCast([]);
    expect(result.kind).toBe('no-match');
  });
});
