# Scoped Management Storage Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans. Implement and review this bounded storage task before the controller uses it.

**Goal:** Expose scoped paginated administration and a complete bounded memory JSON export without materializing all retained chats.

**Architecture:** Concrete Store APIs use existing admission, scoped predicates and one-connection transaction helpers. Export writes to an application-created private file before HTTP transfer; HTTP never holds a database lease while downloading.

**Tech Stack:** Go1.27.2, existing SQLite, no dependency.

**Spec:** docs/superpowers/specs/2026-10-09-core-web-design.md; REQUIREMENTS.md searchable retained history and memory CRUD/export.

## Global Constraints
- Only GPT-6.1 Medium/Low children; no child delegation, actual private data, live calls, schema/quota changes or automatic data pruning.
- Fixed configured account/user scope comes from Core, never HTTP/model-selected scope.
- One SQLite connection; helpers accept rowQuery and never call public Store methods while holding their transaction.
- Existing message claims/state remain intact. History pages show completed nonempty chats; clear erases content and retains duplicate suppression.
- All export writes are cancelable and bounded; no detached producer or in-memory whole-export buffer.

## Review Focus
- Foreign-scope IDs/search terms never expose another user's records.
- Tiny limit with large retained data performs bounded row materialization, not ExportFacts/all-message loading followed by slicing.
- LIKE/FTS syntax from hostile query is escaped and parameter-bound.
- Writer failure or cancellation produces a safe error; an incomplete private artifact is never offered as completed JSON.
- Concurrent clear/confirmation uses captured export transaction; no inconsistent fact/summary/candidate combination.

## Task1: Administrative page and export APIs — Medium
**Files:** create internal/storage/management.go and management_test.go; modify memory.go only to factor existing fact SQL/row decoding helpers; modify messages.go only for shared completed-row decoding if necessary. Report docs/superpowers/reports/2026-10-09-management-storage.md. Wait until Memory Task3 is reviewed; no concurrent storage ownership.

**Interfaces:**
- `FactPage{Items []Fact; NextOffset int; HasMore bool}`; `FactsPage(ctx context.Context,scope Scope,query string,offset,limit int)(FactPage,error)`. Query matches reviewed SearchFacts semantics but includes expired retained records for administrator correction/deletion; order importance DESC,updated_at DESC,id DESC. Offset0..10000, limit1..16. Fetch limit+1 and remove lookahead; HasMore/NextOffset meaningful only for that captured request. Concurrent mutations can shift offset pages; document ordinary pagination, not a cross-request immutable snapshot.
- `HistoryEntry{ID,Content,Reply string; CreatedAt time.Time}`; `HistoryPage{Items []HistoryEntry; NextOffset int; HasMore bool}`; `HistoryPage(ctx context.Context,scope Scope,query string,offset,limit int)(HistoryPage,error)` method/type same spelling is legal. Query valid UTF8,1024bytes/32 terms as facts; literal trimmed substring in content OR reply, with escaped LIKE. Completed state=sent and content/reply nonempty. Order created_at DESC,id DESC; offset0..10000,limit1..8; pre-admission legacy shape before materializing at most9 rows. Return original IDs only through authenticated administration later; never logging them.
- `WriteMemoryJSON(ctx context.Context,scope Scope,w io.Writer)error`: complete object `{version:1,facts:[...],summary:{...},candidates:[...]}` with stable public field names and standard JSON escaping. Includes all retained facts, including expired; current summary and all scoped candidates. Raw receive evidence/tokens/credentials/channel metadata excluded. Historical chats use the separate paginated history view and are clearly labeled separately in UI. One read transaction captures derived data and facts; validate legacy/derived shape/caps before decoding. Encode one fact/candidate at a time into bounded per-row scratch; never []all facts or all-output buffer. Check context between rows and writer errors; cap total written64MiB before attempting beyond cap. Output is valid only on nil return; caller removes incomplete private file. No arbitrary writer error text escapes the storage sentinel contract.

- [ ] Write meaningful RED tests named TestManagementScopeIsolation, TestManagementPageMaterializationBound, TestManagementSearchEscapes, TestMemoryExportCompleteAndConsistent, TestMemoryExportWriterFailureAndCancel. Compare complete decoded export with scoped stored facts/summary/candidates; exceed chat16KiB but remain valid. Use quote/control-character expansion, expired facts, lookahead pagination, hostile terms, foreign scope, clear and canceled writer fixtures. Measure bounded page allocations against many retained large messages/facts to prove the code does not load all rows.
- [ ] Run focused tests to observe missing APIs/behavior before implementation.
- [ ] Factor reusable fact predicate/order and single-row decoder rather than copying the reviewed FTS policy. Implement exact bounded interfaces in the owned files, retaining old SearchFacts/ExportFacts behavior and all schema manifests.
- [ ] Run `go test ./internal/storage ./internal/agent ./internal/summary ./internal/cli` and corresponding vet using verified Go1.27.2/workspace cache. Report RED/GREEN, allocation result, changed files and limitations; freeze for fresh Medium review/root full integration. Root owns commit/scans.

## Root gate/self-review
The task covers memory pagination/export and searchable completed history without changing SQLite or retention. Core/Web/maintenance/CLI/session/UI are separate plans. Every Review Focus condition has a named test above. Root checks actual SQL scope, snapshot and resource behavior and performs a fresh Medium spec/quality review before controller adoption. User standing autonomous implementation instruction supplies execution method; no additional routine permission gate.

Root preflight before dispatch: the planned complete export cap is64MiB, replacing the earlier32MiB proposal. Current valid scope content can be4MiB and categories can total10000*256bytes; legal ASCII control characters expand up to6x in JSON. Including bounded row metadata/candidates/summary can therefore exceed32MiB without violating storage quotas.64MiB is a completed private streamed-file ceiling, not an in-memory allocation or changed512MB/RSS target. No retention/schema/provider change. Add a bulk synthetic control-heavy fact/category export regression proving complete valid output above32MiB while writer observes one-row writes and never retains the whole artifact in RAM. Root updates downstream Core download ceiling to match before implementation.
