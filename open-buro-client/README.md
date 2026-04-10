# @openburo/client

Framework-agnostic browser library for OpenBuro intent brokering: discover capabilities, let the user pick, round-trip the result through a sandboxed iframe.

## Install

```bash
npm install @openburo/client
# or
pnpm add @openburo/client
```

CDN (UMD):

```html
<script src="https://cdn.example.com/openburo-client/obc.umd.js"></script>
```

## Usage

```typescript
import { OpenBuroClient } from '@openburo/client';

const obc = new OpenBuroClient({
  capabilitiesUrl: 'https://openburo.example.com/api/v1/capabilities',
  onError: (err) => console.error(err),
});

obc.castIntent(
  {
    action: 'PICK',
    args: {
      allowedMimeType: 'image/png',
    },
  },
  (result) => {
    if (result.status === 'done') {
      console.log('files:', result.results);
    }
  },
);

// On teardown:
obc.destroy();
```

## Features

- Framework-agnostic vanilla TypeScript (no React/Vue/Angular)
- Tree-shakable ESM + CJS + UMD
- Zero CSS bleed (Shadow DOM isolation)
- Full WCAG accessibility baseline for the chooser modal
- Leak-free `destroy()` via `AbortController`
- Built on [Penpal v7](https://github.com/Aaronius/penpal) for parent/iframe messaging

## Building capabilities

See [docs/capability-authors.md](./docs/capability-authors.md) for the full integration guide on how to build a capability iframe that works with `@openburo/client`.

## License

MIT — see [LICENSE](./LICENSE).
