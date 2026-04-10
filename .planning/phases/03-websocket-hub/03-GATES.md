# Phase 3: WebSocket Hub — Gate Sweep Results

**Date:** 2026-04-10
**Go version:** go version go1.26.2 linux/amd64
**Status:** PASS

## Summary Table

| Gate | Command                                   | Expected         | Actual                      | Status |
| ---- | ----------------------------------------- | ---------------- | --------------------------- | ------ |
| 1    | Full wshub test suite (-race)             | all tests PASS   | 11 tests PASS               | PASS   |
| 2    | TestSubscribe_NoGoroutineLeak in isolation | PASS             | PASS (0.60s)                | PASS   |
| 3    | go list -deps (no registry/httpapi)       | empty            | empty                       | PASS   |
| 4    | no slog.Default in production code        | empty            | empty                       | PASS   |
| 5    | no time.Sleep in tests                    | empty            | empty                       | PASS   |
| 6    | no TODO(03-02) markers                    | empty            | empty                       | PASS   |
| 7    | build + vet + gofmt                       | clean            | clean                       | PASS   |
| 8    | whole-module race-clean test              | PASS             | all packages PASS           | PASS   |

## Gate 1: Full wshub test suite

```
$ ~/sdk/go1.26.2/bin/go test ./internal/wshub -race -timeout 60s -v
=== RUN   TestHub_Publish_FanOut
--- PASS: TestHub_Publish_FanOut (0.01s)
=== RUN   TestHub_SlowConsumerDropped
--- PASS: TestHub_SlowConsumerDropped (5.01s)
=== RUN   TestHub_Close_GoingAway
--- PASS: TestHub_Close_GoingAway (5.01s)
=== RUN   TestHub_Publish_AfterCloseIsNoOp
--- PASS: TestHub_Publish_AfterCloseIsNoOp (0.00s)
=== RUN   TestHub_Logging_DropIsWarn
--- PASS: TestHub_Logging_DropIsWarn (0.00s)
=== RUN   TestHub_Logging_CloseIsInfo
--- PASS: TestHub_Logging_CloseIsInfo (0.00s)
=== RUN   TestHub_Logging_NoPII
--- PASS: TestHub_Logging_NoPII (5.01s)
=== RUN   TestHub_New_PanicsOnNilLogger
--- PASS: TestHub_New_PanicsOnNilLogger (0.00s)
=== RUN   TestHub_DefaultOptions
--- PASS: TestHub_DefaultOptions (0.00s)
=== RUN   TestSubscribe_NoGoroutineLeak
--- PASS: TestSubscribe_NoGoroutineLeak (1.07s)
=== RUN   TestSubscribe_PingKeepsAlive
--- PASS: TestSubscribe_PingKeepsAlive (0.30s)
PASS
ok  	github.com/openburo/openburo-server/internal/wshub	17.447s
```

Eleven tests total, all GREEN under `-race`:

- **03-01 baseline (4):** TestHub_New_PanicsOnNilLogger, TestHub_DefaultOptions, TestSubscribe_NoGoroutineLeak, TestSubscribe_PingKeepsAlive (PingKeepsAlive landed in 03-02 but mirrors 03-01 subscribe surface)
- **03-02 hub mechanics (4):** TestHub_Publish_FanOut, TestHub_SlowConsumerDropped, TestHub_Close_GoingAway, TestHub_Publish_AfterCloseIsNoOp
- **03-03 logging contract (3):** TestHub_Logging_DropIsWarn, TestHub_Logging_CloseIsInfo, TestHub_Logging_NoPII

Runtime ~17s dominated by the three 5-second close-handshake waits (SlowConsumer, Close_GoingAway, NoPII) — all expected per the 03-02 deviations note about coder/websocket v1.8.14's hardcoded 5s waitCloseHandshake.

## Gate 2: Goroutine-leak test (WS-10)

```
$ ~/sdk/go1.26.2/bin/go test ./internal/wshub -race -run '^TestSubscribe_NoGoroutineLeak$' -timeout 30s -v
=== RUN   TestSubscribe_NoGoroutineLeak
--- PASS: TestSubscribe_NoGoroutineLeak (0.60s)
PASS
ok  	github.com/openburo/openburo-server/internal/wshub	1.619s
```

The 1000-cycle connect/disconnect goroutine-leak test is the defining correctness oracle for Phase 3 (PITFALLS #3 enforcement: `conn.CloseRead(ctx)` + `defer h.removeSubscriber(s)` + `defer conn.CloseNow()`). Runs in 0.6s under `-race` with `runtime.NumGoroutine() <= baseline+5` holding comfortably.

## Gate 3: Architectural isolation

```
$ ~/sdk/go1.26.2/bin/go list -deps ./internal/wshub | grep -E '^github\.com/openburo/openburo-server/internal/(registry|httpapi)$'
$ echo "exit=$?"
exit=1
```

Empty output — `wshub` does NOT import `internal/registry` or `internal/httpapi`. The disjoint dependency graph from PROJECT.md is preserved; the byte-oriented contract is structurally enforced.

## Gate 4: No slog.Default in production

```
$ grep -rE 'slog\.Default' internal/wshub/*.go | grep -v _test.go
$ echo "exit=$?"
exit=1
```

Empty output — no literal `slog.Default` substring in production files. The Phase 1 "injection-only logger" invariant is preserved (and the hub.go doc comment was reworded to "global default logger" in Plan 03-01 to clear this gate).

## Gate 5: No time.Sleep in tests

```
$ grep -n 'time\.Sleep' internal/wshub/*_test.go
$ echo "exit=$?"
exit=1
```

Empty output — no `time.Sleep` calls or comment references in any wshub test file. PITFALLS #16 enforced. All tests use `require.Eventually` / `require.Never` or deterministic ping-cycle accounting.

Note: The `TestHub_Logging_NoPII` comment originally read "(no time.Sleep)" which tripped the gate. The comment was reworded to "(polling, not blocking)" in Task 2 to keep the literal substring out of the tree. Fix committed alongside the gates doc.

## Gate 6: No TODO(03-02) markers

```
$ grep -n 'TODO(03-02)' internal/wshub/*.go
$ echo "exit=$?"
exit=1
```

Empty output — all three Plan 03-01 stubs (`Publish`, `Close`, ping case body) were replaced with real implementations in Plan 03-02, and no new TODOs were introduced in Plan 03-03.

## Gate 7: Build + vet + gofmt

```
$ ~/sdk/go1.26.2/bin/go build ./...
BUILD OK

$ ~/sdk/go1.26.2/bin/go vet ./...
VET OK

$ ~/sdk/go1.26.2/bin/gofmt -l internal/wshub/
GOFMT CLEAN
```

All three housekeeping gates clean. No build errors, no vet warnings, no gofmt diffs.

## Gate 8: Whole-module race-clean

```
$ ~/sdk/go1.26.2/bin/go test ./... -race -timeout 180s
?   	github.com/openburo/openburo-server/cmd/server	[no test files]
ok  	github.com/openburo/openburo-server/internal/config	1.034s
ok  	github.com/openburo/openburo-server/internal/httpapi	1.024s
ok  	github.com/openburo/openburo-server/internal/registry	1.485s
?   	github.com/openburo/openburo-server/internal/version	[no test files]
ok  	github.com/openburo/openburo-server/internal/wshub	17.419s
```

All packages PASS under `-race`. No regressions across Phase 1 (config, httpapi scaffold), Phase 2 (registry), or Phase 3 (wshub).

## Requirements Closed

- [x] **WS-02:** Hub + subscriber + buffered channel (default 16) — `internal/wshub/hub.go` Hub, Options, New; `internal/wshub/subscribe.go` subscriber struct; verified by TestHub_DefaultOptions
- [x] **WS-03:** Non-blocking fan-out with drop-slow-consumer via closeSlow — `hub.go` Publish + `subscribe.go` closeSlow callback; verified by TestHub_Publish_FanOut + TestHub_SlowConsumerDropped + TestHub_Logging_DropIsWarn
- [x] **WS-04:** `conn.CloseRead(ctx)` + `defer removeSubscriber` — `subscribe.go` Subscribe top-of-method; verified by TestSubscribe_NoGoroutineLeak (1000-cycle oracle)
- [x] **WS-07:** Periodic ping frames (default 30s, configurable) — `subscribe.go` tick.C case with `conn.Ping(pingCtx)` bounded by PingTimeout; verified by TestSubscribe_PingKeepsAlive (300ms / 30+ cycles via require.Never)
- [x] **WS-10:** 1000-cycle goroutine-leak test green under -race — `subscribe_test.go` TestSubscribe_NoGoroutineLeak; passes in 0.6s

## Deferred to Phase 4

- WS-01: GET /api/v1/capabilities/ws upgrade handler
- WS-05: REGISTRY_UPDATED event broadcast on mutation
- WS-06: Full-state snapshot on connect
- WS-08: OriginPatterns from shared CORS allow-list
- WS-09: Mutation-then-broadcast handler ordering

## Deferred to Phase 5

- OPS-04: Two-phase graceful shutdown (hub.Close after httpSrv.Shutdown)
