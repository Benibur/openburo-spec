// @vitest-environment happy-dom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createShadowHost, createSpinnerOverlay } from './styles.js';

describe('createShadowHost', () => {
  let container: HTMLDivElement;

  beforeEach(() => {
    container = document.createElement('div');
    document.body.appendChild(container);
  });

  afterEach(() => {
    container.remove();
  });

  it('appends host to the container', () => {
    const { host } = createShadowHost(container);
    expect(container.contains(host)).toBe(true);
  });

  it('sets host position to fixed', () => {
    const { host } = createShadowHost(container);
    expect(host.style.position).toBe('fixed');
  });

  it('sets default z-index to 9000 on the light DOM host', () => {
    const { host } = createShadowHost(container);
    expect(host.style.zIndex).toBe('9000');
  });

  it('sets custom z-index on the light DOM host', () => {
    const { host } = createShadowHost(container, 12345);
    expect(host.style.zIndex).toBe('12345');
  });

  it('returns a ShadowRoot attached to host', () => {
    const { host, root } = createShadowHost(container);
    expect(root.host).toBe(host);
  });

  it('shadow root contains a style tag with :host { all: initial', () => {
    const { root } = createShadowHost(container);
    const style = root.querySelector('style');
    expect(style).not.toBeNull();
    expect(style?.textContent).toContain(':host {');
    expect(style?.textContent).toContain('all: initial');
  });

  it('shadow root style contains @keyframes obc-spin', () => {
    const { root } = createShadowHost(container);
    const style = root.querySelector('style');
    expect(style?.textContent).toContain('@keyframes obc-spin');
  });

  it('shadow root style contains #2563eb accent color', () => {
    const { root } = createShadowHost(container);
    const style = root.querySelector('style');
    expect(style?.textContent).toContain('#2563eb');
  });
});

describe('createSpinnerOverlay', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('element display is none initially', () => {
    const { element } = createSpinnerOverlay();
    expect(element.style.display).toBe('none');
  });

  it('show() does not immediately show the spinner', () => {
    const { element, show } = createSpinnerOverlay();
    show();
    expect(element.style.display).toBe('none');
  });

  it('show() displays after 150ms via fake timer', () => {
    const { element, show } = createSpinnerOverlay();
    show();
    vi.advanceTimersByTime(150);
    expect(element.style.display).toBe('flex');
  });

  it('hide() after show but before 150ms keeps display none (never shows)', () => {
    const { element, show, hide } = createSpinnerOverlay();
    show();
    vi.advanceTimersByTime(100);
    hide();
    vi.advanceTimersByTime(100);
    expect(element.style.display).toBe('none');
  });

  it('hide() after display=flex sets it back to none', () => {
    const { element, show, hide } = createSpinnerOverlay();
    show();
    vi.advanceTimersByTime(150);
    expect(element.style.display).toBe('flex');
    hide();
    expect(element.style.display).toBe('none');
  });
});
