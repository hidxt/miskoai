# Text agent Task1 implementation report

2026-10-09. Fresh GPT-6.1 Medium implementation; primary owns integration and independent review. Synthetic Windows-host evidence only. No live provider/WeChat request, private credential/data access, actual service start, delegation, commit or push.

## Owned files

- `internal/agent/agent.go`: exact injected Model/Searcher/Sender interfaces, Incoming/Options/Result, fixed scope validation, cancelable single-slot admission, total 90-second work deadline, durable claims/send states, safe result codes/errors, stable JSON-tuple SHA-256 client IDs, UTF-8/control validation and rune-boundary output limit.
- `internal/agent/context.go`: safety/current preservation, 32 history turns/16 whole pairs, oldest-pair eviction under byte and estimated-token caps, at most eight quoted fact entries and three quoted sources in user-role data, bounded fact-query derivation.
- `internal/agent/commands.go`: explicit remember/query/forget/clear/export memory operations; exact supplied fact content and explicit_user/confidence1/importance50; explicit one-query search routing.
- `internal/agent/profiles.go`: warm/concise/professional expression with common immutable safety instruction, including no false human identity or invented personal experiences.
- `internal/agent/agent_test.go` and `context_test.go`: synthetic private-child SQLite fixtures and counted injected fakes; no actual credentials.
- This report. No other product/document files edited by this implementer.

## Behavior and decisions

Authorization and input envelope bounds precede admission, claim, model and tools. Account/user are fixed at New. Claims in every state return duplicate without model/search/send or memory side effects. Ordinary text uses the 8KiB model-input cap; deterministic memory commands use the general 16KiB storage input cap. Nil Handle context rejects safely.

Options defaults are max output512, context bytes24576, estimated tokens32768; output range1–4096, context bytes16384–49152 and estimated tokens4096–65536. The estimate is UTF-8 content bytes plus32 per message, not provider token accounting. Actual provider Usage is returned separately. Current input and fixed policy remain mandatory; an impossible mandatory budget returns a fixed limit notice without a model call. Optional retrieval follows pair eviction. Provider message-count40/text64KiB bounds remain satisfied.

The single-slot channel is a cancelable admission gate rather than a mutex: the work deadline includes waiting, and waiting cancellation never claims a message. The future runtime is responsible for its bounded worker/wake queue; this package does not spawn per-message goroutines or start a service.

Before the one channel send, sending state is durable. Any send error, cancellation during/after the call, or acknowledgment-persistence failure returns ambiguous and attempts a state write; existing sending/ambiguous claims remain duplicate after restart even when cleanup cannot persist. Cancellation before sending marks failed. Only canceled-state cleanup detaches cancellation, with a2-second bound and exclusively a storage state write. No network operation is initiated from cleanup.

Memory commands never invoke model or search. Successful exports are whole JSON arrays at most16KiB; over-cap exports return a clear limit notice and explicitly say full management export is a future unavailable feature. Export encoding uses a result buffer capped at16KiB and marshals one fact at a time, stopping before the next record and closing bracket would exceed the cap. Empty exports are `[]`. Store content≤16KiB, category≤256bytes and fixed/bounded scalar metadata bound one encoded fact below128KiB even with sixfold JSON escaping; the whole fact slice is never marshaled. No destructive action is recommended just to export. Explicit facts are scoped; model guesses are never persisted as facts.

Search runs only on `/search QUERY` or `搜索：QUERY`, one injected search call, query≤512bytes and max3 results. The agent additionally checks SafeSourceURL, valid text, URL≤2048bytes and rune-bounds title256/content1600 before using results. Results are quoted JSON user-role data; actual accepted titles/URLs are appended, with citation space reserved before model-text truncation. There is no page fetch or model-driven tool execution. Nil optional Searcher and search/model failures get safe fixed replies.

Ruling: use the existing concrete storage APIs and fixed single-agent scope, with no new storage seam — the plan explicitly fixes New to *storage.Store and storage2 already supplies reply reservations — cost if wrong is local seam refactoring during root integration, no external effect.

Ruling: deterministic memory query results show at most8 scoped records and truncate individual display content to1600bytes — query listing should stay comfortably below the16KiB channel bound and complete export remains available only while it fits — cost is shortened long fact display, while stored content is unchanged.

Ruling: retain storage-bounded scoped ExportFacts but encode incrementally per fact into a≤16KiB result buffer — primary identified full-slice escaped-JSON allocation as a resource concern — cost is a limit notice as soon as the next whole encoded record cannot fit, with no memory deletion or new network action.

## Actual RED/GREEN commands and results

Every Go invocation used workspace paths: GOPATH `.tools/go`, GOMODCACHE `.tools/gomod`, GOCACHE `.tools/gocache`, TEMP/TMP `.tools/tmp`.

1. Tests were written before implementation. `go test ./internal/agent -count=1` exited1 with expected undefined Agent/New/Options/Incoming, confirming the absent package surface.
2. Minimal no-op compile scaffolds were added. `go test ./internal/agent -count=1 -run 'TestAuthorizedChatAndDuplicate|TestScopeRejectsBeforeSideEffects|TestAmbiguousSendNeverReplayedAfterRestart|TestCancellationBeforeAndDuringSend|TestReplyBounds|TestContextDropsWholeOldPairsAndTreatsFactsAsData|TestEstimatedTokenBudgetDropsPairs|TestExplicitMemoryNoModel|TestSearchOnlyOnUserCommand|TestMemoryExportOverCapIsNotice|TestSafeModelAndSearchFailures|TestOrdinaryInputLimitAndLongMemory|TestLongOrdinaryFactQueryBounded|TestNilSearcherAndOptions|TestFactQueryCapsTermsAndBytes'` exited1 with behavioral failures for all selected named tests (2.508s). The queue-blocking test was excluded from this no-op scaffold run because its fake waits for a model call.
3. Implementation: `gofmt -w internal/agent`; `go test ./internal/agent -count=1` exited0, package2.794s.
4. Stronger acceptance coverage added: all existing claim states, cross-scope same IDs/facts, real admission-wait deadline, acknowledgment-storage failure, source/query bounds and tuple identity. One test composite-literal syntax error was corrected. The expanded suite passed3.178s. Agent vet then identified eight unkeyed external Scope literals in tests; converted them to keyed literals and reran `go vet ./internal/agent`, exit0.
5. Primary refinements were tested before fixing: export non-destructive notice/honest-profile tests exited1 (1.330s); nil-context test exited1 with the expected context nil-parent panic (0.159s). Implemented focused fixes.
6. Final fresh `go test ./internal/agent -count=1 -v` exited0:25 top-level tests,12 subtests, package3.399s. All plan-named tests ran. `go vet ./internal/agent` exited0 with no output.
7. `git diff --check` exited0; `git status --short` at inspection showed only untracked internal/agent before the report was added. No broad staging or commit was performed.
8. Primary's resource refinement: added `TestEscapedExportStopsEncodingAtReplyCap` and `TestSmallAndEmptyExportsAreExactJSON`; a whole-slice helper implementation produced meaningful RED (`go test ./internal/agent -count=1 -run 'TestEscapedExportStopsEncodingAtReplyCap|TestSmallAndEmptyExportsAreExactJSON'`, exit1,3.279s). Synthetic128fact escaping fixture measured7338415allocated bytes/op, exceeding a generous512KiB regression ceiling. Replaced the helper with per-fact incremental encoding and wired `/export-memory` to it; focused GREEN passed3.192s, full agent suite passed3.189s, agent vet exited0. A final exact16KiB/one-byte-overflow closing-bracket regression was added before final freeze; final results follow below.
9. Final fresh export-refinement verification: `go test ./internal/agent -count=1 -v` exited0,28top-level tests and12subtests passed, package4.351s. `TestExportExactReplyBoundaryAndOverflow` passed for an exact16384byte valid JSON array and rejected a one-byte-larger array without returning a partial payload. Escaping-fixture measurement was66082allocated bytes/op (versus7338415in the whole-slice RED). `go vet ./internal/agent` exited0 with no output. Files frozen again after this report update.

## Remaining limitations / root review focus

- Root must inspect the final code, rerun integrated checks against storage2 and request fresh independent review. This report is implementation evidence, not release acceptance.
- No native Linux/race check was run by this implementer. Windows race requires the previously absent gcc. Real provider/WeChat and512MB/VPS tests were deliberately not run.
- ExportFacts still materializes the scoped fact set, bounded by storage2 at10000facts/4MiB per scope. Encoding now retains only a≤16KiB result buffer and one individually bounded encoded fact, stopping at the cap. The regression measures Go allocation bytes for an escaping fixture, not process RSS or512MB/VPS acceptance. A later full management export should use its own bounded export path.
- Detached state cleanup can fail if storage is unavailable; safety derives from the already persisted claim/sending state, so replay is still suppressed. Owner reconciliation/runtime inbox completion remains outside Task1.
- Safety/profile text expresses the intended honest identity and privacy rules but cannot prove model obedience. The model has no execution/tool authorization capability in this package.
- No summaries, persistent custom profiles, authenticated Web, poll worker/runtime lifecycle or actual chat service were added.

Files frozen for primary integration/review after this report.
