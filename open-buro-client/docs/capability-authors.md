# Capability Author Guide — @openburo/client

This guide explains how to build a capability iframe that can be loaded by an
`OpenBuroClient` host. A "capability" is an iframe that receives an intent (e.g.,
"PICK an image file"), interacts with the user, and returns a result to the host
application via a PostMessage round-trip managed by Penpal v7.

## Requirements

- **Penpal v7** (exact: `penpal@7.0.6` or compatible v7.x). Earlier versions use
  `connectToChild` / `connectToParent` which are incompatible with the v7 API.
- **HTTPS** in production — capabilities must be served over HTTPS.
- **Cross-origin** — your capability origin MUST differ from the host application
  origin. OBC rejects same-origin capabilities to prevent sandbox escape.

## Query parameters your iframe receives

When OBC loads your capability URL it appends these query parameters automatically:

| Param            | Source                        | Example                        | Required |
|------------------|-------------------------------|--------------------------------|----------|
| `clientUrl`      | `intent.args.clientUrl`       | `https://myapp.example.com`    | Yes      |
| `id`             | OBC-generated UUID v4         | `a8f3...`                      | Yes      |
| `type`           | `intent.action`               | `PICK`                         | Yes      |
| `allowedMimeType`| `intent.args.allowedMimeType` | `image/png`                    | No       |
| `multiple`       | `intent.args.multiple`        | `true`                         | No       |

Parse them from `location.search` in your capability:

```js
const params = new URLSearchParams(location.search);
const sessionId = params.get('id');          // always present
const clientUrl = params.get('clientUrl');   // always present
const allowedMimeType = params.get('allowedMimeType') ?? '*/*';
const multiple = params.get('multiple') === 'true';
```

## Penpal handshake

Connect to the parent (OBC host) using Penpal v7's `connect` + `WindowMessenger`:

```typescript
import { connect, WindowMessenger } from 'penpal';

const connection = connect({
  messenger: new WindowMessenger({
    remoteWindow: window.parent,
    // Tighten allowedOrigins to the host origin in production.
    // clientUrl query param gives you the expected host origin.
    allowedOrigins: [clientUrl ?? '*'],
  }),
  methods: {
    // Optional: expose methods the parent can call on your iframe.
  },
});

const parent = await connection.promise;
```

The parent exposes a `resolve(result: IntentResult)` method. Call it exactly once
when the user finishes or cancels.

## Calling resolve()

Pass an `IntentResult` to `parent.resolve()`:

```typescript
// User completed the intent (e.g., picked a file):
await parent.resolve({
  id: sessionId,      // REQUIRED — must match the 'id' query param
  status: 'done',
  results: [
    {
      name: 'photo.png',
      type: 'image/png',
      size: 12345,
      url: 'https://caps.example.com/files/abc',
    },
  ],
});

// User dismissed / cancelled:
await parent.resolve({
  id: sessionId,
  status: 'cancel',
  results: [],
});
```

Always include the `id` you received as a query param. OBC uses it to route the
result to the correct callback when multiple concurrent sessions are running.

## IntentResult and FileResult shapes

```typescript
interface IntentResult {
  id: string;               // UUID v4 — must match the 'id' query param
  status: 'done' | 'cancel';
  results: FileResult[];
}

interface FileResult {
  name: string;   // filename
  type: string;   // MIME type, e.g. 'image/png'
  size: number;   // bytes
  url: string;    // data URI or shareable URL for the file
}
```

## Minimal capability skeleton

A complete minimal capability implementation in plain HTML + JavaScript:

```html
<!doctype html>
<html>
  <head>
    <meta charset="utf-8">
    <title>My Capability</title>
    <meta name="viewport" content="width=device-width, initial-scale=1">
  </head>
  <body>
    <h1>My File Picker</h1>
    <button id="pick">Pick a demo file</button>
    <button id="cancel">Cancel</button>

    <script type="module">
      import { connect, WindowMessenger } from 'https://esm.sh/penpal@7.0.6';

      const params = new URLSearchParams(location.search);
      const sessionId = params.get('id');
      const clientUrl = params.get('clientUrl') ?? '*';
      const allowedMimeType = params.get('allowedMimeType') ?? '*/*';

      const connection = connect({
        messenger: new WindowMessenger({
          remoteWindow: window.parent,
          allowedOrigins: [clientUrl],
        }),
        methods: {},
      });

      const parent = await connection.promise;

      document.getElementById('pick').onclick = async () => {
        await parent.resolve({
          id: sessionId,
          status: 'done',
          results: [
            { name: 'demo.png', type: 'image/png', size: 0, url: '' },
          ],
        });
      };

      document.getElementById('cancel').onclick = async () => {
        await parent.resolve({
          id: sessionId,
          status: 'cancel',
          results: [],
        });
      };
    </script>
  </body>
</html>
```

## Security expectations

- OBC loads your iframe with `sandbox="allow-scripts allow-same-origin allow-forms allow-popups"`.
  Design your capability to work within these constraints.
- **SAME_ORIGIN protection** — OBC rejects capabilities whose origin matches the
  host app origin (`SAME_ORIGIN_CAPABILITY` error). You MUST serve your capability
  from a different origin than the host application.
- Use HTTPS in production. Mixed-content capabilities will be blocked.
- Set a strict `Content-Security-Policy` on your capability HTML to reduce
  supply-chain risk.
- Validate the `clientUrl` param before using it as `allowedOrigins` — in
  production use an allowlist rather than passing the raw query param value.

## Registering your capability

A capability is discovered by OBC via the capability registry (typically an
OpenBuroServer at `GET /api/v1/capabilities`). Each entry must match this shape:

```json
{
  "id": "my-picker",
  "appName": "My Picker",
  "action": "PICK",
  "path": "https://my-picker.example.com/index.html",
  "properties": { "mimeTypes": ["image/png", "image/jpeg"] }
}
```

Use `"*/*"` in `mimeTypes` to match any MIME type.
See the OpenBuroServer docs for registry API details.

## Troubleshooting

- **`IFRAME_TIMEOUT` fires** — your iframe did not call `parent.resolve()` within
  the watchdog window (default: 5 minutes). Extend the timeout via
  `options.sessionTimeoutMs` on the host side, or ensure your capability always
  resolves or cancels.
- **Handshake never completes** — verify you are using Penpal v7 (`penpal@7.x`).
  The v6 `connectToParent` API is incompatible and will silently fail.
- **Messages silently ignored** — confirm you include the `id` query param in
  every `resolve()` call. OBC drops results with unknown session IDs.
- **`SAME_ORIGIN_CAPABILITY` error** — your capability URL origin matches the host
  app origin. Move your capability to a different domain or subdomain.
