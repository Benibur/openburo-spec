import type { IntentResult } from '../types.js';

/** Handle returned from BridgeAdapter.connect() — tears down on destroy(). */
export interface ConnectionHandle {
  destroy(): void;
}

/**
 * Methods exposed by the parent (host page) to the child iframe via Penpal.
 *
 * MSG-04: `resolve` is bound per session — the orchestrator supplies a closure
 * that captures the specific sessionId so IntentResults route to the correct
 * ActiveSession in the session Map. The child iframe calls `parent.resolve(result)`
 * and the closure re-enters orchestrator state with the right key.
 */
export interface ParentMethods {
  resolve(result: IntentResult): void;
}

/**
 * MSG-01: The interface sits between the orchestrator and Penpal. No file
 * other than src/messaging/penpal-bridge.ts is allowed to import 'penpal'.
 *
 * Implementations: PenpalBridge (production) and MockBridge (tests).
 */
export interface BridgeAdapter {
  connect(
    iframe: HTMLIFrameElement,
    allowedOrigin: string,
    methods: ParentMethods,
    timeoutMs?: number,
  ): Promise<ConnectionHandle>;
}
