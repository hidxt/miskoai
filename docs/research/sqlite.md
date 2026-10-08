# SQLite proof of concept

2026-10-09. Implementation: `internal/storage`. Synthetic local data only.

## Pinned dependency and license

`modernc.org/sqlite v1.60.1` is pinned by the primary in the Go module. Downloaded Go module metadata identifies tag commit `b122d0417c01508beb55158faedeb64a0c5bfd8a`, timestamp `2026-09-29T08:41:38Z`, origin <https://gitlab.com/cznic/sqlite>. Checksum verification remains enabled. The downloaded driver `LICENSE` is BSD-3-Clause: binary distributions must carry its notice. SQLite itself is public domain; transitive Go dependencies still require the primary's complete distribution license inventory.

The driver has Go source for Linux amd64/arm64 and Windows amd64. Source/build availability is not native Linux execution evidence. Runtime FTS5 tests, cross-build results and Windows timing are recorded below once commands finish.

## Storage contract

`Open(path)`, `Close`, scoped `AddFact`, `UpdateFact`, `DeleteFact`, `ClearFacts`, `SearchFacts`, `ExportFacts`, `Backup(ctx,dest)` and `Integrity(ctx)` are implemented. `Scope{Account,User}` must be nonempty, valid UTF-8 and at most 256 bytes per field. Every fact SQL read/write applies both fields. Handlers must derive scope from authorization, never from model text.

Fact source must be `explicit_user` or `manual`; guesses/retrieved documents cannot enter confirmed facts under `model` or `external` source. Trusted orchestration must additionally ensure it does not mislabel a model guess as `manual`. Confidence is finite 0–1, importance 0–100. Fact content is limited to 16KiB and category to 256 bytes; search input is at most 1024 bytes/32 terms and return limit 1–1000. A scope has at most 10,000 facts and 4MiB combined content; inserts and updates enforce both budgets within transactions. Thus an export is bounded by 4MiB content plus category/record overhead. Expired facts are hidden from recall, retained in user exports until explicitly removed.

FTS5 is an external-content table maintained by transactional SQL triggers. Query terms are quoted by the application; model/user text never supplies SQL or FTS operators. Literal keyword substring fallback catches Chinese substrings that the default unicode61 tokenizer does not split. LIKE metacharacters are escaped. Recall ranks by importance, update time and ID; BM25, semantic search, automatic summaries and retention are outside this PoC. [SQLite documents its FTS5 tokenizer and external-content trigger requirements](https://sqlite.org/fts5.html).

Schema migration is a transaction with `PRAGMA user_version=1`; a future version is refused. There is one SQL connection, `busy_timeout=5000`, foreign keys enabled, WAL, `synchronous=FULL`, cache target 8MiB and autocheckpoint 1000 pages. SQLite page cache is only one part of process memory, not an RSS guarantee. [SQLite WAL documentation](https://sqlite.org/wal.html) explains its separate WAL/SHM state.

## Durable ingestion and history

`ClaimMessage(ctx,scope,id,text)` inserts `processing` before any remote work; an existing scoped ID returns false in every state. Account and user isolate IDs. `SetMessageState` validates processing→sending/failed and sending→ambiguous/sent; no automatic re-claim of failed, processing, sending or ambiguous records occurs after restart. `CompleteMessage(ctx,scope,id,reply)` atomically persists reply and sending→sent. The caller uses it only after a definite acknowledged send. A crash after sending but before completion remains unresolved, preventing blind retry.

`History(ctx,scope,limit)` returns chronological user/assistant pairs only for completed messages with stored replies; unresolved records cannot become model context. Limit counts completed message pairs (up to 1000, two turns each). Message text/reply each have a 16KiB cap. This is a storage boundary, not a completed channel/pipeline integration or exactly-once external delivery guarantee.

Total message records are capped at 10,000 across scopes. Claim checks the existing scoped ID before the global count, in the same transaction: existing duplicates still return false at capacity; new messages return the safe `ErrCapacity` error. Tombstones are never automatically deleted because deleting them could permit a duplicate send. Administrative archive/retention policy is pending; reaching capacity currently requires stopping new ingestion and explicit maintenance. This is a finite PoC safety bound, not an indefinite-service retention solution.

## Private paths, backup and integrity

Use a dedicated private directory. Newly created directories use 0700; existing parent directory permissions are preserved. On Unix an existing parent with any group/other permission bits is rejected rather than chmodded. Existing/new database files use 0600. Existing final-file symlinks/non-regular files and final-parent symlinks are refused. These checks assume a trusted local owner controls ancestor directories; they do not defeat an attacker concurrently replacing local directory entries. `os.Chmod` does not establish owner-only Windows ACLs: Windows host privacy remains unverified, and real data/credentials must not be used there without ACL setup. Linux permission enforcement still requires native target execution.

Backup reserves a new empty target exclusively at 0600, binds its path as the parameter to `VACUUM INTO ?`, then flushes the completed file. Existing destinations, including the source database itself, are refused. Failure removes the incomplete target on ordinary errors/cancellation; power loss can still leave an incomplete file, so restore must validate it. This does not copy live DB/WAL/SHM files. [SQLite documents VACUUM INTO as a consistent snapshot and permits an existing empty destination](https://sqlite.org/lang_vacuum.html). `Integrity` runs SQLite integrity/foreign-key checks and the FTS external-content integrity check. CLI restore orchestration and filesystem crash recovery belong to separate work and require independent validation.

`ValidateBackup(ctx,path)` must precede `Open` in restore flows. It does not call `Open`, migrate schema, chmod the source or create sidecars. A file URI with `mode=ro&immutable=1` plus query-only mode reads a standalone snapshot; existing WAL/SHM sidecars are refused because immutable reads must not ignore live state. Exact `user_version=1` and a pinned 13-object schema manifest are mandatory: application tables and constraints, columns via full DDL, indexes, synchronization triggers, FTS virtual/shadow tables and autoindex. Extra/missing/altered objects are rejected; future versions require an explicit updated manifest. SQLite `integrity_check` follows schema validation. After copying the accepted snapshot into a private candidate, callers must also run writable `Integrity` to verify the FTS external-content index before replacement. Arbitrary integrity-valid SQLite databases cannot be initialized silently through this validation path.

## Evidence and remaining gates

Test-first initial `go test ./internal/storage` failed on missing `Store`, `Open`, `Scope` and `Fact`. Added completion/history tests failed on missing `CompleteMessage`/`Role` before implementing them. Dependency installation initially lacked `go.mod` entries until the primary completed checksum-verified installation. Native test/benchmark results will be appended after execution.

Native Windows amd64 `go test -v ./internal/storage` passed all seven tests in 10.290s. SQLite runtime version was **3.53.4**, `sqlite_compileoption_used('ENABLE_FTS5')=1`. Actual MATCH queries and FTS trigger update/delete checks passed; a quote-containing backup destination exercised bound path handling, and reopening the backup returned the committed WAL fact and passed database/FTS integrity. Capacity tests first failed with “accepted more than 4MiB fact content” and “unbounded message records”, then passed after transactional checks were implemented. A subsequent test revision also checks update-budget bypass and processing/sending claims after restart.

The subsequent integrated `go test ./...` again passed storage (8.084s), including those revisions, while other concurrently developed packages failed: config had undefined `Load`; CLI init/doctor was not implemented; netx rejected-transition-range tests failed; a provider synthetic PNG had a checksum error. These were returned to the primary rather than changed outside storage ownership. They are not storage acceptance evidence and require integrated re-run by the primary.

Windows N5105 amd64 `go test ./internal/storage -run '^$' -bench BenchmarkChineseRecall -benchmem -benchtime=1s` passed: 100 Chinese scoped facts, 1512 iterations, **762009 ns/op, 23885 B/op, 163 allocs/op**. This measures query timing and Go allocations per operation, not idle/process RSS, large-database performance or VPS resource acceptance.

`go vet ./internal/storage` separately exited 0 with no diagnostics. The primary must independently inspect the files and rerun integrated checks; no commit, staging, push or root document edits were performed by this worker.

Review follow-up: regression tests initially failed on missing `ValidateBackup`. Native Windows validation accepted the snapshot and rejected unrelated schemas at versions 0/1/2, preserving file bytes and creating no WAL/SHM. Tests also cover extra tables, missing/altered triggers, an added fact column and resetting a MiskoAI snapshot to version 0. The existing-directory regression checks rejection of 0755/0750 and acceptance of 0700 without changing any parent mode, but is explicitly skipped on Windows; no native Unix red/green evidence is claimed.

Follow-up `go test -v ./internal/storage` passed eight tests with one explicit Unix-permission skip in 12.255s. A subsequent `go test ./...` passed storage in 10.469s, including the `#`-containing snapshot filename exercising URI escaping; config/netx/provider/weixin passed. CLI `TestBackupRestoreRoundTrip` failed with “cannot preserve existing database: Access is denied”, returned to its primary owner for Windows restore-handle investigation. Follow-up `go vet ./internal/storage` separately exited 0. Root integration acceptance remains its own gate.

All Go commands set local `GOPATH=.tools/go`, `GOMODCACHE=.tools/gomod` and `GOCACHE=.tools/gocache`. No container/WSL or permanent auxiliary runtime is introduced. Windows tests are distinct from live providers, real WeChat, Linux filesystem behavior, target architecture execution and real 2CPU/512MiB RSS gates, all unverified here.
