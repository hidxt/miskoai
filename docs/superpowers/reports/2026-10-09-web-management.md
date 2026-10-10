# Fixed-scope authenticated management Task2 author report

2026-10-10. Implementer: GPT-6.1 Sol Medium (`gpt-6.1-sol`, `reasoning_effort=medium`). Whole-project engineering progress remains **52%** per `docs/superpowers/project-progress.md`. Task2 implementation and assigned local verification are complete and frozen for root integration and a fresh separate Medium review. This author report does not establish independent acceptance, new native Linux CI, live provider/WeChat evidence, VPS/RSS performance, or release acceptance.

## Ownership and dependencies

New owned files: `internal/web/application.go`, `memory.go`, `settings.go`, `system.go`, `application_test.go`, `memory_test.go`, `system_test.go`, and this report. Root additionally transferred narrow Task2 ownership of `internal/web/json.go` and `json_test.go` after the implementer proposed the concrete shared-parser interface. No other source was edited by this implementer.

Root-owned integration dependencies are the working `internal/web/server.go` and `server_test.go` recovery refinement: rethrow exactly `http.ErrAbortHandler`, while ordinary panics still receive the existing safe response. Root separately reported isolated accepted-HEAD runtime RED (two failing tests), then 19 test/subtest passes, zero failures/skips and vet exit 0. Both root-owned files must accompany this Task2 source in the review/integration export, but are not Task2 author ownership.

Read the task brief first, AGENTS.md and its six mandatory policy/state documents. Scope remains the approved fixed Core scope. The inherited public-transport constructor and Core/config/storage/maintenance sources were not edited. ActiveProfile already existed and was reused. No child delegation, Git staging/commit/push, global configuration, actual credentials/databases, cloud requests, real WeChat requests, QR/service startup, or actual-data maintenance occurred. HTTP integration servers below are dedicated synthetic `httptest` loopback fixtures only.

## Implementation and safe boundaries

`Application(*core.Controller) http.Handler` installs the management adapter behind the accepted `Server.Handler` authentication, Host/Origin and CSRF checks. Its private backend/download seams are supplied once at construction; production forwards only to the existing Controller. The adapter imports storage DTO types, never opens a Store or executes SQL. All account/user scope remains captured by Core, and no Web request can supply it. Tests seed dedicated synthetic private stores before Core takes ownership; fake model/search/channel delegates count and reject every invocation, with final zero-invocation assertions.

Shared `strictRawRequestObject(io.Reader, int64, ...string)` factors only existing complete-object traversal into `map[string]json.RawMessage`. It keeps the pre-materialization byte cap, UTF-8/JSON/surrogate validation, decoded-key duplicate/unknown checks, every required key, and complete EOF. The login string wrapper explicitly requires a quoted JSON string, including refusing null, numbers and compound values; original auth/helper tests remain in final verification. Typed scalar/range validation stays in the management files.

All ordinary JSON responses are marshaled and checked against 2 MiB before response success. Existing Core/storage field and page quotas bound their shapes. Facts have at most 16 rows, history 8, candidates 8, custom profiles 16, and status events 128. A 16-row worst-case escaped fact-page fixture exceeds 1 MiB and stays below 2 MiB; an invalid oversized backend response produces only `web_response`. This is local allocation-bound evidence, not a measured process RSS claim.

All mutation objects are exact, complete and have only the documented required keys. Limits are 128 KiB for fact add/patch, 16 KiB settings, 8 KiB profiles, and **1 KiB** for profile selection, candidate ID mutations, clear/resume acknowledgements, and export/backup empty objects. The fact cap accommodates existing 16 KiB content plus 256-byte category at six-byte JSON escaping without enlarging stored quotas. DELETE accepts no body; known nonempty bodies are rejected before mutation and unknown-length bodies are checked with at most one byte. Queries allow only unique `q`/`offset`/`limit` or unique `id` on applicable routes; other routes accept no query names. Raw query is capped at 8 KiB, q at 1024 UTF-8 bytes, offset at canonical decimal 0..10000, fact limit 1..16/default16, history limit 1..8/default8. No float, sign, leading-zero numeric alias, overflow, null-as-integer, duplicate or hostile scope field is coerced.

Fact/candidate interactive IDs are canonical positive decimal strings, parsed losslessly into signed int64 through MaxInt64. Numeric JSON IDs and MaxInt64+1 are refused. History/profile IDs retain their original strings. Fact expiry is explicit null or fixed-width RFC3339 with valid calendar fields, year1..9999, valid timezone offsets and at most nine fractional digits; timestamp input is at most64 bytes. Nanosecond precision prevents Go's silent fractional truncation. Responses use UTC RFC3339Nano timestamps and nullable expiry. Strings, including script/HTML/profile content, stay escaped JSON data for later UI textContent rendering.

Downloads obtain completed Core-owned private artifacts and stream through Read/Close without buffering a whole file or holding a database lease. Filename/size must match fixed `miskoai-memory.json`/64 MiB or `miskoai-backup.db`/256 MiB. Content-Disposition contains only that fixed basename; MIME and Content-Length are fixed from the completed artifact. Known cancellation before headers is a safe error; read/write/Close/context failure after explicit response commitment closes and rethrows the abort sentinel. Cleanup is synchronous with the handler. Tests cover Close on success, pre-cancellation, read/write/close errors, opaque real Core export, allowance release, HTTP truncation, and actual client cancellation with held-read lifetime evidence.

Restore validates method/MIME before reading, uses `http.MaxBytesReader` at256 MiB (at most one overflow probe byte), and passes a context-aware reader to Core.Restore. It never stages/reopens/install SQL itself. Invalid restore preserves the current database; valid real synthetic restore pauses the channel and reports only the two public booleans. Explicit reconciliation acknowledgement is separate. Existing request contexts remain10s, backup/restore30s, server transport read15s/write35s; this patch adds no route deadline override. A slow upload may be refused by the15s transport boundary before the30s Core context. Cancellation cannot magically unblock an arbitrary reader; actual reader/handler return remains the lifetime join.

Root compatibility ruling (D040/D041): the version-1 memory export is an **opaque downloadable artifact** copied byte-for-byte from accepted Core/storage. Its fact/candidate IDs remain lossless numeric JSON tokens in that artifact. Interactive Web DTOs and commands all use decimal-string IDs. Browser code must never parse artifact IDs into JavaScript Number. No storage artifact version or schema was changed.

## Stable interactive JSON contract

Every key below is snake_case. Mutations normally return `{ "ok": true }`, fact add/candidate confirm return `{ "id": "decimal" }`, and errors only `{ "error": "fixed_code" }`.

| Route | Request / response keys |
|---|---|
| GET `/api/settings` | `desired`, `effective`, `overrides`, `restart_required`. Each Settings object has `listen`, `deepseek_url`, `model`, `vision_model`, `weixin_url`, `max_output_tokens`, `context_bytes`, `context_tokens`. `restart_required` is desired != effective, never a live reload claim. Overrides are the safe Core boolean map. |
| POST `/api/settings` | Exactly the eight Settings keys above, typed strings and exact bounded integers; Core persists desired settings only. Never Config or credential serialization. |
| GET `/api/profiles` | `custom`, `builtin_ids`, `active`. Profile keys: `id`, `name`, `description`, `style`, `address`, `length`, `sticker`, `humor`. Builtin IDs exactly `warm`, `concise`, `professional`; default active `warm`. |
| POST/DELETE/select profiles | POST exact Profile object; DELETE unique query `id`; select exact `{ "id": "profile_id" }`. Core enforces custom-profile field quotas/enums; length short/normal/detailed, sticker off/low/normal, humor exact0..3. Sticker is expression preference only. |
| GET `/api/facts` | `items`, `next_offset`, `has_more`. Fact row: `id` string, `content`, `category`, `source`, `confidence`, `importance`, `created_at`, `updated_at`, `expires_at`. |
| POST/PATCH/DELETE facts | Add exact `content`, `category`, `importance`, `expires_at`; patch adds `id` string and provides all editable fields; DELETE unique query `id` string. No supplied source/confidence/account/user. Core assigns explicit_user/1. |
| GET `/api/history` | `items`, `next_offset`, `has_more`; row `id` string, `content`, `reply`, `created_at`. |
| GET `/api/memory` | `derived`, `candidates`; derived `text`, `watermark` string, `revision` string, `updated_at`; candidate `id` string, `content`, `message_id` string, `quote`, `created_at`. Provenance is genuine retained Core data. |
| Confirm/reject candidates | Exact `{ "id": "positive_decimal" }`; confirm returns created fact ID string, reject returns ok. |
| POST `/api/memory/clear` | Exactly `{ "acknowledge": "clear_memory" }`; Core cancels/joins and clears only its scope. |
| POST `/api/export`, `/api/backup` | Exact `{}`; fixed completed streamed download response, separate from2MiB ordinary JSON cap. No supplied paths/filenames. |
| POST `/api/restore` | `application/octet-stream` body <=256MiB; response exactly `prior_retained`, `channel_paused`. No private recovery path. |
| POST `/api/channel/resume` | Exactly `{ "acknowledge": "reconciled_restored_history" }`; separate explicit action, never invoked at startup. |
| GET `/api/diagnostics` | `product`, `go_version`, `os`, `architecture`, `external_probe` false, `status`. No external probe, database reopening or implicit cloud/QR action. |

GET `/api/status` (and diagnostics `status`) keys: `uptime_seconds`, `heap_bytes`, `goroutines`, `gc_count`, `rss_available` false, `channel_state`, `has_deepseek_key`, `has_ollama_key`, `channel`, `summary`, `logical_attempts`, `successful_usage`, `events`. Heap is explicitly heap, not RSS. Logical attempts are delegate operations, not HTTP or billing counts.

Nested channel keys: `started_at`, `state`, `received`, `processed`, `sent`, `duplicate`, `failed`, `ambiguous`, `last_code`. Summary: `completed`, `failed`, `last_code`. Logical attempts: `chat`, `summary`, `vision`, `search`. Successful usage: `prompt_tokens`, `completion_tokens`, `total_tokens`, `cached_tokens`, `reasoning_tokens`. Event: `kind`, `at`. Unknown state/code values normalize to safe fixed enums; unknown events are dropped. Existing safe code/event values are enumerated in system.go. Scope IDs, private paths, raw errors, key fragments and credentials never enter these DTOs. Unsupported sticker/media routes remain absent.

## Runtime TDD and failures retained honestly

- Initial tool invocation used an incorrect verified-Go binary path and exited1 without running Go; corrected to `.tools/toolchains/go1.27.2-verified/go/bin/go.exe`.
- With route-only404 scaffold, `go test ./internal/web -run 'TestManagement' -count=1` exited1: three management tests failed at missing routes. Missing-route RED is real runtime evidence.
- Initial Core fixture run under the restricted token exited1 with `invalid or unsafe settings` in CRUD/restore setup; this is a Windows ACL fixture-environment failure, not feature RED. Same dedicated fixtures under approved normal-user `require_escalated` reached expected404 CRUD/restore RED. Product ACL policy was never relaxed.
- `go test ./internal/web -run 'TestStrict(Raw)?RequestObject' -count=1`: raw parser scaffold runtimeRED exit1, then shared traversal/string wrapper GREEN exit0, including original string-only helper tests.
- Initial full Web run after routes exited1 because the test incorrectly reused auth `decodeToken` for a one-digit fact ID (`id length1`). Corrected the test to decode an ordinary ID DTO; this was a test-helper error, not missing behavior. Subsequent full Web run exited0.
- `TestRestoreStreamBoundsAndTypes` runtimeRED: unknown-length256MiB+1 produced500; classified MaxBytesError safely, GREEN400. Method/type/known oversize refused before reading and oversized streaming input consumed no more than256MiB+1.
- Transfer regression runtimeRED before root recovery fix: read_error/write_error/close_error confirmed one Close but the sentinel was swallowed. Root's separate two-test RED/GREEN and the integrated author GREEN establish the recovery dependency; no author server edit.
- `TestManagementDeleteRejectsUnexpectedBodies` and `TestFactExpiryRejectsInvalidRFC3339Offsets` runtimeRED exited1: DELETE body mutated and time.Parse accepted invalid offsets, comma fractions or truncated precision. Bounded DELETE-body and strict expiry grammar fixes reached focused GREEN exit0.
- First actual-client cancellation fixture exited1 with Client.Timeout, because net/http ReaderFrom's first512-byte sniff interacted with the artificial partial-read fixture. Read installed Go server.go to identify the buffering assumption; a test-only flushing ResponseWriter corrected the fixture. This is a fixture failure, not production RED. `TestDownloadHTTPClientCancelsDuringTransfer -count=1` then exited0. The earlier canceled-server-context characterization was renamed `TestDownloadHTTPTruncationAndPreCanceledContext`; it is accurately distinguished from actual client cancellation.
- Actual client cancellation now observes4096partial response bytes, cancels the client's request, observes cancellation inside the server download reader, deliberately holds that read, and verifies admission remains active1, joined waiter pending and artifact notclosed. Only after explicit release does the handler join and Close count become exactly1. No detached production reader goroutine exists.

## Final local verification

Verified native Windows Go1.27.2, `GOTOOLCHAIN=local`, `GOPROXY=off`, `GOSUMDB=off`, `GOMODCACHE=<root>/.tools/gomod`, `GOCACHE=<root>/.tools/gocache`, TEMP/TMP/GOTMPDIR=`<root>/.tools/tmp`. Dedicated synthetic private fixtures use approved normal-user token (`require_escalated`). No cache/environment/global configuration was changed outside command processes.

Exact final commands (verified executable path above):

```text
go test -json ./internal/web ./internal/core ./internal/maintenance ./internal/storage ./internal/config -count=1
go vet ./internal/web ./internal/core ./internal/maintenance ./internal/storage ./internal/config
git diff --check -- internal/web/application.go internal/web/application_test.go internal/web/memory.go internal/web/memory_test.go internal/web/settings.go internal/web/system.go internal/web/system_test.go internal/web/json.go internal/web/json_test.go
```

Tests: **exit0;439 test/subtest pass events;0 failures;2 explicit platform skips;5 package passes**. Breakdown: Web60/Core46/Maintenance60/Storage203/Config70. Counting excludes the five package-level pass events. Raw completed evidence: `.tools/reviews/web-management-final-tests.jsonl`, including untouched auth tests and root abort tests. Skips: `TestLockRejectsMovedDirectory` (Windows prohibits moving a directory with a live lock handle; native Linux directory-identity evidence is separate) and `TestExistingParentPermissionsArePreserved` (Unix permission bits require native Unix execution).

Scoped vet: **exit0, no diagnostic output**. The output pipeline produced no file for an empty stream; the follow-up Get-Content error was an artifact-display issue and the artifact was then created empty at `.tools/reviews/web-management-final-vet.txt`. Do not misreport that shell display error as vet failure. Diff whitespace check: exit0. A progress inspection briefly read an incomplete final JSONL line while the test process still wrote it; completed-file parsing verified the final counts above. No failed Go test is hidden by these tooling distinctions.

Self-review checked method/query/object allowlists, ID losslessness, scope-only Core calls, DTO redaction/snake_case, export compatibility, byte caps, response cap, cooperative cancellation/Close, and restore/reconciliation behavior. No unresolved author blocker was found. Root integrated tests/scans and fresh independent review remain required; no standalone full repository test, race, new native Linux CI, live,512MiB or final acceptance claim is made by this child, as root owns those gates.

## Frozen source SHA256

```text
application.go      53D13A9269173426FF9BC051EB027BCDA99325C8DBE3D0110CF4A30766B8146B
application_test.go 4F2543D39DF1AB8198D989A3A5630950321EFFDB4DE496C166CD09F6F557BA60
memory.go           F1D2A1B88BBBB7C2713270AEA198C50E61565BB6D1F2C91BAC176412EF962A1A
memory_test.go      0F0A9928E2F21B7A6B1168B0477889264061D15E309CF0FDFEF79CB99C9A10F1
settings.go         DA3A7354E2884F63D2B3771D82F4094A793E6C8AFD952F08B342CE6A36D73D7B
system.go           FB2F8B6D5C41B347A3EC574AC3DAD27EB9347B40032DF97E58D190501B2D4153
system_test.go      EFD09A5056A3B1B2B1324ED62CC5FF7656F9DC02198615FED7956804BE900D76
json.go             24F168266DE04053DD3AA5EDBAF18010AD94CCF76851BAF66EB8F954F69EFAA5
json_test.go        201F242FE65D9C84AADDACC3D432695035FC2B3583389EBE7920965C1D1C5690
```

All nine author-owned source/test files and this report are frozen after the final report whitespace check. Root may now read the exact files/diff, capture immutable review export including root abort dependency, run full integrated gates and dispatch fresh separate GPT-6.1 Sol Medium review. No other Task3/UI/CLI/media work was started.

## FIX1 — canonical listen spelling after independent review I1

2026-10-10. This section is the latest status and supersedes the initial freeze's settings.go/application_test.go hashes and local verification counts above; the original evidence remains historical and is preserved in root's immutable export. Implementer remained ONLY GPT-6.1 Sol Medium (`gpt-6.1-sol`, `reasoning_effort=medium`), no delegation. Whole-project engineering progress remains **52%**.

The fresh reviewer returned spec/quality CHANGES REQUIRED with0Critical/1Important/0Minor: persisted settings accepted `127.0.0.1:08787` and `127.0.0.1:+8787` through config's Atoi parsing, but accepted Server.New refuses those spellings on restart. The same problem affected bracketed IPv6. Root independently agreed and authorized the smallest fix in the Web settings route. This is a confirmed production behavior defect that the initial author review and passing integration suite missed, not a test-fixture issue. Root's prior815pass/0fail/4existingplatformskips/vet0 and static checks did not establish correctness of this untested contract.

Changed only `internal/web/settings.go`, `internal/web/application_test.go` and this author report in FIX1. The other seven author-owned source/test files remain unchanged at the initial frozen hashes. Root server/config/Core/SQL/provider sources, root documentation and CI evidence were not edited. No scope, provider, deployment, safety, resource or storage limits changed; no network, live tests, credentials, actual data or Git writes occurred.

The POST settings route now checks the decoded listen value before `SaveSettings`: literal host must be exactly127.0.0.1 or::1, port must be1..65535, `strconv.Itoa(p) == port`, and `net.JoinHostPort(host, port) == listen`. Split/parse errors refuse. This matches accepted Server.New spelling without modifying that constructor or config validation. Canonical IPv4 and bracketed IPv6 still persist only desired settings; effective settings are not hot-reloaded.

Runtime TDD used authenticated `Server.Handler` requests. `TestManagementSettingsCanonicalListenRefusesAliases` covers all four strings (`127.0.0.1:08787`, `127.0.0.1:+8787`, `[::1]:08787`, `[::1]:+8787`) twice: a narrow mutation backend must not be called, and a dedicated synthetic private Core must preserve desired/effective settings and independently loaded persisted Settings after refusal. `TestManagementSettingsCanonicalListenPersistsDesired` accepts `127.0.0.1:18787` and `[::1]:18787`, checks actual desired/persisted equality, unchanged effective settings, and GET settings' true restart_required contract. All private fixtures reuse the previously frozen offline delegates and assert zero external operations.

Exact focused command with the same verified Go1.27.2 executable/offline cache/temp environment and approved normal-user Windows fixture token:

```text
go test -json ./internal/web -run 'TestManagementSettingsCanonicalListen' -count=1
```

RED before production fix: **exit1;3 pass events;9 failed test/subtest events;0 skips**. Eight refusal subcases plus their parent failed: all four noncanonical variants returned200, reached the mutation backend and changed Core desired/persisted settings. Canonical acceptance's two subcases and parent already passed. Evidence: `.tools/reviews/web-management-fix1-red.jsonl`. GREEN after the narrow guard: **exit0;12 passes;0 failures;0 skips**. Evidence: `.tools/reviews/web-management-fix1-green.jsonl`. This round had no compile, fixture or tooling failures to reinterpret as runtimeRED.

Exact final assigned verification commands:

```text
go test -json ./internal/web ./internal/core ./internal/maintenance ./internal/storage ./internal/config -count=1
go vet ./internal/web ./internal/core ./internal/maintenance ./internal/storage ./internal/config
git diff --check -- internal/web/settings.go internal/web/application_test.go docs/superpowers/reports/2026-10-09-web-management.md
```

Final tests: **exit0;451 test/subtest passes;0 failures;2 existing explicit platform skips;5 package passes**. Web72/Core46/Maintenance60/Storage203/Config70; counts exclude the five package-level pass events. The two skip names/reasons are unchanged from the initial assigned run. Raw evidence: `.tools/reviews/web-management-fix1-final-tests.jsonl`. Scoped vet: **exit0, no diagnostics**, empty completed artifact `.tools/reviews/web-management-fix1-final-vet.txt`. Final whitespace check including this FIX1 report: exit0. Test/vet ran independently against unchanged source; no author source change followed these checks.

Self-review compared the new route guard directly with accepted Server.New's literal host/port/canonical checks, verified it precedes any SaveSettings call, read the refusal/persistence assertions, and confirmed the seven other source hashes unchanged. I1 is addressed by implementation and local regression evidence; independent scoped re-review is still pending and no approval verdict is claimed by this author.

Updated SHA256 values:

```text
application_test.go BD47093C97C34DE898A1A3ADCEED3A6F8876BD3F0366ED35A3CC17733F394ABC
settings.go         9904334D717DE9A5AE4FB4284712D6CD2DCDDCF95100060DBB7FDA271A6F3210
```

All nine author-owned source/test files plus this updated report are now re-frozen for root's exact-source integration and the **same reviewer's scoped FIX1 re-review**. Root-owned abort files remain integration dependencies. The prerequisite8232e9c27279162819f12c692004330e5b910207 nativeCI38025850451 passed as verified by root; it excludes Task2's working API/JSON/root-abort changes. Task2's new native CI, any live/provider/WeChat/VPS evidence and final acceptance remain separate pending root gates. No unresolved author blocker remains after FIX1; the accepted response/upload/export and desired/effective limitations recorded above are unchanged.
