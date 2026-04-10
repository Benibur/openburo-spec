// src/client.ts — OpenBuroClient orchestrator
//
// LAYER BOUNDARY NOTE: This is the ONLY file in the codebase permitted to
// import across layers (capabilities, intent, lifecycle, ui, messaging).
// No other src/ file may import from more than one of those directories.

import { fetchCapabilities, type LoaderResult } from './capabilities/loader.js';
import { resolve as resolveCapabilities } from './capabilities/resolver.js';
import { deriveWsUrl, WsListener } from './capabilities/ws-listener.js';
import { OBCError } from './errors.js';
import { planCast } from './intent/cast.js';
import { generateSessionId } from './intent/id.js';
import type { ActiveSession } from './intent/session.js';
import { type AbortContext, createAbortContext } from './lifecycle/abort-context.js';
import type { BridgeAdapter, ConnectionHandle } from './messaging/bridge-adapter.js';
import { PenpalBridge } from './messaging/penpal-bridge.js';
import type {
  Capability,
  IntentCallback,
  IntentRequest,
  IntentResult,
  OBCOptions,
} from './types.js';
import { buildIframe } from './ui/iframe.js';
import { buildModal } from './ui/modal.js';
import { createShadowHost } from './ui/styles.js';

export interface OpenBuroClientOptions extends OBCOptions {
  /**
   * @internal — Injected bridge adapter for tests; defaults to PenpalBridge.
   * Not part of the published v1 public API surface.
   */
  bridgeFactory?: BridgeAdapter;
  /** Session watchdog timeout in ms (default: 5 * 60 * 1000 = 5 minutes). */
  sessionTimeoutMs?: number;
}

/**
 * @public
 * OpenBuroClient — the intent broker facade. Composes every Phase 2 layer
 * (capabilities, intent, lifecycle, UI, messaging) behind a stable,
 * leak-free API.
 *
 * Typical usage:
 *   const obc = new OpenBuroClient({ capabilitiesUrl: 'https://...' });
 *   obc.castIntent({ action: 'PICK', args: {} }, (result) => { ... });
 *   // Later: obc.destroy();
 */
export class OpenBuroClient {
  private readonly options: OpenBuroClientOptions;
  private readonly bridgeFactory: BridgeAdapter;
  private readonly sessionTimeoutMs: number;
  private readonly abortContext: AbortContext;
  private readonly sessions: Map<string, ActiveSession> = new Map();

  private capabilities: Capability[] = [];
  private inflightFetch: Promise<LoaderResult> | null = null;
  private wsListener: WsListener | null = null;
  private destroyed = false;

  constructor(options: OpenBuroClientOptions) {
    // ORCH-01: Synchronous URL validation — no async side effects here
    this.validateUrl(
      options.capabilitiesUrl,
      'capabilitiesUrl',
      'CAPABILITIES_FETCH_FAILED',
      false,
    );
    if (options.wsUrl !== undefined) {
      this.validateUrl(options.wsUrl, 'wsUrl', 'WS_CONNECTION_FAILED', true);
    }

    this.options = options;
    this.bridgeFactory = options.bridgeFactory ?? new PenpalBridge();
    this.sessionTimeoutMs = options.sessionTimeoutMs ?? 5 * 60 * 1000;
    this.abortContext = createAbortContext();

    // ORCH-02: NO async side effects in constructor.
    // No fetch. No DOM mutations. No WebSocket. No event listeners beyond AbortContext.
    // First HTTP request is deferred to first castIntent() or explicit refreshCapabilities().
  }

  // ---------------------------------------------------------------------------
  // Private: URL validation
  // ---------------------------------------------------------------------------

  private validateUrl(
    url: string,
    _optionName: 'capabilitiesUrl' | 'wsUrl',
    errorCode: 'CAPABILITIES_FETCH_FAILED' | 'WS_CONNECTION_FAILED',
    wsMode: boolean,
  ): void {
    const secureScheme = wsMode ? 'wss://' : 'https://';
    const localPrefix = wsMode ? 'ws://localhost' : 'http://localhost';
    if (!url.startsWith(secureScheme) && !url.startsWith(localPrefix)) {
      throw new OBCError(
        errorCode,
        `Mixed Content: ${_optionName} must be ${secureScheme} in production (got ${url})`,
      );
    }
  }

  // ---------------------------------------------------------------------------
  // Public: capability accessors
  // ---------------------------------------------------------------------------

  /** CAP-06 / ORCH-06: synchronous in-memory read. Returns [] when destroyed or before first fetch. */
  getCapabilities(): Capability[] {
    if (this.destroyed) return [];
    return [...this.capabilities];
  }

  /**
   * CAP-07: Force a fresh capability fetch.
   * Single-flight guards concurrent callers — two simultaneous calls share one Promise.
   * After destroy(), returns a rejected Promise with OBCError('DESTROYED').
   */
  async refreshCapabilities(): Promise<Capability[]> {
    if (this.destroyed) {
      return Promise.reject(new OBCError('DESTROYED', 'OpenBuroClient has been destroyed'));
    }
    return this.ensureCapabilities(/* forceReload */ true);
  }

  // ---------------------------------------------------------------------------
  // Private: lazy fetch + single-flight cache
  // ---------------------------------------------------------------------------

  /**
   * Lazy first-fetch with single-flight cache.
   * - If capabilities already loaded AND not forcing, returns them immediately.
   * - If a fetch is in-flight, joins the same Promise (single-flight, ORCH-02).
   * - On success, stores capabilities and (first time only) may start WsListener.
   * - On failure, clears the in-flight cache so the next call retries fresh.
   */
  private async ensureCapabilities(forceReload = false): Promise<Capability[]> {
    if (!forceReload && this.capabilities.length > 0) {
      return this.capabilities;
    }

    // Single-flight: join the in-flight Promise if one exists
    if (this.inflightFetch !== null) {
      const result = await this.inflightFetch;
      return result.capabilities;
    }

    const promise = fetchCapabilities(this.options.capabilitiesUrl, this.abortContext.signal);
    this.inflightFetch = promise;

    try {
      const result = await promise;
      this.capabilities = result.capabilities;
      this.maybeStartWsListener(result);
      return result.capabilities;
    } catch (err) {
      const obcErr =
        err instanceof OBCError
          ? err
          : new OBCError('CAPABILITIES_FETCH_FAILED', 'Unknown fetch error', err);
      this.options.onError?.(obcErr);
      throw obcErr;
    } finally {
      this.inflightFetch = null;
    }
  }

  /**
   * Start WsListener if conditions are met (ORCH-02):
   * - First time only (not already started)
   * - liveUpdates !== false
   * - Server responded with X-OpenBuro-Server: true
   * - Not destroyed
   */
  private maybeStartWsListener(result: LoaderResult): void {
    if (this.wsListener !== null) return; // already started
    if (this.options.liveUpdates === false) return;
    if (!result.isOpenBuroServer) return;
    if (this.destroyed) return;

    const wsUrl = this.options.wsUrl ?? deriveWsUrl(this.options.capabilitiesUrl);
    const listener = new WsListener({
      wsUrl,
      onUpdate: () => {
        void this.refreshCapabilities().then(
          (caps) => this.options.onCapabilitiesUpdated?.(caps),
          () => {
            // onError already fired inside refreshCapabilities
          },
        );
      },
      onError: (err) => this.options.onError?.(err),
    });
    listener.start();
    this.wsListener = listener;
    this.abortContext.addCleanup(() => {
      listener.stop();
    });
  }

  // ---------------------------------------------------------------------------
  // Public: castIntent — the core orchestration flow
  // ---------------------------------------------------------------------------

  /**
   * ORCH-03: Cast an intent to a matching capability.
   * Throws (as a rejected Promise) with OBCError('DESTROYED') if called after destroy().
   *
   * Flow:
   * 1. Check destroyed guard
   * 2. Lazy capability fetch (single-flight)
   * 3. resolveCapabilities → planCast → branch on no-match / direct / select
   * 4. Open iframe session (in one synchronous block — see pitfall note below)
   */
  async castIntent(intent: IntentRequest, callback: IntentCallback): Promise<IntentResult> {
    // ORCH-06: sync throw (as rejected Promise since this is async)
    if (this.destroyed) {
      throw new OBCError('DESTROYED', 'OpenBuroClient has been destroyed');
    }

    // Lazy capability fetch (single-flight)
    const capabilities = await this.ensureCapabilities();

    // Re-check destroyed flag after the async fetch completes
    if (this.destroyed) {
      throw new OBCError('DESTROYED', 'OpenBuroClient has been destroyed');
    }

    // planCast branching on discriminated union
    const matches = resolveCapabilities(capabilities, intent);
    const plan = planCast(matches);

    if (plan.kind === 'no-match') {
      const id = generateSessionId();
      this.options.onError?.(
        new OBCError('NO_MATCHING_CAPABILITY', `No capability for action "${intent.action}"`),
      );
      const cancelResult: IntentResult = { id, status: 'cancel', results: [] };
      callback(cancelResult);
      return cancelResult;
    }

    if (plan.kind === 'direct') {
      return this.openIframeSession(plan.capability, intent, callback);
    }

    // plan.kind === 'select' → chooser modal
    return this.openChooserModal(plan.capabilities, intent, callback);
  }

  // ---------------------------------------------------------------------------
  // Private: chooser modal (select path)
  // ---------------------------------------------------------------------------

  private openChooserModal(
    capabilities: Capability[],
    intent: IntentRequest,
    callback: IntentCallback,
  ): Promise<IntentResult> {
    const container = this.options.container ?? document.body;
    const { host, root } = createShadowHost(container);

    return new Promise<IntentResult>((resolvePromise, rejectPromise) => {
      let settled = false;

      const modal = buildModal(
        capabilities,
        {
          onSelect: (capability) => {
            if (settled) return;
            settled = true;
            modal.destroy();
            host.remove();
            // Re-enter the iframe opening path
            this.openIframeSession(capability, intent, callback).then(
              resolvePromise,
              rejectPromise,
            );
          },
          onCancel: () => {
            if (settled) return;
            settled = true;
            modal.destroy();
            host.remove();
            const cancelResult: IntentResult = {
              id: generateSessionId(),
              status: 'cancel',
              results: [],
            };
            callback(cancelResult);
            resolvePromise(cancelResult);
          },
        },
        root,
      );
      root.appendChild(modal.element);

      // Register cleanup so destroy() during a live modal tears it down
      this.abortContext.addCleanup(() => {
        if (settled) return;
        settled = true;
        modal.destroy();
        host.remove();
        const cancelResult: IntentResult = {
          id: generateSessionId(),
          status: 'cancel',
          results: [],
        };
        try {
          callback(cancelResult);
        } catch {
          /* swallow errors in callback during teardown */
        }
        resolvePromise(cancelResult);
      });
    });
  }

  // ---------------------------------------------------------------------------
  // Private: iframe session (direct path + after modal selection)
  // ---------------------------------------------------------------------------

  private async openIframeSession(
    capability: Capability,
    intent: IntentRequest,
    callback: IntentCallback,
  ): Promise<IntentResult> {
    const id = generateSessionId();

    // Synchronicity pitfall (Phase 3 research): iframe → shadow append → bridge.connect
    // MUST run in ONE synchronous block before any await. If we await between
    // appendChild and connect(), the child frame can load and postMessage before
    // Penpal is listening — the handshake silently fails.
    let iframe: HTMLIFrameElement;
    let shadowHost: HTMLElement;
    let connectPromise: Promise<ConnectionHandle>;

    try {
      // 1. Build iframe (may throw SAME_ORIGIN_CAPABILITY synchronously — IFR-08)
      iframe = buildIframe(capability, {
        id,
        clientUrl: typeof location !== 'undefined' ? location.origin : '',
        type: intent.action,
        allowedMimeType: intent.args.allowedMimeType,
        multiple: intent.args.multiple,
      });

      // 2. Create shadow host and append iframe — all synchronous
      const container = this.options.container ?? document.body;
      const shadow = createShadowHost(container);
      shadowHost = shadow.host;
      shadow.root.appendChild(iframe);

      // 3. Kick off bridge connect (returns a Promise — do NOT await yet)
      // This must be in the same synchronous block as steps 1 and 2.
      const allowedOrigin = new URL(capability.path).origin;
      connectPromise = this.bridgeFactory.connect(iframe, allowedOrigin, {
        resolve: (result) => this.handleSessionResolve(result),
      });
    } catch (err) {
      // Synchronous throw (e.g. SAME_ORIGIN_CAPABILITY) — fire cancel callback + onError
      if (err instanceof OBCError) {
        this.options.onError?.(err);
      }
      const cancelResult: IntentResult = { id, status: 'cancel', results: [] };
      callback(cancelResult);
      return cancelResult;
    }

    // Now we can await — the sync block above is complete
    let connectionHandle: ConnectionHandle;
    try {
      connectionHandle = await connectPromise;
    } catch (err) {
      // Bridge connection failed — tear down DOM and fire cancel
      shadowHost.remove();
      if (err instanceof OBCError) {
        this.options.onError?.(err);
      }
      const cancelResult: IntentResult = { id, status: 'cancel', results: [] };
      callback(cancelResult);
      return cancelResult;
    }

    // Check destroyed AFTER the async await — destroy() may have been called while connecting
    if (this.destroyed) {
      try {
        connectionHandle.destroy();
      } catch {
        /* ignore */
      }
      shadowHost.remove();
      const cancelResult: IntentResult = { id, status: 'cancel', results: [] };
      callback(cancelResult);
      return cancelResult;
    }

    return new Promise<IntentResult>((resolvePromise, rejectPromise) => {
      // INT-08: session watchdog timer
      const timeoutHandle = setTimeout(() => {
        this.teardownSession(id);
        this.options.onError?.(new OBCError('IFRAME_TIMEOUT', `Session ${id} timed out`));
        const cancelResult: IntentResult = { id, status: 'cancel', results: [] };
        callback(cancelResult);
        resolvePromise(cancelResult);
      }, this.sessionTimeoutMs);

      const session: ActiveSession = {
        id,
        capability,
        iframe,
        shadowHost,
        connectionHandle,
        timeoutHandle,
        resolve: resolvePromise,
        reject: rejectPromise,
        callback,
      };
      this.sessions.set(id, session);

      // Register cleanup so destroy() cancels this session
      this.abortContext.addCleanup(() => {
        const s = this.sessions.get(id);
        if (s === undefined) return;
        clearTimeout(s.timeoutHandle);
        try {
          s.connectionHandle.destroy();
        } catch {
          /* ignore bridge destroy errors during teardown */
        }
        s.shadowHost.remove();
        this.sessions.delete(id);
        const cancelResult: IntentResult = { id, status: 'cancel', results: [] };
        try {
          s.callback(cancelResult);
        } catch {
          /* swallow errors in callback during teardown */
        }
        resolvePromise(cancelResult);
      });
    });
  }

  // ---------------------------------------------------------------------------
  // Private: session resolution + teardown helpers
  // ---------------------------------------------------------------------------

  private handleSessionResolve(result: IntentResult): void {
    // INT-09: silently ignore unknown/stale session ids
    const session = this.sessions.get(result.id);
    if (session === undefined) return;
    this.teardownSession(result.id);
    try {
      session.callback(result);
    } finally {
      session.resolve(result);
    }
  }

  private teardownSession(id: string): void {
    const session = this.sessions.get(id);
    if (session === undefined) return;
    clearTimeout(session.timeoutHandle);
    try {
      session.connectionHandle.destroy();
    } catch {
      /* ignore bridge destroy errors */
    }
    session.shadowHost.remove();
    this.sessions.delete(id);
  }

  // ---------------------------------------------------------------------------
  // Public: destroy — ORCH-04 / ORCH-05 / ORCH-06
  // ---------------------------------------------------------------------------

  /**
   * ORCH-04/05: Tear down everything via AbortContext. Idempotent.
   *
   * Cleanup order (LIFO via AbortContext stack):
   * - Per-session: clear watchdog, bridge.destroy(), shadowHost.remove(), callback(cancel)
   * - Per-modal-chooser: modal.destroy(), host.remove(), callback(cancel)
   * - WsListener.stop()
   * - fetch AbortSignal fired
   */
  destroy(): void {
    if (this.destroyed) return; // ORCH-06: idempotent no-op
    this.destroyed = true;

    // AbortContext drains its LIFO cleanup stack — cancels sessions, modals, WS, fetch
    this.abortContext.abort();

    // Defensive clear in case any cleanup threw before finishing
    this.sessions.clear();
    this.wsListener = null;
    this.capabilities = [];
    this.inflightFetch = null;
  }

  // ---------------------------------------------------------------------------
  // Internal: test-only introspection (not part of public API)
  // ---------------------------------------------------------------------------

  /** @internal — test-only. Returns active session IDs for assertions. */
  __debugGetActiveSessionIds(): string[] {
    return Array.from(this.sessions.keys());
  }
}
