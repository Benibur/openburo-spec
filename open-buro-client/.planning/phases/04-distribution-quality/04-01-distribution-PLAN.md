---
phase: 04-distribution-quality
plan: "01"
type: execute
wave: 1
depends_on: []
files_modified:
  - src/client.test.ts
  - docs/capability-authors.md
  - README.md
  - LICENSE
  - package.json
autonomous: true
requirements:
  - PKG-01
  - PKG-02
  - PKG-03
  - PKG-04
  - PKG-05
  - PKG-06
  - PKG-07
  - PKG-08
  - QA-01
  - QA-02
  - QA-03
  - QA-04
  - QA-05
  - QA-06
  - QA-07
  - QA-08
  - QA-09
  - QA-10

must_haves:
  truths:
    - "docs/capability-authors.md exists and documents the Penpal v7 contract for capability iframes"
    - "README.md exists with install, basic usage, and links to integration guide"
    - "LICENSE file exists with MIT license text"
    - "package.json contains description, keywords, repository, homepage, bugs, license fields"
    - "src/client.test.ts contains a test that castIntent with a same-origin capability.path rejects with SAME_ORIGIN_CAPABILITY (QA-09)"
    - "pnpm run ci exits 0"
    - "pnpm pack produces openburo-client-<version>.tgz"
    - "pnpm dlx @arethetypeswrong/cli on the packed tarball reports 'No problems found'"
    - "Every QA-01..10 requirement maps to an existing or new passing test"
  artifacts:
    - path: "docs/capability-authors.md"
      provides: "Capability author integration guide"
      min_lines: 80
    - path: "README.md"
      provides: "Package README with install + usage + links"
      min_lines: 30
    - path: "LICENSE"
      provides: "MIT license"
      min_lines: 20
  key_links:
    - from: "README.md"
      to: "docs/capability-authors.md"
      via: "markdown link"
      pattern: "capability-authors"
    - from: "package.json"
      to: "README.md"
      via: "files field + npm resolver"
      pattern: "README"
    - from: "docs/capability-authors.md"
      to: "Penpal v7 API"
      via: "documentation"
      pattern: "connectToParent.*WindowMessenger|penpal.*7"
---

<objective>
Close out `@openburo/client` v1. Fill the two outstanding gaps — QA-09 same-origin integration test and PKG-08 capability-author integration guide — polish package metadata, add LICENSE + README, and confirm the full distribution pipeline (`pnpm run ci` + `pnpm pack` + `attw` on tarball) runs end-to-end. No actual `npm publish`.
</objective>

<execution_context>
@/home/ben/.claude/get-shit-done/workflows/execute-plan.md
@/home/ben/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/04-distribution-quality/04-CONTEXT.md
@.planning/phases/04-distribution-quality/04-VALIDATION.md
@.planning/PROJECT.md
@.planning/REQUIREMENTS.md
@src/client.ts
@src/client.test.ts
@src/ui/iframe.ts
@package.json
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: QA-09 same-origin capability integration test</name>
  <files>src/client.test.ts</files>

  <read_first>
    - src/client.test.ts (existing tests to extend)
    - src/ui/iframe.ts (confirm SAME_ORIGIN_CAPABILITY is thrown in buildIframe)
    - src/client.ts (confirm castIntent catches and propagates)
  </read_first>

  <action>
    Extend `src/client.test.ts` with a new describe block `same-origin capability rejection`. Add a test that:

    1. Creates an OpenBuroClient with a MockBridge factory
    2. Primes capabilities via mocked fetch to return ONE capability whose `path` matches `location.origin` (in happy-dom, `http://localhost:3000`), e.g. `{ id: 'local', appName: 'Local', action: 'PICK', path: 'http://localhost:3000/same-origin-cap', properties: { mimeTypes: ['*/*'] } }`
    3. Calls `obc.castIntent({ action: 'PICK', args: { clientUrl: 'http://localhost:3000', allowedMimeType: '*/*' } }, callback)`
    4. Asserts that:
       - The callback is invoked with `{ status: 'cancel', id: <string>, results: [] }`
       - `options.onError` was called with an `OBCError` whose `.code === 'SAME_ORIGIN_CAPABILITY'`
       - The returned Promise rejects OR resolves with the cancel callback (whichever matches the current client.ts contract — check the existing same-origin path first)
    5. Cleanup: `obc.destroy()` at the end

    If the current `client.ts` catches the `SAME_ORIGIN_CAPABILITY` error from `buildIframe` but doesn't propagate it to `onError`, fix `client.ts` in this task as well (add to `files_modified`).
  </action>

  <verify>
    <automated>pnpm vitest run src/client.test.ts -t "same-origin"</automated>
  </verify>

  <acceptance_criteria>
    - `src/client.test.ts` contains the literal `same-origin` (case-insensitive OK)
    - `src/client.test.ts` contains `SAME_ORIGIN_CAPABILITY`
    - The new test passes
    - `pnpm typecheck` exits 0
  </acceptance_criteria>
</task>

<task type="auto">
  <name>Task 2: Capability-author integration guide (PKG-08)</name>
  <files>docs/capability-authors.md</files>

  <read_first>
    - src/messaging/penpal-bridge.ts (exact Penpal v7 API the parent uses)
    - src/types.ts (IntentResult, FileResult shapes)
    - src/ui/iframe.ts (query params passed to the iframe URL)
    - .planning/PROJECT.md (project vision)
  </read_first>

  <action>
    Create `docs/capability-authors.md` — an 80-150 line markdown guide for developers building capability iframes for the OpenBuro ecosystem. Structure:

    ```markdown
    # Capability Author Guide — @openburo/client

    This guide explains how to build a capability that can be loaded by an OpenBuroClient host. A "capability" is an iframe that receives an intent (e.g., "PICK an image file"), interacts with the user, and returns a result.

    ## Requirements

    - **Penpal v7** (exact: `penpal@7.0.6` or compatible v7.x). Earlier versions use `connectToChild` which is incompatible.
    - **HTTPS** in production (capabilities served over HTTPS).
    - **Cross-origin**: your capability origin must differ from the host app's origin.

    ## Query parameters your iframe receives

    When OBC loads your capability URL, it appends these query parameters:

    | Param | Source | Example | Required |
    |-------|--------|---------|----------|
    | `clientUrl` | `intent.args.clientUrl` | `https://myapp.example.com` | Yes |
    | `id` | OBC-generated UUID v4 | `a8f3...` | Yes |
    | `type` | `intent.args.type` (optional) | `profile-picture` | No |
    | `allowedMimeType` | `intent.args.allowedMimeType` | `image/png` | No |
    | `multiple` | `intent.args.multiple` | `true` | No |

    ## Penpal handshake

    Your iframe connects to the parent (OBC) using Penpal v7:

    ```typescript
    import { connect, WindowMessenger } from 'penpal';

    const connection = connect({
      messenger: new WindowMessenger({
        remoteWindow: window.parent,
        allowedOrigins: ['*'], // tighten in production to the host app origin
      }),
      methods: {
        // Optional: expose methods the parent can call on you
      },
    });

    const parent = await connection.promise;
    ```

    The parent exposes a `resolve(result: IntentResult)` method that you call when the user finishes or cancels.

    ## Calling resolve()

    When the user has made their selection (or cancelled), call `parent.resolve(result)`:

    ```typescript
    // On user done:
    await parent.resolve({
      id: sessionId, // from the query param above
      status: 'done',
      results: [
        {
          name: 'photo.png',
          mimeType: 'image/png',
          size: 12345,
          sharingUrl: 'https://caps.example.com/files/abc',
          downloadUrl: 'https://caps.example.com/files/abc/download',
        },
      ],
    });

    // On user cancel:
    await parent.resolve({
      id: sessionId,
      status: 'cancel',
      results: [],
    });
    ```

    Always include the `id` you received as a query param — OBC uses it to route the result to the correct callback for concurrent sessions.

    ## IntentResult shape

    ```typescript
    interface IntentResult {
      id: string;
      status: 'done' | 'cancel';
      results: FileResult[];
    }

    interface FileResult {
      name: string;
      mimeType: string;
      size: number;         // bytes
      sharingUrl?: string;
      downloadUrl?: string;
      payload?: unknown;    // optional opaque payload
    }
    ```

    ## Minimal capability skeleton

    ```html
    <!doctype html>
    <html>
      <head><meta charset="utf-8"><title>My Capability</title></head>
      <body>
        <button id="pick">Pick file</button>
        <button id="cancel">Cancel</button>
        <script type="module">
          import { connect, WindowMessenger } from 'https://esm.sh/penpal@7.0.6';

          const params = new URLSearchParams(location.search);
          const sessionId = params.get('id');
          const allowedMimeType = params.get('allowedMimeType') ?? '*/*';

          const connection = connect({
            messenger: new WindowMessenger({
              remoteWindow: window.parent,
              allowedOrigins: ['*'],
            }),
            methods: {},
          });

          const parent = await connection.promise;

          document.getElementById('pick').onclick = async () => {
            await parent.resolve({
              id: sessionId,
              status: 'done',
              results: [{ name: 'demo.png', mimeType: 'image/png', size: 0 }],
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

    - OBC loads your iframe with `sandbox="allow-scripts allow-same-origin allow-forms allow-popups"` — design your capability to work within these constraints.
    - OBC rejects capabilities whose origin matches the host app origin (prevents sandbox escape). You MUST serve from a different origin than the host.
    - Use HTTPS in production. OBC refuses non-HTTPS capability URLs in production mode.
    - Set a strict Content-Security-Policy on your capability HTML to reduce supply-chain risk.

    ## Registering your capability

    A capability is discovered by OBC via the capability registry (typically an OpenBuroServer at `GET /api/v1/capabilities`). Each registered capability entry looks like:

    ```json
    {
      "id": "my-picker",
      "appName": "My Picker",
      "action": "PICK",
      "path": "https://my-picker.example.com/index.html",
      "properties": { "mimeTypes": ["image/png", "image/jpeg"] }
    }
    ```

    See the OpenBuroServer docs for registry API details.

    ## Troubleshooting

    - **`IFRAME_TIMEOUT` fires after 5 min** — your iframe didn't call `parent.resolve()` within the watchdog window. Extend timeout via `options.sessionTimeoutMs` on the host side, or make sure your capability always resolves.
    - **Handshake never completes** — verify you're using Penpal v7 (not v6). v6 `connectToChild`/`connectToParent` are incompatible.
    - **Messages silently ignored** — confirm you include the `id` query param in every `resolve()` call. OBC drops results with unknown session ids.
    ```

    Adjust the ESM CDN URL (`esm.sh/penpal@7.0.6`) if a more recent Penpal v7 release is appropriate at time of writing.
  </action>

  <verify>
    <automated>test -f docs/capability-authors.md && grep -q "Penpal" docs/capability-authors.md && grep -q "WindowMessenger" docs/capability-authors.md && grep -q "resolve(" docs/capability-authors.md</automated>
  </verify>

  <acceptance_criteria>
    - `docs/capability-authors.md` exists
    - File contains literals: `Penpal`, `v7`, `WindowMessenger`, `resolve`, `sessionId`, `SAME_ORIGIN`
    - File is at least 80 lines
  </acceptance_criteria>
</task>

<task type="auto">
  <name>Task 3: Package polish — README, LICENSE, package.json metadata</name>
  <files>README.md, LICENSE, package.json</files>

  <read_first>
    - package.json (current state)
    - .planning/PROJECT.md (for project description)
    - docs/capability-authors.md (for linking)
  </read_first>

  <action>
    **Step 1: Create or overwrite `README.md`** with a minimal but complete package readme:

    ```markdown
    # @openburo/client

    Framework-agnostic browser library for OpenBuro intent brokering: discover capabilities, let the user pick, round-trip the result through a sandboxed iframe.

    ## Install

    \`\`\`bash
    npm install @openburo/client
    # or
    pnpm add @openburo/client
    \`\`\`

    CDN (UMD):

    \`\`\`html
    <script src="https://cdn.example.com/openburo-client/obc.umd.js"></script>
    \`\`\`

    ## Usage

    \`\`\`typescript
    import { OpenBuroClient } from '@openburo/client';

    const obc = new OpenBuroClient({
      capabilitiesUrl: 'https://openburo.example.com/api/v1/capabilities',
      onError: (err) => console.error(err),
    });

    obc.castIntent(
      {
        action: 'PICK',
        args: {
          clientUrl: 'https://myapp.example.com',
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
    \`\`\`

    ## Features

    - Framework-agnostic vanilla TypeScript (no React/Vue/Angular)
    - Tree-shakable ESM + CJS + UMD
    - Zero CSS bleed (Shadow DOM isolation)
    - Full WCAG accessibility baseline for the chooser modal
    - Leak-free `destroy()` via `AbortController`
    - Built on [Penpal v7](https://github.com/Aaronius/penpal) for parent↔iframe messaging

    ## Building capabilities

    See [docs/capability-authors.md](./docs/capability-authors.md) for the full integration guide.

    ## License

    MIT — see [LICENSE](./LICENSE).
    ```

    Use literal backticks (the `\`\`\`` in this template is illustrative — write actual fenced code blocks).

    **Step 2: Create `LICENSE`** with standard MIT license text:

    ```
    MIT License

    Copyright (c) 2026 OpenBuro contributors

    Permission is hereby granted, free of charge, to any person obtaining a copy
    of this software and associated documentation files (the "Software"), to deal
    in the Software without restriction, including without limitation the rights
    to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
    copies of the Software, and to permit persons to whom the Software is
    furnished to do so, subject to the following conditions:

    The above copyright notice and this permission notice shall be included in all
    copies or substantial portions of the Software.

    THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
    IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
    FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
    AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
    LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
    OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
    SOFTWARE.
    ```

    **Step 3: Polish `package.json`** — add/verify these fields (edit surgically, do NOT rewrite the whole file):

    ```json
    {
      "description": "Framework-agnostic browser library for OpenBuro intent brokering — discover capabilities, pick, round-trip via sandboxed iframe.",
      "keywords": [
        "openburo",
        "intent",
        "iframe",
        "file-picker",
        "capability",
        "penpal",
        "postmessage",
        "typescript"
      ],
      "license": "MIT",
      "repository": {
        "type": "git",
        "url": "https://github.com/openburo/openburo-spec.git",
        "directory": "open-buro-client"
      },
      "homepage": "https://github.com/openburo/openburo-spec/tree/master/open-buro-client#readme",
      "bugs": {
        "url": "https://github.com/openburo/openburo-spec/issues"
      },
      "files": [
        "dist",
        "README.md",
        "LICENSE"
      ]
    }
    ```

    Leave all existing fields (name, version, type, exports, scripts, dependencies, devDependencies) untouched.
  </action>

  <verify>
    <automated>test -f README.md && test -f LICENSE && node -e "const p=require('./package.json'); if(!p.description||!p.keywords||!p.license||!p.repository||!p.files) process.exit(1)"</automated>
  </verify>

  <acceptance_criteria>
    - `README.md` exists, contains "OpenBuroClient" or "@openburo/client", contains "install", contains link to `docs/capability-authors.md`
    - `LICENSE` exists, contains "MIT License"
    - `package.json` has `description`, `keywords` (array, length > 0), `license: "MIT"`, `repository`, `files` fields
  </acceptance_criteria>
</task>

<task type="auto">
  <name>Task 4: QA audit — map every QA-01..10 requirement to an existing test</name>
  <files>.planning/phases/04-distribution-quality/04-01-QA-AUDIT.md</files>

  <read_first>
    - .planning/REQUIREMENTS.md (QA-01..10 items)
    - src/capabilities/resolver.test.ts
    - src/intent/cast.test.ts
    - src/intent/id.test.ts
    - src/capabilities/ws-listener.test.ts
    - src/ui/modal.test.ts
    - src/ui/focus-trap.test.ts
    - src/client.test.ts
  </read_first>

  <action>
    Create `.planning/phases/04-distribution-quality/04-01-QA-AUDIT.md` — a markdown table mapping each QA-01..10 requirement to the specific test file(s) that cover it. For each:

    ```markdown
    # QA Audit — Phase 4

    | REQ | Description | Covered by | Status |
    |-----|-------------|------------|--------|
    | QA-01 | Unit tests for resolver mime-matching rules | src/capabilities/resolver.test.ts (9 tests) | ✓ |
    | QA-02 | Unit tests for planCast discriminated union branches | src/intent/cast.test.ts (8 tests) | ✓ |
    | QA-03 | Unit tests for UUID generation + getRandomValues fallback | src/intent/id.test.ts (2 tests, fallback branch exercised) | ✓ |
    | QA-04 | Unit tests for WebSocket backoff + destroyed guard | src/capabilities/ws-listener.test.ts (11 tests) | ✓ |
    | QA-05 | Unit tests for modal focus trap, ESC key, backdrop click | src/ui/focus-trap.test.ts + src/ui/modal.test.ts | ✓ |
    | QA-06 | Integration test — happy-path castIntent with MockBridge | src/client.test.ts (happy-path round-trip test) | ✓ |
    | QA-07 | Integration test — two concurrent sessions route correctly | src/client.test.ts (concurrent sessions test) | ✓ |
    | QA-08 | Integration test — destroy() leaves zero artifacts | src/client.test.ts (destroy leak test) | ✓ |
    | QA-09 | Integration test — same-origin capability rejected | src/client.test.ts (same-origin test, added Task 1) | ✓ |
    | QA-10 | @arethetypeswrong/cli --pack CI gate | package.json scripts.ci + pnpm run ci step | ✓ |

    **Result:** 10/10 QA requirements covered. All tests pass (`pnpm run ci` exits 0).
    ```

    If any row cannot point to a concrete test, STOP and report BLOCKER — do not fabricate coverage.
  </action>

  <verify>
    <automated>test -f .planning/phases/04-distribution-quality/04-01-QA-AUDIT.md && grep -q "QA-10" .planning/phases/04-distribution-quality/04-01-QA-AUDIT.md</automated>
  </verify>

  <acceptance_criteria>
    - `04-01-QA-AUDIT.md` exists
    - Contains rows for all 10 QA requirements (QA-01 through QA-10)
    - Every row has a "✓" marker and a concrete test file reference
  </acceptance_criteria>
</task>

</tasks>

<wave wave="2">

<task type="auto">
  <name>Task 5: Phase gate — pnpm run ci + pnpm pack + attw on tarball</name>
  <files>(verification only)</files>

  <read_first>
    - package.json (scripts.ci)
  </read_first>

  <action>
    Run the full distribution pipeline end-to-end and confirm green:

    1. `pnpm run ci` — must exit 0 (typecheck + lint + test + attw on source)
    2. `pnpm pack` — produces `openburo-client-<version>.tgz` at repo root
    3. `pnpm dlx @arethetypeswrong/cli ./openburo-client-<version>.tgz` — must report "No problems found" and all 4 resolution modes (node10, node16 CJS, node16 ESM, bundler) green
    4. `tar -tzf ./openburo-client-<version>.tgz` — verify the tarball contains:
       - `package/README.md`
       - `package/LICENSE`
       - `package/package.json`
       - `package/dist/` (index.js, index.cjs, index.umd.js, index.d.ts, index.d.cts)
       - No `.planning/`, no `src/`, no `node_modules/`
    5. Clean up the tarball: `rm openburo-client-*.tgz`

    If ANY step fails, fix inline and report in SUMMARY deviations.

    Also verify the capability-author integration guide is NOT included in the tarball (it's for the repo, not the npm package) — OR include it if the `files` field lists `docs/`. Your call; document the decision in SUMMARY.
  </action>

  <verify>
    <automated>pnpm run ci && pnpm pack && pnpm dlx @arethetypeswrong/cli ./openburo-client-*.tgz && rm openburo-client-*.tgz</automated>
  </verify>

  <acceptance_criteria>
    - `pnpm run ci` exits 0
    - `pnpm pack` succeeds
    - `attw` on the tarball reports "No problems found"
    - Tarball contents verified (README, LICENSE, package.json, dist/)
  </acceptance_criteria>
</task>

</wave>

<success_criteria>
- [ ] All 5 tasks executed and committed atomically
- [ ] 18 Phase 4 requirements (PKG-01..08, QA-01..10) all verified
- [ ] `docs/capability-authors.md` created (PKG-08)
- [ ] QA-09 same-origin integration test added and passing
- [ ] `README.md`, `LICENSE`, `package.json` metadata polished
- [ ] QA audit document produced with 10/10 coverage
- [ ] Full CI + pack + attw pipeline green
- [ ] 04-01-SUMMARY.md created
- [ ] STATE.md + ROADMAP.md updated
</success_criteria>
