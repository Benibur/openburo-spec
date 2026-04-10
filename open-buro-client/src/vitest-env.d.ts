// Type augmentation for happy-dom APIs used in test files.
// happy-dom injects `window.happyDOM` at test-time; this shim satisfies tsc.

interface Window {
  happyDOM: {
    setURL(url: string): void;
    settings: {
      navigation: {
        disableChildFrameNavigation: boolean;
      };
      [key: string]: unknown;
    };
    [key: string]: unknown;
  };
}
