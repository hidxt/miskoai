# Core private library integration implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development or executing-plans. Root reviews/integrates/Git; routine continuous development is owner-authorized.

**Goal:** Bind the reviewed private content manager to Core's actual shared media and Store-generation lifetime, providing real scoped management operations.

**Architecture:** Default Core owns one persistent Manager borrowing the existing ImageProcessor. Each operation obtains the current fixed authorization scope and fresh Core read leases; scoped leases recheck the captured scope. Close joins manager operations before closing owned CDN/Store/lock. Borrowed legacy image dependencies that lack local processing leave this capability unavailable rather than create another processor.

**Tech Stack:** Existing native Go1.27.2/Core/library/storage/media; no new runtime/dependency/schema/provider/channel policy.

**Spec:** docs/superpowers/specs/2026-10-12-private-library-design.md Core section, reviewed Task3 content plan/D069/D071 and current Core generation/lease contracts. Full bundle/Web/reply remain separate.

## Global constraints

- Children only gpt-6.1-sol/medium; actual private schema2/data and spent live allowance untouched.
- Reuse exactly existing cap1 media processor; no extra image decoder/pool or model call. Manager and Core admission retain actual synchronous cleanup lifetimes.
- Core management uses configured authorization's account/user scope; unconfigured/changed/closing/unavailable states refuse safely before content access. Provider readiness is not needed for authorized offline library management.
- Fresh read lease per manager callback; never call Manager while holding a Store lease. Brief lookup lease releases before media; fresh consumer lease retains actual use. No stale Store retained across Restore.
- Existing Restore remains SQLite-only and channel-pausing, retaining prior database/content; no bundle/install/registry implementation or content sweep. Missing restored content refuses later validation. In-memory identity cache is not a restore-availability claim.

## Review focus

1. Default composition actually shares the current image/document processor; legacy borrowed injection stays borrowed/unavailable without another pool.
2. Foreign/old scope, high IDs, canceled/nil inputs and closed Core cannot invoke a content consumer or read private bytes.
3. Close and constructor unwind join actual Manager operations before CDN/Store/lock closure; a held real read/consumer keeps the underlying Store available until it returns.
4. Database restore cannot reuse stale Store/ID/hash; all per-call leases come from the current generation and a captured-scope recheck.
5. Authenticated offline administration works without model/provider calls; real upload/edit/enable/preview/delete operates over synthetic private files and SQLite, not stubs.

### Task5: Core owner, scope and management methods

**Owned five files:** modify internal/core/core.go, lifetime.go; create internal/core/library.go, library_test.go, docs/superpowers/reports/2026-10-13-core-library.md. All other existing tests/files/rootdocs/Git and library/snapshot/codec/document sources read-only. Own ignored .tools/reviews/core-library-*.

**Interfaces:**
- Core gains one private `library *library.Manager` field. If `c.images` implements accepted `library.Processor`, construct Manager after that processor exists, tracking it for constructor unwind. A failed manager construction returns a safe Core error. No new Dependency factory or independent media implementation.
- Constructor failure cleanup: owned manager.Close before owned CDN/Store/lock Close. Core.Close: publish closing/cancel/stop+join existing generation, then manager.Close before waiting/finalizing Store/CDN/lock close. Manager pointers are persistent across DB generations and Restore; borrowed Processor/Store ownership remains unchanged.
- Internal `libraryAccess(ctx context.Context) (*library.Manager,storage.Scope,error)` checks current available/closing/configured scope and capability without provider-ready gating. Internal fresh lease callback checks current scope equals captured scope while its acquired Core read lease is held; release every nonnil lease even on mismatch/error. Return only fixed Core/library/storage/context classes.
- `(*Controller).LibraryAssets(ctx context.Context,afterID int64,limit int) ([]storage.LibraryAsset,error)`: existing scoped keyset1..32 metadata listing via fresh lease. Return metadata/state only; it does not claim full file validation or availability.
- `LibraryUpload(ctx context.Context,input io.Reader,size int64,meta storage.LibraryMetadata) (storage.LibraryAsset,bool,error)`, `LibraryUpdate(ctx context.Context,id int64,meta storage.LibraryMetadata) error`, `LibraryDelete(ctx context.Context,id int64) error`: call actual Manager methods outside Store lease, captured fixed scope and fresh guarded lease callback, bounded operation context30s including waits.
- `WithLibraryAsset(ctx context.Context,id int64,requireEnabled bool,consume func(storage.LibraryAsset,attachment.LocalImage,attachment.Artifact) error) error`: actual borrowed validated temporary image plus current row; context30s; callback must finish synchronous consumption and must not reenter Core/library or retain the Artifact. This is the later preview/reply boundary, no HTTP/network implementation here.
- `LibraryRecordUse(ctx context.Context,id int64,digest string,atMillis int64) error`: fresh scoped digest-safe actual Manager mutation. Trusted confirmed-send caller owns semantics; no public usage inference or automatic increment.

- [ ] Write compiled semantic RED after minimal new API scaffolding using real default/synthetic Core, scope and local image fixtures; freeze commands/source/results. No compile/setup credit.
- [ ] Implement exact ownership/access/lease/method contracts using only owned files. Preserve existing model/search/channel/summary/serve/backup/restore interfaces and behaviors.
- [ ] Test five focus cases with real synthetic PNG/GIF/SQLite/default shared processor, exact duplicates/foreign-highIDs/disabled/preview cleanup, config without provider key, legacy borrowed capability absence, held Manager consumer/Close ordering, default processor pointer identity and post-Close borrowed behavior. Test restore to another synthetic DB with reused ID/different digest, old use refusal and new live lease; original content preserved and channel pause honest.
- [ ] Run finite30s disjoint new/focused Core checks and scoped vet, with cleanup/join registered before lifetime assertions. No unchanged whole-root or aggregate slow-suite repetition; no ACL/assertion weakening.
- [ ] Self-review/freeze five owned/evidence identities and complete source, return actual commands/results/limits/quiescence. Main adoption after root clearance/current checkpoint publication; fresh review/root whole tests/static/native required. No39/Web/bundle/reply/live/resource/final credit yet.

Self-review: every interface is defined and uses existing actual Manager/Core lease types; all five focus cases have explicit tests; no second processor or untrusted path/usage/network endpoint is introduced. Metadata listing remains distinct from complete on-use validation, and existing SQLite-only restore cannot imply binary recovery.
