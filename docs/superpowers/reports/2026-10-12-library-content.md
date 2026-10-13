# Private immutable library content manager — Task3

2026-10-13. Author: GPT-6.1 Sol, Medium reasoning (`gpt-6.1-sol`, `medium`). Engineering remains **74% = 37/50**. This is local Task3 implementation/evidence, awaiting root independent review and integration. No package39 completion credit. Accepted foundation basis is the immutable 353-file `library-foundation-fix1-root-reviewed-src`; all 353 existing files compare byte-for-byte unchanged. No existing storage/media/attachment/Core/Web/root documentation/Git/module/license files were edited.

## Delivered interfaces and ownership

Exactly ten new files: `internal/privatefs/reader.go`, `reader_test.go`; `internal/maintenance/sync_export.go`; `internal/library/library.go`, `content.go`, `lifecycle.go`, `library_test.go`, `content_test.go`, `lifecycle_test.go`; and this report. The evidence workspace uses only `library-content-*` prefixes.

`privatefs.OpenRead` delegates the existing checked open boundary and lends an owner-private read-only handle. Its caller closes the handle; it never removes persistent content. `maintenance.SyncPrivateDirectory` delegates the existing platform sync boundary without changing policy. Windows verifies private metadata only; Linux directory-fsync and real power-crash evidence remain separate.

The manager owns one synchronous admission and Close lifetime. It borrows the existing shared `WithLocalImage` processor and generation-aware `Lease`; it never closes either the processor or Store, retains no Store, and creates no decoder pool or worker goroutine. Every nonnil lease release runs once, including error-with-lease. Close stops admission, cancels active work and joins the actual reader, consumer, media return/Close, persistent-handle Close and cleanup. Readers, callbacks and dependencies must actually return; there is no forced interruption, hard timeout or RSS claim.

Targets are `asset-` plus lowercase SHA256 of uint32 big-endian length-prefixed Account and User followed by the decoded content SHA256. Names/descriptions/tags never become paths. The identity cache holds at most128 FileInfo values, with Windows identity explicitly captured. Upload validates scope, metadata, disabled state and1..4MiB size before reads. Real shared image validation supplies digest/format/dimensions/frames. Publication copies with4096-byte scratch into create-exclusive private content, synchronously Syncs/Closes/checks identity/privacy and syncs the directory before SQL metadata commit. Known scoped duplicates retain metadata/state/usage unchanged and compare the full persistent digest against the fully validated input. Untracked same-key files are never adopted or overwritten.

All uploads, including duplicates, run bounded `ReadDir(129)` physical admission against the trusted bounded SQL snapshot. Count128 and64MiB are physical ceilings; unknown entries and retained failed objects are preserved and block admission. Regularity,1..4MiB size, generated names, privacy and single-link policy are checked. SQL quotas are also checked before publication. Failed publication removes only the captured newly created original after actual Close. Ambiguous SQL commit errors require an authoritative snapshot under the same leased Store before absence-based removal; this cleanup check uses `context.WithoutCancel` synchronously, preserving the admitted lifetime and bounded Store contracts. Failed verification leaves recovery state rather than guessing ownership.

WithAsset performs scoped ready/enabled lookup before persistent access, releases that lookup lease before shared media admission, stages persistent bytes through a bounded SectionReader into a temporary validated artifact, verifies full computed immutable metadata and original identity, then reacquires a fresh lease and rechecks ID/scope/digest/immutable fields/state before the consumer. The fresh lease stays held through consumer return. Persistent content is never wrapped in a deleting media Artifact. Update enabling uses this validation path; disabled metadata edits need no file repair. RecordUse uses a fresh scoped row and matching digest before the existing storage mutation; caller owns confirmed-send semantics.

## Root clarifications and recovery ordering

Ruling: physical directory admission gates every Upload, including duplicate. Safe metadata-only edit/disable and verified deletion of the scoped original may proceed while unrelated orphans remain preserved. Enabling and selection fully verify their own content under the shared media slot. Cost if wrong: an unrelated orphan could incorrectly block safe recovery edits, or an unsafe mutation could overlook physical admission. `TestContentOrphanBlocksNewAndDuplicateUpload` and `TestContentRecoveryAllowsScopedDisableAndDelete` exercise the chosen behavior.

Ruling: deletion persists disabled deleting intent before full existing-content validation. Foreign scope and a ready row whose content is already missing refuse before mutation. A fresh immutable-row recheck precedes MarkDeleting; its lease releases before entering media processing. Corrupt/replaced content remains intact with a disabled tombstone and ErrRecovery; verified originals are removed/synced before a fresh scoped finalization. Only a known missing deleting row can finalize without content. Cost if wrong: an uncertain enabled row could survive a requested deletion, or an unowned replacement could be removed. `TestContentFailedDeletionPersistsDisabledIntent` checks disabled durable state, exact retained bytes/identity and no Store-to-media inversion. Missing tombstone finalization also releases its obsolete bounded identity entry.

No sweep, automatic reconciliation, orphan adoption, restore registry, bundle workflow, Core/Web/reply integration or usage inference is introduced. Retained objects after a later restore still require the separate explicit recovery contract. Recovery/tombstone rows consume metadata quotas; retained files consume actual physical admission.

## Verification and retained attempts

Native Windows Go1.27.2 using the repository's verified toolchain and local offline caches. Six finite30s disjoint final test commands are recorded in `library-content-final-*-argv.json`, with actual JSON events, stderr, exit files and inventories. Final evidence is:

| Final prefix | Named pass | Fail | Skip | Exit |
| --- | ---: | ---: | ---: | ---: |
| final-basic3 |27|0|0|0|
| final-recovery3 |23|0|0|0|
| final-lifecycle3 |3|0|0|0|
| final-rowquota3 |1|0|0|0|
| final-physical-owner3 |9|0|0|0|
| final-privatefs-owner3 |18|0|2|0|

Union: **63 distinct named library passes across28 top-level tests**, plus18 privatefs passes and two existing Windows platform skips; **81 passes,0 failures,2 skips**. These are disjoint scoped commands, not a whole-module pass. All final stderr files are empty. Scoped `go vet ./internal/library ./internal/privatefs ./internal/maintenance` is `final-vet3`, exit0 with empty diagnostics. Root owns whole-module/static/security/native/publication verification.

Real synthetic private PNG/JPEG/GIF, two-frame GIF, SQLite and the accepted shared processor cover scoped duplicates and distinct targets, foreign/high-ID refusal before processing, reopened full digest/type/size/pixel/frame checks, disabled/missing/corrupt/replacement handling, exact4MiB upload and untracked-key refusal, global128 rows, physical128/129 files and64/over64MiB boundaries, unknown names/directories/hardlinks, cancellation, raw error canaries, create/copy/SQL failure cleanup and original-only replacement refusal. A max-int SQLite sequence fixture produces genuine post-publication SQL-capacity failure and verifies the known new file is removed while existing content remains. Swapping synthetic Store generations proves fresh identity recheck and digest-safe usage. Shutdown tests hold actual reader/consumer/processor return, block second entrants and shared media use, join concurrent/idempotent Close, and verify borrowed Store/processor remain usable. Actual Windows media cleanup failure is induced by a held second read handle: cancellation and ErrCleanup coexist, the media artifact has closed, and persistent library content remains.

Preserved attempts, with no RED credit for setup errors:

- Original default-toolchain run could not download/open its restricted external cache; initial copy commands also used an incorrect relative cwd. Original `red.log`/exit and inline attempt record remain. No compilation was claimed.
- Compiled semantic `red-native`: PNG/JPEG/GIF upload cases fail because scaffold returns ErrInvalid; new OpenRead fails because its scaffold refuses. The independent invalid-input test passes. Initial RED scaffold files remain frozen.
- Expanded semantic RED preserves duplicate-orphan bypass and cancellation arriving during processor cleanup return. Corrected GREEN prefixes pass.
- Deletion-intent RED preserves corrupt/replacement rows remaining ready+enabled and finalized missing tombstone retaining identity. Exact actual executed RED source and corrected GREEN are retained.
- Restricted hardlink setup fails with Access denied; authentic normal-owner rerun passes. No ACL or assertion was weakened. Final physical/privatefs suites run as the normal owner.
- First actual-cleanup fixture attempted to rename an open Windows stage, which Windows refused; this setup failure remains. The corrected Windows fixture uses a real second-handle removal refusal; Linux test code uses replacement identity but has not run natively here.
- `final-vet2` reports three unkeyed external Scope literals in tests. Keyed fields replace them; final source passes scoped vet and final disjoint checks. No production fix or semantic RED is inferred from that vet diagnostic.
- Original format-runner follow-on relative invocation/read errors remain in the inline attempt record. They do not establish test or review success.

## Self-review, freeze and limits

Author self-review read the complete production boundary, lifecycle, content operations, tests and report. Focus covered scope-before-read, original-only failure cleanup, duplicates/tombstones/replacements, generation/Close races and complete validation before enable/lending. No unresolved author finding is asserted as independent approval. Fresh root review remains required.

The frozen review package uses `library-content-reviewed-src`, `library-content-owned-manifest.json`, `library-content-source-manifest.json`, `library-content-evidence-manifest.json`, copied original evidence and command/result inventories. Owned and evidence rows identify actual paths, copied paths, byte lengths and SHA256. Source/evidence freezes follow actual command completion; adoption copies only these ten exact new files after root clearance. No foundation bytes, LICENSE or existing public content change.

This task has no actual private schema2 migration, service/login/probe, network/model/provider/CDN/WeChat/server execution, container/WSL or permanent runtime effect. Native Linux race/systemd, arm64 runtime, live channel,512MB/RSS, actual power-crash and final acceptance remain unverified. Latest accepted foundation8c06761/native38281939206 excludes this WIP, as verified by root; old08814e0/native38248405265 constructor failure remains unresolved history. The paid allowance is spent and real WeChat is deferred. Bundle/retained recovery/Core/Web/selection are future gates, so package39 remains uncredited.
