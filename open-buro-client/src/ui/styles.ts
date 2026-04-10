// src/ui/styles.ts

export interface ShadowHostResult {
  host: HTMLElement;
  root: ShadowRoot;
}

/**
 * Creates a fixed-position light-DOM host element, attaches an open ShadowRoot,
 * injects a CSS reset and spinner keyframes into the shadow, and appends the host
 * to the supplied container.
 *
 * IFR-02 + Pitfall 4: z-index and position MUST live on the light-DOM host element,
 * NOT inside the shadow root. Shadow DOM does not create a stacking context by itself.
 */
export function createShadowHost(container: HTMLElement, zIndex = 9000): ShadowHostResult {
  const host = document.createElement('div');
  // z-index on the LIGHT DOM host — required for correct stacking
  host.style.cssText = `position:fixed;inset:0;z-index:${zIndex};pointer-events:none;`;
  host.dataset.obcHost = '';

  const root = host.attachShadow({ mode: 'open' });

  // UI-09: CSS reset so host-page typography/colors do not leak into the modal
  const style = document.createElement('style');
  style.textContent = `
    :host {
      all: initial;
      display: block;
    }
    *, *::before, *::after {
      box-sizing: border-box;
      margin: 0;
      padding: 0;
    }
    @keyframes obc-spin {
      to { transform: rotate(360deg); }
    }
    .obc-spinner {
      width: 32px;
      height: 32px;
      border: 3px solid rgba(37,99,235,0.2);
      border-top-color: #2563eb;
      border-radius: 50%;
      animation: obc-spin 0.8s linear infinite;
    }
    .obc-spinner-overlay {
      position: absolute;
      inset: 0;
      display: none;
      align-items: center;
      justify-content: center;
      background: rgba(255,255,255,0.6);
      pointer-events: auto;
    }
  `;
  root.appendChild(style);

  container.appendChild(host);
  return { host, root };
}

export interface SpinnerOverlayResult {
  element: HTMLElement;
  show: () => void;
  hide: () => void;
}

/**
 * IFR-09: spinner shows after 150ms delay; hidden on connect.
 * The 150ms delay avoids a flash for fast connections.
 */
export function createSpinnerOverlay(): SpinnerOverlayResult {
  const overlay = document.createElement('div');
  overlay.className = 'obc-spinner-overlay';
  overlay.style.display = 'none';

  const spinner = document.createElement('div');
  spinner.className = 'obc-spinner';
  overlay.appendChild(spinner);

  let timer: ReturnType<typeof setTimeout> | null = null;

  return {
    element: overlay,
    show() {
      timer = setTimeout(() => {
        overlay.style.display = 'flex';
      }, 150);
    },
    hide() {
      if (timer !== null) {
        clearTimeout(timer);
        timer = null;
      }
      overlay.style.display = 'none';
    },
  };
}
