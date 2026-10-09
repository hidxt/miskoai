# Scoped management storage Task1 report

Implemented the isolated `.tools/reviews/management-storage-brief.md` against reviewed runtime-config base `a3f6e9a72cea1423b61fd300bccfcc689f0eb0b9`. Implementation is ready for the root's fresh Medium review and full integration; this report does not establish final acceptance.

## Changes and compatibility

- Added `internal/storage/management.go`: `FactPage`, `FactsPage`, `HistoryEntry`, `HistoryPage`, and `WriteMemoryJSON`.
- Added `internal/storage/management_test.go`: synthetic scope/search/pagination/snapshot/writer/admission/allocation/large-export coverage.
- Modified `internal/storage/memory.go` only to share the existing scoped fact predicate, order and single-row decoder. Existing conversational expiry, quoted FTS terms, literal keyword fallback, ordering, limit validation and `ExportFacts` behavior remain intact. No `messages.go` change was required.

Fact pages accept offset0..10000/limit1..16 and include expired retained records. History pages accept offset0..10000/limit1..8 and return only completed nonempty pairs, with literal escaped trimmed substring matching in content or reply. Both pages validate legacy shape before decoding, capture one transaction, select at most limit+1 identifiers before loading payloads, and use descending documented order. Both inner and outer queries bind configured account/user scope. NextOffset is offset+limit only when HasMore is true; otherwise zero. Subsequent requests are ordinary offset pagination and can shift after mutations.

Memory export uses one read transaction and one encoded row at a time, with SQL shape/global/scoped quota checks before fact decoding plus existing derived admission. It does not call `ExportFacts`, `History`, or other public Store methods while holding that transaction and never loads retained chats. Export fields are explicit stable lowercase/snake_case JSON names: top-level `version`, `facts`, `summary`, `candidates`; facts expose `id`, `content`, `category`, `source`, `confidence`, `importance`, `created_at`, `updated_at`, `expires_at`; summary exposes `text`, `watermark`, `revision`, `updated_at`; candidates expose `id`, `content`, `message_id`, `quote`, `created_at`. Candidate quote/source identity is the stored reviewable candidate, not raw receive evidence. Historical chats are available separately through HistoryPage. No account/user, raw frame, channel metadata, token or credential is exported.

The private streamed artifact ceiling is64MiB. Before any write, the writer checks cancellation and whether that complete row/framing write would exceed the remaining ceiling. Arbitrary writer errors and short writes become `ErrStorage` (or the context error when canceled). JSON encoding errors become `ErrInvalid`. Only nil return establishes a completed artifact; callers must discard partial output on error. A synchronous arbitrary `io.Writer` must itself honor cancellation while blocked; storage checks immediately before and after writes and between rows, with no detached producer. No schema, migration, quota, provider, storage-policy or resource-target changes.

## RED and verification

Used TDD and inline execution skills. Missing API signatures initially returned `ErrStorage`; the first focused runtime run failed all six initial named tests at their behavior assertions: scope/page, escaped search, complete captured export, actual writer invocation/error/cancel, page allocation fixture and bulk expansion export. This was runtime RED, not compilation failure. Log: `.tools/reviews/management-storage-red.txt`.

All native Go invocations used `.tools/toolchains/go1.27.2-verified/go/bin/go.exe`; `go version` returned `go version go1.27.2 windows/amd64`. Environment: `GOTOOLCHAIN=local`, `GOPROXY=off`, `GOSUMDB=off`, workspace-resolved GOPATH `.tools/go`, GOMODCACHE `.tools/gomod`, GOCACHE `.tools/gocache`, GOTMPDIR/TEMP/TMP `.tools/tmp`. No downloads or external requests.

Commands and actual results:

```powershell
& .tools/toolchains/go1.27.2-verified/go/bin/go.exe test ./internal/storage -run 'TestManagement|TestMemoryExport' -count=1 -v
# Initial missing-API run: FAIL, all six initial tests fail.
# Final focused run: PASS, 9 top-level tests + 3 admission/ceiling subtests, 8.108s.
& .tools/toolchains/go1.27.2-verified/go/bin/gofmt.exe -w internal/storage/management.go internal/storage/management_test.go internal/storage/memory.go
& .tools/toolchains/go1.27.2-verified/go/bin/go.exe test ./internal/storage ./internal/agent ./internal/summary ./internal/cli -json -count=1
# Exit0:293 passing test/subtest entries,0failures,1platform skip.
& .tools/toolchains/go1.27.2-verified/go/bin/go.exe vet ./internal/storage ./internal/agent ./internal/summary ./internal/cli
# Exit0, no diagnostics.
git diff --check -- internal/storage/memory.go
# Exit0, no diagnostics.
```

Integrated package elapsed times: storage95.536s, agent20.003s, summary17.203s, CLI14.680s. Only skip: `TestExistingParentPermissionsArePreserved`, because Unix permission bits require native Unix execution. JSON log: `.tools/reviews/management-storage-tests.json`; focused log: `.tools/reviews/management-storage-green.txt`.

Tests compare every exported fact/summary/candidate field with scoped stored values, validate expired records and aggregate output exceeding16KiB, prove captured clear and candidate-confirmation consistency, check foreign account/user search isolation, compare existing SearchFacts semantics, exercise hostile literal/FTS input and input bounds, verify completed-state/tie ordering/lookahead/terminal pages, reject oversized legacy/scoped quota before writing, and assert no writer invocation beyond the exact64MiB ceiling. Writer failure/short write/cancellation are classified safely and never return incomplete JSON success.

## Allocation and expansion evidence

The initial256KiB-over-small-fixture allocation assertion failed. Investigation showed SQL admission scanning retained pages incurs modernc/SQLite allocation even though Go decodes only bounded page rows. Added identifier-only limited selection to keep tied sorting from retaining full payloads. The test now records compact and large retained controls and uses measured ceilings materially below whole-payload materialization; it does **not** claim constant total allocations.

Final integrated page measurements, averaging four operations after warmup/GC:

| Page | One large retained row | Many compact rows | Same many rows with large payloads |
|---|---:|---:|---:|
| Facts limit1 |25,992 B/op|10,312 B/op,250 rows|583,484 B/op,4,000,000 content bytes|
| History limit1 |41,776 B/op|9,292 B/op,1000 rows|1,365,828 B/op,32,000,000 content+reply bytes|

The test ceilings are1MiB/page for facts and3MiB/page for history. SQL scans still visit retained pages; the bound demonstrated here is bounded payload materialization and sub-whole-retention allocation, not O(1) total admission cost or RSS acceptance.

The control-heavy legal10000-fact fixture contains4,190,000 content bytes and2,560,000 category bytes. Complete JSON streamed **42,299,018 bytes**, exceeding32MiB while remaining under64MiB. The counting writer retained no whole artifact, observed maximum individual write **4,230 bytes**, and independently decoded/validated each of10000 complete fact rows plus fixed framing. This is synthetic local output evidence, not a filesystem/HTTP/VPS transfer test.

## Self-review and remaining gates

Read the production diff and new files. Checked both scoped predicates, parameter binding and escaped query paths; SQL payload limit+1; compatible fact retrieval; transaction-only snapshot reads; row-scratch JSON encoding; capacity checks before writes; context/sentinel handling; empty arrays/default summary; no raw-chat export; unchanged manifests/quotas. Preserved LICENSE and existing content. No other agent-owned files edited, no actual private DB/chats/credentials accessed, no staging/commit/push, no network/live tests or delegation.

Root owns the fresh Medium spec/quality review, full integrated suite, source/history scans and commit. Native CI for this new uncommitted task is pending; the pushed runtime-config base has separate CI status. Live provider/real WeChat and native Linux/arm64 runtime evidence were not exercised here. Actual512MB/VPS measurements and full product acceptance remain open.
