# Private library content implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development or executing-plans. Root owns integration, independent review and Git; owner authorized continuous routine development.

**Goal:** Add a bounded private content manager over the reviewed metadata/local-image foundations. No package39 credit until later bundle/Core/Web integration.

**Architecture:** One persistent manager admission surrounds synchronous file/metadata operations. It borrows the already shared local image processor and a generation-aware store lease for each call; it retains neither a stale Store nor decoded images. Known immutable files use derived keys; uncertain content is retained and refuses further unsafe writes.

**Tech Stack:** Native Go1.27.2, existing privatefs/attachment/media/SQLite. No new module/runtime/network.

**Spec:** docs/superpowers/specs/2026-10-12-private-library-design.md

## Global constraints

- All children ONLYgpt-6.1-sol/medium; actual private schema2 untouched, realWeChat deferred, paid allowance spent.
-128 global files/rows and64MiB physical content (including retained orphans); each1..4MiB, existing full decoder/format/pixel/frame guards.
- One existing media slot; no independent decoder/admission for library validation. Stream with4096scratch; no whole-file memory/base64 catalogs.
- No arbitrary paths, raw error text, unknown-file deletion, automatic private repair, server/startup/deployment/model/CDN effects.
- Lock order manager admission -> media admission -> store lease. Brief metadata lookup may precede media only when the lease is completely released first.

## Review focus

1. Foreign scope/high ID: scoped metadata lookup refuses before any persistent private content read.
2. Failure between content creation and metadata commit: remove only captured new original; uncertain cleanup consumes physical quota and reports recovery.
3. Duplicate/tombstone/replacement: never overwrite another identity or silently edit state; unknown orphan/reparse/hardlink refuses.
4. Generation/Close races: release actual file/media cleanup before manager admission, join on Close, reacquire Store via callback, never use stale ID/hash across restore.
5. Disabled/missing/corrupt content: no successful lending/enablement/use; no preview/selection before full validation, no false availability claim.

### Task3: Private immutable content manager

**Owned files:** create internal/privatefs/reader.go, reader_test.go, internal/maintenance/sync_export.go, internal/library/library.go, content.go, lifecycle.go, library_test.go, content_test.go, lifecycle_test.go, docs/superpowers/reports/2026-10-12-library-content.md. Existing storage/media/attachment/Core/Web/rootdocs/spec/plan/Git read-only. Own ignored .tools/reviews/library-content-* evidence.

**Interfaces:**
- `privatefs.OpenRead(path string, maxBytes int64) (*os.File,error)`: delegates existing checkedOpen; borrowed read-only handle, caller Close. No policy weakening or new path grammar.
- `maintenance.SyncPrivateDirectory(dir string) error`: wrapper around the existing checked platform syncDirectory; no policy change. Call after new persistent file publication before metadata commit and after successful removal before final tombstone deletion. Existing Windows path verifies privacy only; Linux directory-fsync and actual power-crash are separate evidence.
- `library.Processor` has the exact accepted `WithLocalImage(context.Context,io.Reader,int64,func(attachment.LocalImage,attachment.Artifact) error) error`.
- `library.Lease func(context.Context) (*storage.Store,func(),error)`: trusted generation-aware callback, must return a live Store/release or fixed error. Every returned nonnil release must be called exactly once, including error-with-lease; nil release with nil error is invalid. Manager never retains Store or exposes lease to an untrusted caller.
- `library.New(dataDir string, processor Processor) (*Manager,error)`, `(*Manager).Close() error` idempotent and joined. Constructor ensures a dedicated private `library` child only after admitted parent; no catalog scan or external work on construction.
- `(*Manager).Upload(ctx context.Context, lease Lease, sc storage.Scope, input io.Reader, size int64, meta storage.LibraryMetadata) (storage.LibraryAsset,bool,error)`: validates scope/meta/size before reads; input Enabled must be false. Uses computed image metadata/SHA, scoped duplicate returns original unchanged, physical +SQL quotas before publication/commit. Returned bool means new metadata row.
- `(*Manager).Update(ctx context.Context,lease Lease,sc storage.Scope,id int64,meta storage.LibraryMetadata) error`: real scoped edit; enabling requires actual valid content under existing media admission before storage update. Disabling/editing never needs to delete/repair uncertain files.
- `(*Manager).Delete(ctx context.Context,lease Lease,sc storage.Scope,id int64) error`: scoped lookup, disabled tombstone, verified owned cleanup, then final row delete. Missing tombstone content can finalize; missing ready or changed/untrusted identity reports recovery. No unrelated sweep.
- `(*Manager).WithAsset(ctx context.Context,lease Lease,sc storage.Scope,id int64,requireEnabled bool,consume func(storage.LibraryAsset,attachment.LocalImage,attachment.Artifact) error) error`: before any file read, lookup scoped row and check ready/state. Release lookup lease, safely open persistent bytes, call shared WithLocalImage using a bounded SectionReader, verify actual SHA/format/size/dimensions/frames against row and unchanged original file identity. Reacquire fresh lease and recheck ID/scope/hash/immutable fields before actual consumer, retaining it through consumer return. Callback borrows temporary validated artifact; processor removes it after actual Close. Complete persistent handle Close before manager admission releases. No network/model call supplied by this task.
- `(*Manager).RecordUse(ctx context.Context,lease Lease,sc storage.Scope,id int64,sha256 string,atMillis int64) error`: manager admission +fresh scoped lease, require matching digest before existing RecordLibraryUse, so old restore IDs cannot update a different object. Caller owns confirmed-send semantics; no usage inferred here.
- Fixed errors `ErrInvalid`, `ErrCapacity`, `ErrContent`, `ErrRecovery`, `ErrCleanup`, `ErrClosed`; preserve context cancellation and known storage classes safely, never arbitrary callback/OS wrapper text.

**Concrete decisions:** Content basename is `asset-` +64lowerhex SHA256 of uint32-BE byte-length-prefixed Account, User, then decoded32-byte content SHA256. Target only inside resolved private `dataDir/library`. File payload SHA equals metadata SHA; key is separate scoped identity. Capture original FileInfo through created/open handle and force Windows identity capture with SameFile(info,info). No metadata/request name becomes a path. Full upload copies validated borrowed artifact into a create-exclusive permanent file with4096scratch, Sync/Close and exact identity/size/private checks before AddLibraryAsset. On canceled/failed commit, delete only newly created captured originals after actual Close; return recovery/cleanup when identity changed. Reuse existing content only for a known matching scoped metadata duplicate after full verification; an untracked same-key file is recovery, never overwritten/adopted silently.

Bound global directory enumeration with `ReadDir(129)` (not unbounded os.ReadDir): require at most128 regular, owner-private, single-link, exact generated basenames and actual sizes1..4MiB; total<=64MiB. Build expected keys from bounded trusted LibrarySnapshot, not user input. Unknown entries/orphans are preserved and produce ErrRecovery for mutation admission. Missing/deleting content has explicit errors; no automated startup reconciliation. In-memory captured identity cache<=128 entries holds FileInfo only, never payload; validated reopen after restart can establish a known row's identity only through full hash/size/private validation. Corrupt/replacement ambiguity refuses deletion rather than guessing. Retained objects after a later restore need an explicit bounded recovery contract in the bundle plan; this task cannot invent/sweep that registry.

Own manager lifetime cancels derived operation contexts and stops admitting calls on Close, then waits for the actual admitted operation's reads/consumer/media Close/persistent Close/cleanup to return. Processor and Store are borrowed and never closed. Use one bounded manager token and fixed state/closeDone; no detached I/O or parsing workers. A callback/reader must cooperate and actually return; tests assert held lifetime, not a hard timeout. All read/consumer/file cleanup errors remain fixed classes, with cancellation and cleanup allowed to coexist.

- [ ] Read accepted foundation interfaces/full spec/current ownership; write compiled semantic RED after minimal new API scaffolding, with frozen RED source and actual failures (compile setup alone earns no RED).
- [ ] Implement OpenRead wrapper/manager/path/lifetime/upload/update/delete/WithAsset/RecordUse contracts, with only owned files. No Core/Web/bundle/reply implementation or existing source edits.
- [ ] Use real private synthetic PNG/JPEG/GIF+SQLite+accepted processor. Test scoped duplicates/high IDs/foreign-before-read, reopen/full digest/type/pixel/frame consistency, exact quotas incl orphan/oversized directory count, corrupted/missing/disabled/tombstone, safe cleanup/replacements/hardlinks/rollback/cancellation and raw canaries. Test manager Close and second entrants blocked through actual reader/consumer/Close, then rejection after Close; borrowed processor/store remain usable. Use swapping lease fixture to prove fresh-generation recheck and digest-safe usage.
- [ ] Run new library tests with finite30s disjoint shards and privatefs focused/new tests; if existing suite fits include its finite inventory. Preserve failures and any authentic Windows owner escalation, no ACL/assertion weakening. Scoped vet; no unchanged whole-root repetition (root integrates).
- [ ] Self-review, freeze all10 owned bytes/SHA with actual+copied evidence/RED source/command inventories, adopt only owned exact bytes after quiescence. Report limitations and known recovery states. Root fresh review/full/static/native/Git required before acceptance; no39/live/resource credit.

Plan self-review: interfaces use actual accepted fields/functions, store-generation callback and release order are explicit, no borrowed lifetime is closed, physical quota includes uncertain files, five review-focus cases have meaningful tests. Bundle/retained recovery/Core/UI/reply are deliberately separate future deliverables; this content task does not assert their completion. Routine design decisions under owner autonomy; no actual data/service effect or scope change.
