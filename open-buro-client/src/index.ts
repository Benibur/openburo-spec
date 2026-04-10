// src/index.ts — public API barrel
//
// Phase 1: errors + shared types + session id
// Phase 2: capabilities + intent + ui + messaging layers (re-exported for Phase 3 orchestrator consumption)
//
// Phase 3 will add OpenBuroClient as the default entry point.

// ---- Phase 1 exports (do not modify) ----

export type { OBCErrorCode } from './errors';
export { OBCError } from './errors';

export { generateSessionId } from './intent/id';

export type {
  Capability,
  CastPlan,
  FileResult,
  IntentCallback,
  IntentRequest,
  IntentResult,
  OBCOptions,
} from './types';

// ---- Phase 2: Capabilities layer ----

export type { LoaderResult } from './capabilities/loader';
export { fetchCapabilities } from './capabilities/loader';
export { resolve } from './capabilities/resolver';
export type { WsListenerOptions } from './capabilities/ws-listener';
export { deriveWsUrl, WsListener } from './capabilities/ws-listener';

// ---- Phase 2: Lifecycle layer ----

export type { AbortContext } from './lifecycle/abort-context';
export { createAbortContext } from './lifecycle/abort-context';

// ---- Phase 2: Intent layer ----

export { planCast } from './intent/cast';
export type { ActiveSession } from './intent/session';

// ---- Phase 2: UI layer ----

export { trapFocus } from './ui/focus-trap';
export type { IframeParams } from './ui/iframe';
export { buildIframe } from './ui/iframe';
export type { ModalCallbacks, ModalResult } from './ui/modal';
export { buildModal } from './ui/modal';
export { lockBodyScroll } from './ui/scroll-lock';
export type { ShadowHostResult } from './ui/styles';
export { createShadowHost, createSpinnerOverlay } from './ui/styles';

// ---- Phase 2: Messaging layer ----

export type {
  BridgeAdapter,
  ConnectionHandle,
  ParentMethods,
} from './messaging/bridge-adapter';
export { MockBridge } from './messaging/mock-bridge';
export { PenpalBridge } from './messaging/penpal-bridge';
