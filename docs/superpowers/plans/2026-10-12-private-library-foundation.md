# Private library foundation implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development (or executing-plans). Root owns integration/review/Git; owner authorized continuous routine execution.

**Goal:** Build two independently reviewable foundations for the complete private-library spec; neither alone completes package39.

**Architecture:** Compatible bounded schema5 metadata, and local staged-image processing sharing the accepted image/document admission. Later library content/bundle/Core/Web tasks require a separate concrete plan against these accepted interfaces.

**Tech Stack:** Go1.27.2, existing pure-Go SQLite/privatefs/media/attachment. No dependency/runtime changes.

**Spec:** docs/superpowers/specs/2026-10-12-private-library-design.md

## Global constraints

- Children ONLYgpt-6.1-sol/medium. No actual private schema2 migration/startup, network/provider/CDN/WeChat calls, server/container/WSL or cap changes.
-128 global rows,64MiB summed content,1..4MiB each; description1024,emotion64,tags16*32/JSON2048; existing scopes/ID/privatefs/image bounds.
- Preserve exact schema1..4 and all old constraints; actual complete reads/cleanup before shared cap1 release.
- Preserve failed/RED evidence and exact source freezes; no missing compile failure labeled semantic RED or weak watchdog/ACL workaround.

## Review focus

1. Malicious schema5 values/effective WAL: pre-admit types/counts/lengths before Go allocation.
2. Scope and duplicates: high IDs/foreign updates or repeated upload never replace existing metadata or increment quota.
3. Tombstones/usage overflow: disabled deleting rows stay quota-accounted, only successful finalization removes them.
4. Local stream truncation/trailing bytes/cancel errors: reject, safe class, only owned cleanup.
5. Shared image/document/local admission: actual consumer and Close block successors even after cancellation.

### Task1: Scoped schema5 metadata

**Owned files:** create internal/storage/library.go, library_test.go, admission5.go, schema5_test.go; modify migrations.go, validate.go, store.go, admission3.go only as needed for explicit old/current manifest/admission/sequence routing. Create docs/superpowers/reports/2026-10-12-library-storage.md. Own ignored .tools/reviews/library-storage-* evidence. No other files/Git/rootdocs.

**Interfaces:** `storage.LibraryAsset` fields `ID int64`, `Scope Scope`, `SHA256,Format string`, `Bytes int64`, `Width,Height,Frames int`, `Tags []string`, `Emotion,Description,State string`, `Enabled bool`, `Uses,LastUsedMillis int64`; State `ready|deleting`. `LibraryMetadata{Tags []string, Emotion,Description string, Enabled bool}`. All methods on `*Store`: `AddLibraryAsset(ctx context.Context, sc Scope, asset LibraryAsset) (LibraryAsset,bool,error)` bool true means new insert; `LibraryAssets(ctx,sc,after int64,limit int) ([]LibraryAsset,error)` limit1..32 keyset; `LibraryAsset(ctx,sc,id int64) (LibraryAsset,error)`; `UpdateLibraryAsset(ctx,sc,id int64,meta LibraryMetadata) error`; `MarkLibraryDeleting(ctx,sc,id int64) error`; `FinishLibraryDelete(ctx,sc,id int64) error` only tombstone; `RecordLibraryUse(ctx,sc,id int64,atMillis int64) error` ready+enabled only with overflow guard; `LibrarySnapshot(ctx context.Context) ([]LibraryAsset,error)` <=128 admitted global rows for trusted internal backup. SHA/scope/format/dimensions/bytes/usage immutable under edit. Reject nonzero input ID/usage/lastused, caller scope mismatch, nonready/new-disabled requirement: uploads initially disabled and are explicitly enabled later. Format `png|jpeg|gif`; dimensions1..8192 with width*height<=16000000/frames1..32, non-GIF frames exactly1, pixels/estimated full-decoder consistency additionally proven by actual manager validator. Existing image validation may impose stricter constraints; storage never legitimizes bypassing it.

- [ ] Write meaningful compiled RED scoped CRUD/duplicate/quota/state/high-ID/usage tests. For new interfaces, an initial compile failure is scaffolding only; then provide minimal stubs and capture actual semantic failures before implementation.
- [ ] Add transactional schema5/manifest/admission and methods; all writes scope-bound. Extract exact legacy4 manifest and route old versions explicitly, including sqlite_sequence. Unknown/future DDL/types/over-capacity/error rollback refuse without mutation.
- [ ] Test synthetic fresh/open/reopen, exact versions1..4 restore/admission unchanged, schema5 malformed row/type/UTF8/JSON/count/byte budgets and effective WAL. Tests must prove no bulk string materialization before admission and no changed actualdata policy.
- [ ] Run disjoint finite15s scoped storage shards (existing slow package split), every actual top-level test once; vet. Preserve RED/source/logs/exits. No repeat expensive unchanged whole-root suite; root will integrate.
- [ ] Self-review and freeze owned/report/evidence bytes+SHA with copies and command inventories; report quiescence/actual limitations. No package39 credit or live claims.

### Task2: Local private image lifetime

**Owned files:** create internal/media/local.go, local_test.go, internal/attachment/local.go, local_test.go, docs/superpowers/reports/2026-10-12-local-image.md. Existing spool.go/image.go/document.go read-only; root must approve any narrowly needed additional ownership. Own ignored .tools/reviews/local-image-* evidence. No storage/Core/Web/rootdocs/Git.

**Interfaces:** `media.StageLocal(ctx context.Context, dir string, input io.Reader, size int64) (*media.Artifact,error)` with existing spool implementation; transferred returned artifact ownership even if nonnil error. `attachment.LocalImage{Info media.ImageInfo, SHA256 string, Bytes int64}`; `(*attachment.ImageProcessor).WithLocalImage(ctx context.Context,input io.Reader,size int64,consume func(LocalImage,Artifact) error) error`. Constructor/slot unchanged; local-only tests can instantiate existing processor with refusing synthetic downloader. New library consumes this operation later.

- [ ] Write finite semantic RED for exact stream/truncated/trailing/error/cancel and whole lifetime. If interfaces absent, compile scaffold first, then capture compiled expected failures.
- [ ] Implement StageLocal with existing spool, bounded4096scratch/exact bytes+trailing EOF probe/Sync/Close/reopen; safe fixed errors and actual owned cleanup. Implement local processor using same `p.slot`, full image validator, SHA256 all bytes, synchronous consumer and actual artifact Close before admission release.
- [ ] Test PNG/JPEG/GIF actual private artifacts, invalid/oversized/trailing/corrupt images, raw canary redaction, cancel during reader/consumer/Close and held lifetime serializing image/document/local entrants. Existing helpers may be read, not modified. Preserve cleanup/replacement/hardlink boundaries and no downloads/model calls.
- [ ] Run finite disjoint15s scoped media/attachment shards exactly once and vet; preserve compiled RED and actual errors separately. Root whole-product checks later.
- [ ] Self-review/freeze5owned plus evidence identities/copies, command inventory/exits and quiescence. No library/backend/reply/live/resource credit.

Plan self-review: interfaces and constants concrete; five review-focus classes have task tests; tasks disjoint and independently rejectable. Storage frame ceiling follows existing32-frame full GIF validation (the earlier128-frame draft was corrected before dispatch); dimensions follow existing imageDimensions bound. This is explicitly a foundation subplan, not a claim the later content/archive/Core/Web plan exists or package39 is complete.
