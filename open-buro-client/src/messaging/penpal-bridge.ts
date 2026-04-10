// MSG-01: This is the ONLY file in the codebase that imports from 'penpal'.
// All other layers must depend on BridgeAdapter instead.
import { connect, type Methods, WindowMessenger } from 'penpal';
import type { BridgeAdapter, ConnectionHandle, ParentMethods } from './bridge-adapter.js';

export class PenpalBridge implements BridgeAdapter {
  async connect(
    iframe: HTMLIFrameElement,
    allowedOrigin: string,
    methods: ParentMethods,
    timeoutMs = 10_000,
  ): Promise<ConnectionHandle> {
    const remoteWindow = iframe.contentWindow;
    if (!remoteWindow) {
      throw new Error(
        'iframe has no contentWindow — ensure it is attached to the DOM before connecting',
      );
    }

    // MSG-03: restrict to specific capability origin per session
    const messenger = new WindowMessenger({
      remoteWindow,
      allowedOrigins: [allowedOrigin],
    });

    // MSG-02: Penpal v7 connect() API — NOT connectToChild
    // Cast methods to Penpal's Methods type (which requires an index signature)
    const connection = connect<Record<string, never>>({
      messenger,
      methods: methods as unknown as Methods,
      timeout: timeoutMs,
    });

    // Wait for handshake to complete
    await connection.promise;

    // MSG-06: destroy tears down the Penpal connection
    return {
      destroy: () => {
        connection.destroy();
      },
    };
  }
}
