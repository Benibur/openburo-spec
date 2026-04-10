// Type augmentation for happy-dom APIs used in test files.
// happy-dom injects `window.happyDOM` at test-time; this shim satisfies tsc.

interface Window {
  happyDOM: {
    setURL(url: string): void;
    [key: string]: unknown;
  };
}
