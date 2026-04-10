// @vitest-environment happy-dom
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { OBCError } from '../errors.js';
import type { Capability } from '../types.js';
import { buildIframe } from './iframe.js';

const crossOriginCap: Capability = {
  id: 'cap-1',
  appName: 'Cloud Picker',
  action: 'PICK',
  path: 'https://picker.example.com/picker',
  properties: { mimeTypes: ['*/*'] },
};

const defaultParams = {
  id: 'sess-1',
  clientUrl: 'https://host.example.com',
  type: 'PICK',
};

describe('buildIframe', () => {
  // Set a non-matching origin so same-origin guard doesn't trigger by default
  beforeEach(() => {
    window.happyDOM.setURL('https://host.example.com');
  });

  afterEach(() => {
    window.happyDOM.setURL('https://localhost/');
  });

  it('returns an HTMLIFrameElement for cross-origin capability', () => {
    const el = buildIframe(crossOriginCap, defaultParams);
    expect(el instanceof HTMLIFrameElement).toBe(true);
  });

  it('sets iframe.title to capability.appName (WCAG 2.4.1)', () => {
    const el = buildIframe(crossOriginCap, defaultParams);
    expect(el.title).toBe('Cloud Picker');
  });

  it('sets sandbox attribute to exact required string', () => {
    const el = buildIframe(crossOriginCap, defaultParams);
    expect(el.getAttribute('sandbox')).toBe(
      'allow-scripts allow-same-origin allow-forms allow-popups',
    );
  });

  it('sets allow attribute to clipboard permissions', () => {
    const el = buildIframe(crossOriginCap, defaultParams);
    expect(el.getAttribute('allow')).toBe('clipboard-read; clipboard-write');
  });

  it('throws OBCError(SAME_ORIGIN_CAPABILITY) for same-origin capability', () => {
    const sameOriginCap: Capability = {
      ...crossOriginCap,
      path: 'https://host.example.com/picker',
    };
    let err: unknown;
    try {
      buildIframe(sameOriginCap, defaultParams);
    } catch (e) {
      err = e;
    }
    expect(err instanceof OBCError).toBe(true);
    expect((err as OBCError).code).toBe('SAME_ORIGIN_CAPABILITY');
  });

  it('includes all query params in iframe src URL', () => {
    const el = buildIframe(crossOriginCap, {
      id: 'sess-1',
      clientUrl: 'https://host.example.com',
      type: 'PICK',
      allowedMimeType: 'image/png',
      multiple: true,
    });
    const url = new URL(el.src);
    expect(url.searchParams.get('clientUrl')).toBe('https://host.example.com');
    expect(url.searchParams.get('id')).toBe('sess-1');
    expect(url.searchParams.get('type')).toBe('PICK');
    expect(url.searchParams.get('allowedMimeType')).toBe('image/png');
    expect(url.searchParams.get('multiple')).toBe('true');
  });

  it('omits allowedMimeType param when undefined', () => {
    const el = buildIframe(crossOriginCap, defaultParams);
    const url = new URL(el.src);
    expect(url.searchParams.has('allowedMimeType')).toBe(false);
  });

  it('omits multiple param when undefined', () => {
    const el = buildIframe(crossOriginCap, defaultParams);
    const url = new URL(el.src);
    expect(url.searchParams.has('multiple')).toBe(false);
  });

  it('sets responsive sizing in style', () => {
    const el = buildIframe(crossOriginCap, defaultParams);
    // happy-dom normalizes min() in style.cssText, so check the raw style attribute
    // which preserves the literal values set via setAttribute('style', ...)
    const rawStyle = el.getAttribute('style') ?? '';
    expect(rawStyle).toContain('width:min(90vw,800px)');
    expect(rawStyle).toContain('height:min(85vh,600px)');
    expect(rawStyle).toContain('border-radius:8px');
  });

  // Regression: 0.1.1 hotfix — iframe must override the shadow host's
  // pointer-events:none wrapper. Without these two rules, clicks on the
  // iframe silently fall through to the body behind it in a real browser
  // (happy-dom has no layout engine so it cannot observe the hit-testing
  // failure — we assert the raw style attribute as a grep-level proxy).
  it('sets pointer-events:auto to override shadow host wrapper (0.1.1 regression)', () => {
    const el = buildIframe(crossOriginCap, defaultParams);
    const rawStyle = el.getAttribute('style') ?? '';
    expect(rawStyle).toContain('pointer-events:auto');
  });

  it('sets position:fixed + centered translate for viewport centering (0.1.1 regression)', () => {
    const el = buildIframe(crossOriginCap, defaultParams);
    const rawStyle = el.getAttribute('style') ?? '';
    expect(rawStyle).toContain('position:fixed');
    expect(rawStyle).toContain('top:50%');
    expect(rawStyle).toContain('left:50%');
    expect(rawStyle).toContain('transform:translate(-50%,-50%)');
  });
});
