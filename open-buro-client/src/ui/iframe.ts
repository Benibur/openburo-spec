// src/ui/iframe.ts
import { OBCError } from '../errors.js';
import type { Capability } from '../types.js';

export interface IframeParams {
  id: string;
  clientUrl: string;
  type: string;
  allowedMimeType?: string;
  multiple?: boolean;
}

/**
 * Creates a sandboxed iframe element for a capability.
 *
 * IFR-08: same-origin guard — throws OBCError(SAME_ORIGIN_CAPABILITY) BEFORE
 * creating the iframe if the capability origin matches the host page origin.
 * Combining allow-scripts + allow-same-origin nullifies sandboxing.
 */
export function buildIframe(capability: Capability, params: IframeParams): HTMLIFrameElement {
  // IFR-08: same-origin guard — throw BEFORE creating the iframe
  const capOrigin = new URL(capability.path).origin;
  if (typeof location !== 'undefined' && capOrigin === location.origin) {
    throw new OBCError(
      'SAME_ORIGIN_CAPABILITY',
      `Capability "${capability.appName}" (${capability.path}) shares origin with host page. ` +
        'Same-origin capabilities are blocked because allow-scripts + allow-same-origin nullifies sandboxing.',
    );
  }

  const iframe = document.createElement('iframe');

  // IFR-03: query params carrying session context to the capability iframe
  const url = new URL(capability.path);
  url.searchParams.set('clientUrl', params.clientUrl);
  url.searchParams.set('id', params.id);
  url.searchParams.set('type', params.type);
  if (params.allowedMimeType !== undefined) {
    url.searchParams.set('allowedMimeType', params.allowedMimeType);
  }
  if (params.multiple !== undefined) {
    url.searchParams.set('multiple', String(params.multiple));
  }
  iframe.src = url.toString();

  // IFR-04: sandbox — allow-scripts + allow-same-origin required for Penpal postMessage
  iframe.setAttribute('sandbox', 'allow-scripts allow-same-origin allow-forms allow-popups');

  // IFR-05: permissions policy for clipboard access
  iframe.setAttribute('allow', 'clipboard-read; clipboard-write');

  // IFR-06: WCAG 2.4.1 / ACT cae760 — accessible name is required on iframe elements
  iframe.title = capability.appName;

  // IFR-07: centered responsive sizing
  // Use setAttribute to preserve min() values; style.cssText normalizes them away in some DOM impls.
  //
  // Two load-bearing rules (DO NOT REMOVE — see CHANGELOG 0.1.1 and 02-RESEARCH.md Pitfall 11):
  //
  //   1. position:fixed + top/left/transform centers the iframe in the viewport
  //      independent of the shadow host wrapper's layout. Without position:fixed,
  //      the iframe renders at (0,0) in the top-left corner because the fixed
  //      shadow-host wrapper does not provide flex/grid centering.
  //
  //   2. pointer-events:auto is MANDATORY. The shadow host (see ui/styles.ts)
  //      uses pointer-events:none on its fullscreen wrapper so clicks on empty
  //      areas fall through to the host page. CSS spec: when an ancestor has
  //      pointer-events:none, ALL its descendants are also excluded from
  //      hit-testing unless they explicitly set pointer-events:auto.
  //      Without this line, every click on the iframe silently passes through
  //      to the element behind it — capability UIs appear completely inert.
  //      Happy-dom unit tests cannot catch this because happy-dom has no
  //      layout/hit-testing engine; only real browsers (or Playwright) reveal it.
  iframe.setAttribute(
    'style',
    'position:fixed;' +
      'top:50%;' +
      'left:50%;' +
      'transform:translate(-50%,-50%);' +
      'display:block;' +
      'width:min(90vw,800px);' +
      'height:min(85vh,600px);' +
      'border:none;' +
      'border-radius:8px;' +
      'box-shadow:0 4px 32px rgba(0,0,0,0.24);' +
      'pointer-events:auto;',
  );

  return iframe;
}
