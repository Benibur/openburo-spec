import { describe, expect, it } from 'vitest';
import type { IntentResult } from '../types.js';
import type { BridgeAdapter, ConnectionHandle, ParentMethods } from './bridge-adapter.js';

describe('bridge-adapter types', () => {
  it('ParentMethods.resolve signature accepts IntentResult', () => {
    const methods: ParentMethods = {
      resolve: (_result: IntentResult) => {
        /* intentionally empty */
      },
    };
    methods.resolve({ id: 'test', status: 'cancel', results: [] });
    expect(typeof methods.resolve).toBe('function');
  });

  it('ConnectionHandle has destroy method', () => {
    const handle: ConnectionHandle = {
      destroy: () => {
        /* intentionally empty */
      },
    };
    handle.destroy();
    expect(typeof handle.destroy).toBe('function');
  });

  it('BridgeAdapter can be implemented by a plain object', async () => {
    // This test exists to guarantee the interface surface never silently changes.
    const fakeAdapter: BridgeAdapter = {
      connect: async (_iframe, _origin, _methods, _timeoutMs) => ({
        destroy: () => {
          /* intentionally empty */
        },
      }),
    };
    const handle = await fakeAdapter.connect(
      null as unknown as HTMLIFrameElement,
      'https://cap.example.com',
      { resolve: () => {} },
    );
    expect(handle.destroy).toBeDefined();
  });
});
