---
phase: 03-websocket-hub
plan: 03
type: execute
wave: 3
depends_on:
  - 03-01
  - 03-02
files_modified:
  - internal/wshub/hub_test.go
autonomous: true
gap_closure: false
requirements:
  - WS-10
requirements_addressed:
  - WS-10

must_haves:
  truths:
    - "TestHub_Logging_DropIsWarn captures slog output across a slow-consumer kick and asserts: level=WARN, message 'wshub: subscriber dropped (slow consumer)', buffer_size field present, and NO 'peer_addr' or 'user_agent' PII fields"
    - "TestHub_Logging_CloseIsInfo captures slog output across Hub.Close and asserts: level=INFO, message 'wshub: closing hub', subscribers count field present, exactly one line"
    - "TestHub_Logging_NoPII captures slog output across a full connect → publish → close flow and asserts the captured buffer contains no username, no IP address, no Authorization header, no r.RemoteAddr substring"
    - "Architectural gate verification: `go list -deps ./internal/wshub` produces no matches for registry or httpapi"
    - "No-slog.Default gate: `grep -rE 'slog\\.Default' internal/wshub/*.go | grep -v _test.go` is empty"
    - "No-time.Sleep gate: `grep -n 'time\\.Sleep' internal/wshub/*_test.go` is empty"
    - "Phase 3 acceptance: WS-10's goroutine-leak test (TestSubscribe_NoGoroutineLeak from Plan 03-01) is green under -race and is the last test to run in the phase verification sweep"
  artifacts:
    - path: "internal/wshub/hub_test.go"
      provides: "Three logging-capture tests appended: TestHub_Logging_DropIsWarn, TestHub_Logging_CloseIsInfo, TestHub_Logging_NoPII"
      contains: "TestHub_Logging_DropIsWarn"
  key_links:
    - from: "internal/wshub/hub_test.go TestHub_Logging_*"
      to: "slog.NewTextHandler(&bytes.Buffer{})"
      via: "captured log handler asserted via require.Contains on buffer.String()"
      pattern: "slog\\.NewTextHandler\\(&buf"
    - from: "Phase 3 verification gate script"
      to: "go list -deps ./internal/wshub"
      via: "grep -E 'registry|httpapi' must be empty"
      pattern: "go list -deps"
---

<objective>
Close Phase 3 by landing the logging contract tests and running the full architectural gate sweep. This plan adds three log-capture tests to `hub_test.go` (RED → GREEN for logging observability) and codifies the three architectural gates (no registry/httpapi imports, no slog.Default, no time.Sleep) as the phase's verification script.

This plan does NOT re-implement WS-10's goroutine-leak test — `TestSubscribe_NoGoroutineLeak` already lives in `subscribe_test.go` from Plan 03-01 and has been running green on every commit since. What this plan DOES do is make the leak test's green status the final gate of the phase: the plan's verification section runs the full wshub suite + the three gate checks in a single command, and the phase is not complete until all of them exit 0.

The three logging tests close a subtler gap: they assert the OBSERVABLE log contract (the exact level, message text, and fields) that the production code promises in CONTEXT.md §"Drop-subscriber logging". Without these tests, a future refactor could change `Warn` to `Info` or drop the `buffer_size` field and nothing would fail. With them, the log format is frozen by assertion.

Purpose: Lock in the logging observability contract and ship the phase's gate script. After this plan, Phase 3 is feature-complete and ready for `/gsd:verify-work`.

Output: three new test functions appended to `internal/wshub/hub_test.go` and a verification block that runs the full gate sweep.
</objective>

<execution_context>
@/home/ben/.claude/get-shit-done/workflows/execute-plan.md
@/home/ben/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/ROADMAP.md
@.planning/REQUIREMENTS.md
@.planning/phases/03-websocket-hub/03-CONTEXT.md
@.planning/phases/03-websocket-hub/03-RESEARCH.md
@.planning/phases/03-websocket-hub/03-VALIDATION.md
@.planning/phases/03-websocket-hub/03-01-SUMMARY.md
@.planning/phases/03-websocket-hub/03-02-SUMMARY.md
@.planning/research/PITFALLS.md
@internal/wshub/hub.go
@internal/wshub/subscribe.go
@internal/wshub/hub_test.go
@internal/wshub/subscribe_test.go

<interfaces>
<!-- Production log lines that the tests assert. Quoted verbatim from Plan 03-02's hub.go. -->

```go
// hub.go Publish (slow drop path):
h.logger.Warn("wshub: subscriber dropped (slow consumer)",
    "buffer_size", h.opts.MessageBuffer)

// hub.go Close:
h.logger.Info("wshub: closing hub", "subscribers", len(h.subscribers))

// subscribe.go writer loop abnormal exit:
h.logger.Debug("wshub: subscriber writer loop exited", "error", err.Error())
```

The three log-capture tests assert the Warn and Info lines above. They do NOT assert the Debug line (it's a best-effort diagnostic and its presence depends on timing; asserting it would introduce flakiness).
</interfaces>

<locked_decisions>
<!-- All from 03-CONTEXT.md §"Drop-subscriber logging" + §"Test surface". DO NOT re-open. -->
1. **Log capture mechanism**: `var buf bytes.Buffer; logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))`. The `LevelDebug` level ensures Debug lines would also be captured IF they appear, so the "no PII" test can assert their absence too.
2. **TextHandler, not JSONHandler**, for tests. The output format is stable enough for substring assertions (`level=WARN`, `msg=...`, key=value) and easier to read in test failures.
3. **Assertions use `require.Contains` / `require.NotContains` on the buffer's string representation**. No regex, no parsing — the assertions are grep-equivalent.
4. **No PII list** (things that MUST NOT appear in any wshub log line):
   - `peer_addr`, `remote_addr`, `r.RemoteAddr` literal substring
   - `user_agent`, `User-Agent`
   - `authorization`, `Authorization`, `Basic `, `Bearer `
   - `username`, `password`
5. **The three logging tests live in `hub_test.go`**, appended after the Plan 03-02 tests. Same `package wshub` internal test file.
6. **No new production code in this plan.** If a logging test fails because the production log format is wrong, the fix goes in `hub.go`, but the planning expectation is that Plan 03-02's implementation is already correct — this plan is a belt-and-suspenders lock-in, not a gap fix.
7. **Architectural gates are run as part of the plan's verification section**, not as Go test functions. Shell commands (grep + go list) are more appropriate than writing a Go test that shells out to run `go list`.
</locked_decisions>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: RED — append TestHub_Logging_DropIsWarn, TestHub_Logging_CloseIsInfo, TestHub_Logging_NoPII to hub_test.go</name>
  <files>internal/wshub/hub_test.go</files>
  <read_first>
- internal/wshub/hub_test.go (the existing four tests from Plan 03-02 — APPEND, do not replace)
- internal/wshub/hub.go (the exact Warn/Info log line strings from Plan 03-02)
- .planning/phases/03-websocket-hub/03-CONTEXT.md §"Drop-subscriber logging" (exact log line formats, no-PII list)
- .planning/phases/03-websocket-hub/03-RESEARCH.md §"Example 6: Log-capture test for drop-log format" (verbatim pattern)
- .planning/phases/03-websocket-hub/03-VALIDATION.md row 03-03-02
  </read_first>
  <behavior>
Append three new test functions to `internal/wshub/hub_test.go`, all in `package wshub`:

1. **TestHub_Logging_DropIsWarn** — constructs a Hub with a bytes.Buffer-backed TextHandler at LevelDebug, MessageBuffer=1, short PingInterval. Connects a slow subscriber via httptest.NewServer (same pattern as `TestHub_SlowConsumerDropped`). Publishes 5 messages. Waits via `require.Eventually` until the buffer contains `"wshub: subscriber dropped (slow consumer)"`. Then asserts: buffer contains `level=WARN`; buffer contains `buffer_size=1`; buffer does NOT contain any of the PII substrings from the no-PII list.

2. **TestHub_Logging_CloseIsInfo** — constructs a Hub with a bytes.Buffer logger. Calls `hub.Close()` directly (no subscribers needed). Asserts: buffer contains exactly one `"wshub: closing hub"` line; buffer contains `level=INFO`; buffer contains `subscribers=0`; buffer does NOT contain `level=WARN` or `level=ERROR`. The exactly-one assertion uses `strings.Count(buf.String(), "wshub: closing hub") == 1`.

3. **TestHub_Logging_NoPII** — constructs a Hub with a bytes.Buffer logger, runs a full connect → publish → drop → close flow (combining the slow-consumer pattern with a Hub.Close at the end), and asserts that the full captured log contains none of the PII substrings. This is the comprehensive no-PII assertion — the first two tests scope-limit to single log lines; this one asserts across the entire hub lifecycle.

All three tests MUST use `require.Eventually` (not `time.Sleep`) wherever they wait on log output.

**Note on RED phase for this task**: the production code from Plan 03-02 ALREADY logs at the correct level with the correct format. These tests will likely pass on their first run. That's OK — the RED bar is symbolic here: we're locking in the observable contract. If the tests do happen to fail because 03-02 got the format wrong, fix `hub.go` in Task 2 (GREEN).
  </behavior>
  <action>
**APPEND (do not replace) the following three test functions to `internal/wshub/hub_test.go`.**

Add these at the bottom of the file, after the existing `TestHub_Publish_AfterCloseIsNoOp` function. Do NOT re-declare imports — the imports already include `bytes`, `log/slog`, `strings` is needed NEW so add it to the imports block.

First, update the imports block at the top of `hub_test.go` to add `"strings"`:

```go
import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)
```

Then append these three functions at the end of the file:

```go
// piiSubstrings enumerates strings that MUST NOT appear in any wshub
// log line. The hub is byte-oriented and has no notion of client
// identity; logging any of these would violate the no-PII contract
// from 03-CONTEXT.md §"Drop-subscriber logging".
var piiSubstrings = []string{
	"peer_addr",
	"remote_addr",
	"RemoteAddr",
	"user_agent",
	"User-Agent",
	"authorization",
	"Authorization",
	"Basic ",
	"Bearer ",
	"username",
	"password",
}

func requireNoPII(t *testing.T, out string) {
	t.Helper()
	for _, pii := range piiSubstrings {
		require.NotContainsf(t, out, pii,
			"no-PII contract violated: %q appeared in log output", pii)
	}
}

func TestHub_Logging_DropIsWarn(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	hub := New(logger, Options{
		MessageBuffer: 1,
		PingInterval:  10 * time.Millisecond,
	})
	srv := httptest.NewServer(subscribeHandler(hub))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, srv.URL, nil)
	require.NoError(t, err)
	defer conn.CloseNow()

	// Slow-consumer simulation: never read from conn.
	waitForSubscribers(t, hub, 1, time.Second)

	for i := 0; i < 5; i++ {
		hub.Publish([]byte("msg"))
	}

	// Wait for the Warn line to appear.
	require.Eventually(t, func() bool {
		return strings.Contains(buf.String(), "wshub: subscriber dropped (slow consumer)")
	}, time.Second, 10*time.Millisecond, "drop Warn log never appeared")

	out := buf.String()
	require.Contains(t, out, "level=WARN", "drop log must be at Warn level")
	require.Contains(t, out, "buffer_size=1", "drop log must carry buffer_size field")
	requireNoPII(t, out)
}

func TestHub_Logging_CloseIsInfo(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	hub := New(logger, Options{})

	hub.Close()

	out := buf.String()
	require.Contains(t, out, "wshub: closing hub", "Close must log Info")
	require.Contains(t, out, "level=INFO", "close log must be at Info level")
	require.Contains(t, out, "subscribers=0", "close log must carry subscribers count")
	require.Equal(t, 1, strings.Count(out, "wshub: closing hub"),
		"Close must log exactly one line")
	require.NotContains(t, out, "level=WARN", "Close must not log at Warn")
	require.NotContains(t, out, "level=ERROR", "Close must not log at Error")
	requireNoPII(t, out)
}

func TestHub_Logging_NoPII(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	hub := New(logger, Options{
		MessageBuffer: 1,
		PingInterval:  10 * time.Millisecond,
	})
	srv := httptest.NewServer(subscribeHandler(hub))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Full lifecycle: connect, publish (drops the slow subscriber),
	// close the hub. The captured log buffer must contain NONE of
	// the PII substrings across this entire flow.
	conn, _, err := websocket.Dial(ctx, srv.URL, nil)
	require.NoError(t, err)
	defer conn.CloseNow()

	waitForSubscribers(t, hub, 1, time.Second)

	for i := 0; i < 5; i++ {
		hub.Publish([]byte("msg"))
	}

	waitForSubscribers(t, hub, 0, time.Second)

	hub.Close()

	// Give any deferred log writes a chance to flush via require.Eventually
	// (no time.Sleep). The condition is immediately true on entry in the
	// common case; the Eventually loop is just a safety margin.
	require.Eventually(t, func() bool {
		return strings.Contains(buf.String(), "wshub: closing hub")
	}, time.Second, 10*time.Millisecond)

	requireNoPII(t, buf.String())
}
```

Run the tests:

```
cd /home/ben/Dev-local/openburo-spec/open-buro-server
~/sdk/go1.26.2/bin/go test ./internal/wshub -race -run '^TestHub_Logging_' -timeout 30s -v
```

Expected outcome: all three tests PASS immediately (Plan 03-02's production logs already match the contract). If any fail, see Task 2 for remediation.

Commit as: `test(03-03): lock logging contract (Warn-on-drop, Info-on-close, no PII)`.
  </action>
  <verify>
    <automated>cd /home/ben/Dev-local/openburo-spec/open-buro-server && ~/sdk/go1.26.2/bin/go test ./internal/wshub -race -run '^TestHub_Logging_DropIsWarn$|^TestHub_Logging_CloseIsInfo$|^TestHub_Logging_NoPII$' -timeout 30s && ~/sdk/go1.26.2/bin/go vet ./internal/wshub && ~/sdk/go1.26.2/bin/gofmt -l internal/wshub/hub_test.go</automated>
  </verify>
  <acceptance_criteria>
- `grep -c 'func TestHub_Logging_DropIsWarn' internal/wshub/hub_test.go` equals 1
- `grep -c 'func TestHub_Logging_CloseIsInfo' internal/wshub/hub_test.go` equals 1
- `grep -c 'func TestHub_Logging_NoPII' internal/wshub/hub_test.go` equals 1
- `grep -c 'var piiSubstrings' internal/wshub/hub_test.go` equals 1
- `grep -c 'func requireNoPII' internal/wshub/hub_test.go` equals 1
- `grep -c '"strings"' internal/wshub/hub_test.go` equals 1 (import added)
- `! grep -n 'time\.Sleep' internal/wshub/hub_test.go` (PITFALLS #16)
- `~/sdk/go1.26.2/bin/go test ./internal/wshub -race -run '^TestHub_Logging_' -timeout 30s` exits 0
- `~/sdk/go1.26.2/bin/gofmt -l internal/wshub/hub_test.go` produces no output
- Plan 03-01 and 03-02 tests still pass (no regression in `TestHub_Publish_FanOut`, `TestHub_SlowConsumerDropped`, etc.)
  </acceptance_criteria>
  <done>
Three logging-capture tests landed, all GREEN on first run (locking in Plan 03-02's already-correct log format). The no-PII contract is now enforced by test, not just by convention.
  </done>
</task>

<task type="auto">
  <name>Task 2: Architectural gate sweep — run the full Phase 3 verification script and document results</name>
  <files>.planning/phases/03-websocket-hub/03-GATES.md</files>
  <read_first>
- internal/wshub/hub.go (final state)
- internal/wshub/subscribe.go (final state)
- internal/wshub/hub_test.go (final state after Task 1)
- internal/wshub/subscribe_test.go (final state)
- go.mod (the v1.8.14 pin)
- .planning/phases/03-websocket-hub/03-CONTEXT.md §"Integration Points" (the three gates)
- .planning/phases/03-websocket-hub/03-VALIDATION.md row 03-03-03 (architectural gates)
  </read_first>
  <action>
This task is a verification-only task — it runs the phase's full gate sweep and writes the results to `.planning/phases/03-websocket-hub/03-GATES.md`. No production code changes.

**Step 1: Run the full gate sweep from the repo root.** Each command's output goes into the markdown file.

```
cd /home/ben/Dev-local/openburo-spec/open-buro-server

# Gate 1: Full wshub test suite under -race.
~/sdk/go1.26.2/bin/go test ./internal/wshub -race -timeout 60s -v

# Gate 2: Goroutine-leak test in isolation (WS-10 acceptance).
~/sdk/go1.26.2/bin/go test ./internal/wshub -race -run '^TestSubscribe_NoGoroutineLeak$' -timeout 30s -v

# Gate 3: Architectural isolation — wshub does NOT import registry or httpapi.
~/sdk/go1.26.2/bin/go list -deps ./internal/wshub | grep -E '^github\.com/openburo/openburo-server/internal/(registry|httpapi)$' ; echo "exit=$?"
# Expected: grep exit=1 (no matches found)

# Gate 4: Logging gate — no slog.Default in production wshub code.
grep -rE 'slog\.Default' internal/wshub/*.go | grep -v _test.go ; echo "exit=$?"
# Expected: grep exit=1 (no matches found)

# Gate 5: No-time.Sleep gate — PITFALLS #16 enforcement.
grep -n 'time\.Sleep' internal/wshub/*_test.go ; echo "exit=$?"
# Expected: grep exit=1 (no matches found)

# Gate 6: No leftover TODO markers from prior plans.
grep -n 'TODO(03-02)' internal/wshub/*.go ; echo "exit=$?"
# Expected: grep exit=1 (no matches found)

# Gate 7: Full module build, vet, gofmt.
~/sdk/go1.26.2/bin/go build ./...
~/sdk/go1.26.2/bin/go vet ./...
~/sdk/go1.26.2/bin/gofmt -l internal/wshub/
# Expected: zero output from gofmt; exit 0 from build + vet

# Gate 8: Whole-module race-clean test.
~/sdk/go1.26.2/bin/go test ./... -race -timeout 120s
# Expected: PASS across all packages
```

**Step 2: Write `.planning/phases/03-websocket-hub/03-GATES.md`** with the following structure. Fill in the actual command output verbatim — do NOT paraphrase.

```markdown
# Phase 3: WebSocket Hub — Gate Sweep Results

**Date:** <YYYY-MM-DD of execution>
**Go version:** <output of `~/sdk/go1.26.2/bin/go version`>
**Status:** <PASS | FAIL>

## Summary Table

| Gate | Command | Expected | Actual | Status |
|------|---------|----------|--------|--------|
| 1 | Full wshub test suite (-race) | all tests PASS | <N tests PASS> | <PASS/FAIL> |
| 2 | TestSubscribe_NoGoroutineLeak in isolation | PASS | <...> | <PASS/FAIL> |
| 3 | go list -deps (no registry/httpapi) | empty | <empty/<list>> | <PASS/FAIL> |
| 4 | no slog.Default in production code | empty | <empty/<list>> | <PASS/FAIL> |
| 5 | no time.Sleep in tests | empty | <empty/<list>> | <PASS/FAIL> |
| 6 | no TODO(03-02) markers | empty | <empty/<list>> | <PASS/FAIL> |
| 7 | build + vet + gofmt | clean | <clean/<errors>> | <PASS/FAIL> |
| 8 | whole-module race-clean test | PASS | <...> | <PASS/FAIL> |

## Gate 1: Full wshub test suite

\`\`\`
<verbatim output of `go test ./internal/wshub -race -timeout 60s -v`>
\`\`\`

## Gate 2: Goroutine-leak test (WS-10)

\`\`\`
<verbatim output>
\`\`\`

## Gate 3: Architectural isolation

\`\`\`
$ go list -deps ./internal/wshub | grep -E '^github\.com/openburo/openburo-server/internal/(registry|httpapi)$'
<verbatim output, should be empty>
$ echo "exit=$?"
exit=1
\`\`\`

## Gate 4: No slog.Default in production

\`\`\`
$ grep -rE 'slog\.Default' internal/wshub/*.go | grep -v _test.go
<verbatim output, should be empty>
$ echo "exit=$?"
exit=1
\`\`\`

## Gate 5: No time.Sleep in tests

\`\`\`
$ grep -n 'time\.Sleep' internal/wshub/*_test.go
<verbatim output, should be empty>
$ echo "exit=$?"
exit=1
\`\`\`

## Gate 6: No TODO(03-02) markers

\`\`\`
$ grep -n 'TODO(03-02)' internal/wshub/*.go
<verbatim output, should be empty>
$ echo "exit=$?"
exit=1
\`\`\`

## Gate 7: Build + vet + gofmt

\`\`\`
<verbatim output of the three commands>
\`\`\`

## Gate 8: Whole-module race-clean

\`\`\`
<verbatim output of `go test ./... -race -timeout 120s`>
\`\`\`

## Requirements Closed

- [x] WS-02: Hub + subscriber + buffered channel (default 16)
- [x] WS-03: Non-blocking fan-out with drop-slow-consumer via closeSlow
- [x] WS-04: conn.CloseRead(ctx) + defer removeSubscriber
- [x] WS-07: Periodic ping frames (default 30s, configurable)
- [x] WS-10: 1000-cycle goroutine-leak test green under -race

## Deferred to Phase 4

- WS-01: GET /api/v1/capabilities/ws upgrade handler
- WS-05: REGISTRY_UPDATED event broadcast on mutation
- WS-06: Full-state snapshot on connect
- WS-08: OriginPatterns from shared CORS allow-list
- WS-09: Mutation-then-broadcast handler ordering

## Deferred to Phase 5

- OPS-04: Two-phase graceful shutdown (hub.Close after httpSrv.Shutdown)
```

**Step 3:** If ANY gate fails, STOP and escalate. Do NOT adjust gates to make them pass. Investigate the root cause and either fix the production code (creating a new RED→GREEN cycle if needed) or flag the failure to the orchestrator for gap-closure planning.

Commit as: `docs(03-03): phase 3 gate sweep results — all gates green`.
  </action>
  <verify>
    <automated>cd /home/ben/Dev-local/openburo-spec/open-buro-server && test -f .planning/phases/03-websocket-hub/03-GATES.md && grep -q 'Status:.*PASS' .planning/phases/03-websocket-hub/03-GATES.md && ~/sdk/go1.26.2/bin/go test ./internal/wshub -race -timeout 60s && ~/sdk/go1.26.2/bin/go test ./... -race -timeout 120s</automated>
  </verify>
  <acceptance_criteria>
- `.planning/phases/03-websocket-hub/03-GATES.md` exists and contains the string `Status: PASS`
- All 8 gate rows in the summary table show `PASS`
- `~/sdk/go1.26.2/bin/go test ./internal/wshub -race -timeout 60s` exits 0 (all 11 tests pass: 3 from Plan 03-01, 5 from Plan 03-02, 3 from Plan 03-03)
- `~/sdk/go1.26.2/bin/go test ./... -race -timeout 120s` exits 0 across the whole module
- `~/sdk/go1.26.2/bin/go list -deps ./internal/wshub | grep -E '^github\.com/openburo/openburo-server/internal/(registry|httpapi)$'` produces no output
- `grep -rE 'slog\.Default' internal/wshub/*.go | grep -v _test.go` produces no output
- `grep -n 'time\.Sleep' internal/wshub/*_test.go` produces no output
- `grep -n 'TODO(03-02)' internal/wshub/*.go` produces no output
- `~/sdk/go1.26.2/bin/gofmt -l internal/wshub/` produces no output
- `~/sdk/go1.26.2/bin/go vet ./...` exits 0
  </acceptance_criteria>
  <done>
Phase 3 verification is locked in a committed artifact. All gates green. The byte-oriented contract (no registry/httpapi imports), the injected-logger contract (no slog.Default), and the no-flaky-time-tests contract (no time.Sleep) are all passing. WS-02, WS-03, WS-04, WS-07, WS-10 are verified by specific test names.
  </done>
</task>

</tasks>

<verification>
Overall plan verification (runs all eight gates in order; any failure aborts):

```
cd /home/ben/Dev-local/openburo-spec/open-buro-server
set -e

# Gate 1: Full wshub test suite under -race.
~/sdk/go1.26.2/bin/go test ./internal/wshub -race -timeout 60s

# Gate 2: Goroutine-leak test in isolation (WS-10).
~/sdk/go1.26.2/bin/go test ./internal/wshub -race -run '^TestSubscribe_NoGoroutineLeak$' -timeout 30s

# Gate 3: Architectural isolation.
if ~/sdk/go1.26.2/bin/go list -deps ./internal/wshub | grep -E '^github\.com/openburo/openburo-server/internal/(registry|httpapi)$'; then
  echo "GATE FAIL: wshub imports registry or httpapi" >&2
  exit 1
fi

# Gate 4: No slog.Default in production wshub.
if grep -rE 'slog\.Default' internal/wshub/*.go | grep -v _test.go; then
  echo "GATE FAIL: slog.Default found in wshub production code" >&2
  exit 1
fi

# Gate 5: No time.Sleep in wshub tests.
if grep -n 'time\.Sleep' internal/wshub/*_test.go; then
  echo "GATE FAIL: time.Sleep found in wshub tests" >&2
  exit 1
fi

# Gate 6: No TODO(03-02) markers.
if grep -n 'TODO(03-02)' internal/wshub/*.go; then
  echo "GATE FAIL: leftover TODO(03-02) markers" >&2
  exit 1
fi

# Gate 7: Build + vet + gofmt.
~/sdk/go1.26.2/bin/go build ./...
~/sdk/go1.26.2/bin/go vet ./...
if [ -n "$(~/sdk/go1.26.2/bin/gofmt -l internal/wshub/)" ]; then
  echo "GATE FAIL: gofmt found unformatted files" >&2
  exit 1
fi

# Gate 8: Whole-module race-clean.
~/sdk/go1.26.2/bin/go test ./... -race -timeout 120s

echo "ALL GATES PASS"
```
</verification>

<success_criteria>
- `internal/wshub/hub_test.go` contains `TestHub_Logging_DropIsWarn`, `TestHub_Logging_CloseIsInfo`, `TestHub_Logging_NoPII`, plus the `piiSubstrings` list and `requireNoPII` helper
- All three logging tests pass on first run (Plan 03-02's production log format is locked by assertion)
- `.planning/phases/03-websocket-hub/03-GATES.md` exists and documents PASS for all 8 gates with verbatim command output
- The full wshub test suite (11 tests: 3 from 03-01 + 5 from 03-02 + 3 from 03-03) passes under `-race` in under 60 seconds
- `TestSubscribe_NoGoroutineLeak` (WS-10 acceptance) is the defining correctness test and is green — if it flakes, Phase 3 is not done
- All 8 architectural gates (wshub ⊄ registry/httpapi, no slog.Default, no time.Sleep, no stale TODOs, build, vet, gofmt, whole-module race) are green and documented
- WS-02, WS-03, WS-04, WS-07, WS-10 are all closed; the 5 Phase 3 requirements from ROADMAP.md are observably implemented
- Phase 3 is feature-complete and ready for `/gsd:verify-work`
</success_criteria>

<output>
After completion, create `.planning/phases/03-websocket-hub/03-03-SUMMARY.md` following the template. Include: the three logging tests added, the gate sweep results summarized (all 8 PASS), confirmation that WS-10's goroutine-leak test is the load-bearing correctness test and is stable under `-race`, and the list of Phase 3 requirements closed (WS-02, WS-03, WS-04, WS-07, WS-10). Reference `.planning/phases/03-websocket-hub/03-GATES.md` for verbatim command output.
</output>
