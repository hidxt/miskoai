# Task3 memory/profile integration handoff

Status: DONE. Owned production source and tests are frozen for independent review.
Base checkpoint: 358fc7a. Child configuration: GPT-6.1 Medium, no delegation.

## Changed files

- internal/agent/agent.go
- internal/agent/context.go
- internal/agent/commands.go
- internal/agent/profiles.go
- internal/agent/agent_test.go
- internal/agent/context_test.go
- internal/service/service.go
- internal/service/service_test.go
- internal/storage/derived.go
- internal/storage/profiles.go
- internal/storage/profiles_test.go
- docs/superpowers/reports/2026-10-09-memory-integration.md

No other module or root document was edited by this task. Concurrent root-owned documentation changes visible in status were left untouched. No staging, commit, push, subagent, live provider request, private database access/migration, or service start with actual credentials occurred.

## Implementation and contract checks

Ordinary answers capture summary, scoped profile, history, and facts through one ChatContextWithProfile transaction. Existing ChatContext delegates to the same implementation. Empty options use persistent selection; explicit builtin/custom options override only the captured profile. Profile IDs use the existing lowercase ASCII/digit/underscore/hyphen rules and 64-byte cap. Custom option construction performs a bounded scoped lookup, and each answer checks again before search/model work so deletion produces a safe input refusal. Active and explicit resolution share profileByID; missing active data retains its prior ErrInvalid behavior while a missing explicit ID yields ErrNotFound. No schema, admission, quota, or selection write changed.

The fixed safety system instruction is preserved verbatim. Builtin expression remains system instruction; custom profile JSON and a UTF-8-safe summary prefix of at most 2048 bytes are quoted lower user-role data. Context drops oldest whole history pairs before optional sources/facts/summary/custom expression. Current input and safety remain mandatory and are rejected if they cannot fit.

Candidate commands use storage directly: list requests at most eight candidates, confirm requires an explicit numeric ID and creates an explicit_user fact through the reviewed store, and reject deletes only the scoped candidate. Foreign/missing IDs receive a safe input refusal. Listing encodes one bounded candidate at a time and checks full JSON expansion against 16384 bytes before appending, returning a size-limit notice rather than truncated JSON. No model/search is invoked by review commands.

/memory clear now calls ClearMemory and honestly describes clearing explicit memory, derived summary, candidates, and prior context while keeping message IDs for duplicate prevention. The reviewed revision check rejects stale generated writes after clearing. ClearMemory also erases the current command content; its subsequently persisted reply alone does not enter History because History excludes messages with empty content. Later completed messages with nonempty content form new context.

Service.SetObserver accepts an optional Observer only before Run starts. The serial worker invokes Wake synchronously only after a sent result with nil handler error and successful durable inbox completion. The supplied summary.Worker implements a nonblocking coalesced Wake. No new worker or per-message goroutine was introduced; service shutdown and ambiguous-state handling remain unchanged.

## RED evidence

All commands used verified .tools/toolchains/go1.27.2-verified/go/bin/go.exe with GOTOOLCHAIN=local, GOPROXY=off, GOSUMDB=off, GOPATH=.tools/go, GOMODCACHE=.tools/gomod, GOCACHE=.tools/gocache, and GOTMPDIR/TEMP/TMP=.tools/tmp resolved under the workspace. Tests used only synthetic temporary fixtures.

1. `go test ./internal/agent -run 'TestPersistentProfile|TestMemoryClear|TestExplicitCustom|TestCandidateCommands' -count=1` exited 1 before production edits. Persistent derived/profile data was absent (`hello` was the only user data), the old clear reply omitted context and retained-ID semantics, custom options were rejected, and candidate listing returned the ordinary no-facts response instead of JSON.
2. `go test ./internal/storage -run TestCapturedExplicitProfileDoesNotChangeSelection -count=1` exited 1 because ChatContextWithProfile did not exist.
3. `go test ./internal/agent ./internal/service -run 'TestCapturedSummary|TestObserver' -count=1` exited 1 because buildSnapshotContext and SetObserver did not exist.

An initial test draft used an incorrect ErrConflict symbol and omitted a required synthetic fact Source. These fixture issues were corrected before the meaningful agent RED run above; neither was counted as a behavior regression.

## GREEN verification

Commands below use the exact verified executable/environment described above. No cached test results were used (`-count=1`).

- `go test ./internal/agent ./internal/service ./internal/storage ./internal/summary -count=1` exited 0: agent 31.989s, service 6.199s, storage 111.022s, summary 30.127s.
- After bounded candidate and slow-summary tests: `go test ./internal/agent ./internal/service -run 'TestCandidateReply|TestSummaryObserver' -count=1` exited 0: agent 1.865s, service 1.812s.
- After actual send-ack observer tests: `go test ./internal/agent ./internal/service ./internal/storage ./internal/summary -count=1` exited 0: agent 5.513s, service 12.873s, storage 88.697s, summary 7.694s.
- After the final explicit custom/builtin override regression test: `go test ./internal/agent -count=1` exited 0: agent 7.274s.
- Final `go vet ./internal/agent ./internal/service ./internal/storage ./internal/summary` exited 0, no diagnostics.
- Final `git diff --check` exited 0. Git emitted only CRLF normalization warnings for concurrent root-owned documents.
- `go version` reported `go version go1.27.2 windows/amd64`.

New tests cover persistent selection and hot custom updates, quoted injection data, summary UTF-8 byte bounds and mandatory budget preservation, custom/builtin explicit overrides without selection mutation, foreign/deleted custom refusal before search/model, deterministic candidate listing/confirmation/rejection and scope isolation, JSON escape expansion and the eight-item cap, clear/stale revision/duplicate preservation, pre-Run observer registration, durable completion ordering, actual ACK/ambiguous-send boundaries, and continued serial processing during a blocked summary call. Existing reviewed storage/summary suites passed alongside the integration.

## Self-review and limitations

Inspected owned diffs for scoped reads, absence of selection mutation, fixed policy retention, bounded allocations, deterministic commands, existing single-send behavior, observer lifecycle synchronization, and no detached/background integration work. No unresolved implementation issue identified in the assigned scope.

This is Windows synthetic evidence only. Native Linux/race, root full-suite integration, scans, fresh independent review, actual provider/WeChat validation, and 512MB performance acceptance remain root gates. The observer interface intentionally requires prompt/nonblocking Wake; it cannot prevent an arbitrary third-party implementation from blocking, and no goroutine is added to hide that behavior. This task adds the hook; higher-level Core lifetime wiring remains separately owned/planned.

## Independent review round1 correction

Accepted Important I1: encodeFacts and encodeCandidates duplicated the bounded-array marshal/separator/limit/append/close logic. Factored that logic into the small generic encodeBoundedArray helper. encodeFacts passes the existing fact slice directly; encodeCandidates passes only the at-most-eight-item prefix view. Neither wrapper copies/converts the slice or marshals it as a whole. Empty arrays retain the tiny exact `[]` result. Nonempty arrays retain one reply-sized output buffer and one temporary item encoding, stop immediately on overflow, and return complete valid JSON or a limit result.

Only internal/agent/commands.go and this report changed in this correction. Existing exact-boundary, small/empty JSON, escape/allocation, and candidate escape/eight-item regressions already exercise the shared boundary; no extra test was needed for this behavior-preserving refactor. Corrected the ClearMemory explanation above to match the existing History empty-content exclusion.

Final round1 verification on the same verified Go1.27.2/environment:

- `go test ./internal/agent -count=1 -v` exited 0, package 5.900s; all agent tests passed. TestEscapedExportStopsEncodingAtReplyCap measured 66317 allocated bytes/op, below its existing bound. Candidate escape expansion/eight-item cap, empty/small exact JSON, and exact reply boundary/overflow tests passed.
- `go vet ./internal/agent` exited 0 with no diagnostics.
- `git diff --check -- internal/agent/commands.go internal/agent/agent_test.go docs/superpowers/reports/2026-10-09-memory-integration.md` exited 0 with no diagnostics.

Owned source is frozen again for root re-review. No staging, commit, network request, private data access, or delegation occurred.
