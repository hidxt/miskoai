# Full library bundle Core integration implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development or executing-plans. Owner authorizes continuous routine work; root owns decisions/integration/Git.

**Goal:** Expose checked complete bundle downloads and full restore through the existing joined Core controller, preserving old SQLite-only methods and channel reconciliation.

**Architecture:** New Core methods reuse the persistent library Manager, current Store leases, existing maintenance/download admission, opaque staged Download and existing validated DB installation. Full restore stops/joins its generation before entering Manager, then captures current rows under a short lease and defers exclusive Store closure until content preflight/retention/installation has completed. No retained stale Store or new worker/pool.

**Tech Stack:** Existing native Go/Core/library/maintenance/privatefs/storage; no dependency/schema/provider/channel/deployment/resource change.

**Spec:** docs/superpowers/specs/2026-10-12-private-library-design.md; D077/D078; accepted Task9a and Task9b interface contracts. CLI/Web/UI are separate dependent tasks; package39 awaits their integration.

## Global constraints

- ONLY gpt-6.1-sol / medium children; engineering74%. All tests fresh synthetic data, no actual private schema2/credentials/network/WeChat/Core.Run with real dependencies/Git effects.
- Existing Core maintenance gate, download slots, persistent cap1 Processor and Store lease rules remain. Existing30s operation bound and321MiB bundle maximum; SQLite-only256MiB path stays unchanged and never accepts envelope bytes.
- Global administrative backup/restore includes all admitted scopes, like existing whole-database backup. Fixed-scope library CRUD remains unchanged. Provider/channel readiness is unnecessary for stopped administrative recovery; construct no extra transport/processor and send no request.
- Completed output is private, fully encoded, actual Sync/Close/reopened/identity-checked before Download metadata returns. Fixed filename only, no path serialized, no partial successful HTTP response. Borrowed Download remains caller-owned and must Close after transfer/abandonment.
- Full restore stops chat generation before taking Manager admission, preventing a joined worker waiting for the same Manager while holding generation/Store lifetime. A malformed/canceled full restore can leave the generation stopped with the original database unchanged; do not silently restart external channels. Persisted pause/result follows actual existing InstallRestore, not a fabricated flag. UI must state this behavior.

## Review focus

1. Exported snapshot and images must actually roundtrip across scopes/reused IDs, and downloaded bytes remain usable without a held Store lease.
2. Full restore never acquires Manager while holding an exclusive/read Store lease, nor stops/joins a worker waiting for Manager from inside that same admission.
3. Actual content/candidate/Store/DB failures preserve original resources and Core availability/paused state accurately; no automatic retry or channel start after ambiguity.
4. Core shutdown/cancellation joins real input/output/cleanup work before store/owned resources close; caller-cooperation and existing maintenance-gate semantics remain explicit.
5. Slot exhaustion, replaced completed output, cleanup/source Close and prepared copy/lifetime failures cannot produce a successful downloadable artifact or stale manager Store.

### Task10: Complete Core backup and restore methods

**Dependencies:** Task9b primary/fresh approval and root scoped verification, exact source/manifest supplied at dispatch. Task9a/9b immutable sources remain untouched.

**Owned three additions:** internal/core/library_bundle.go, library_bundle_test.go, docs/superpowers/reports/2026-10-13-library-bundle-core.md. Existing Core/management/lifetime/library/storage/maintenance/CLI/Web/rootdocs/Git read-only. Unique isolated `.tools/reviews/library-bundle-core-*` work/evidence only.

**Interfaces:**

- `func (c *Controller) PrepareLibraryBundle(parent context.Context) (*Download,error)`: bound30s, acquire existing Core maintenance gate, check current controller/manager state without imposing configured-scope/provider readiness on global administrative backup. Reserve existing download slot nonblocking. Create one random owner-private output with captured original handle identity and cap-limited context-aware writer<=321MiB. Enter Manager.WithMaintenance before a Store lease; call WriteBundle using a fresh `c.lease(ctx,false,false)` adapter which releases on every branch. After actual complete encoder and all borrowed sources return, join output Sync/Close, validate private final size/identity, reopen original for existing Download fields and fixed Filename `miskoai-library.bundle`. Release maintenance/Manager before returning Download; keep only existing download slot until Download.Close. On failure close/join handles and remove only original private output, preserve uncertainty and classify safe cleanup errors. Do not alter existing PrepareBackup/PrepareMemoryExport or their caps/bytes.
- `func (c *Controller) RestoreLibraryBundle(parent context.Context, input io.Reader) (maintenance.RestoreResult,error)`: bound30s and existing maintenance gate; nil/canceled/closed input/state refuse. Stop and join generation BEFORE entering Manager, with no held Store lease. Call Manager.WithMaintenance -> PrepareBundle -> prepared callback. Inside callback capture actual current global LibrarySnapshot via one fresh short unscoped Store lease and release it before InstallBundle. Use `v.InstallBundle(p,current,callback)`; its trusted callback then obtains `c.lease(ctx,true,false)` only after all full media/preflight/retention/copy gates, closes old Store, marks store nil, calls existing maintenance.InstallRestore under c.lock, reads actual RestorePaused state, admits/reopens active DB and updates c.store/unavailable/paused/events exactly as current Restore does. Exclusive lease is released before callback return, including errors; no nested Manager/Processor call while exclusive. Preserve actual result fields, fixed library/recovery/context/maintenance errors and joined candidate/stage cleanup. Never invoke startGeneration/ResumeAfterReconciliation. If preparation fails before DB callback, keep original Store and retained references intact; generation stays stopped. Once DB callback starts, preserve explicit library recovery and actual reopened/paused state even on error. Existing ordinary Restore is byte/behavior unchanged.
- Both methods share existing persistent manager/processor; no default dependency reconstruction or stale captured Store after successful restore. Existing scoped CRUD/preview must work against newly reopened Store or safely refuse missing content, with actual old/new IDs/digests proving the distinction.

- [ ] Freeze compilable semantic RED before execution: `TestCoreLibraryBundleDownloadRoundTrip`, `TestCoreLibraryBundleRestoreAndPause`, `TestCoreLibraryBundleLeaseOrdering`. Use actual synthetic Core/SQLite/PNG/GIF/library/Lock, refused network delegates and never real Core.Run. RED must fail the new Core method semantics after accepted preparation/installer prerequisites pass.
- [ ] Implement two methods in the new production file using existing helpers/lifecycle; no global hooks or relaxed privacy/validation/timeout policy.
- [ ] Add actual all-scope/empty legacy/download-slot/private-output/cancel tests, download Close after Controller.Close, valid restore+new Store CRUD/preview/protected previous content, malformed/trailing/database-only format confusion, stopped-on-preparation-failure, exclusive lease/media-order checks, actual active synthetic generation join with refusal-only channels, and held input/output/consumer/close/cleanup replacement/failure paths. Register unblock/join before assertions. Do not fake installed candidate success or infer owned constructor closure from source-only observations.
- [ ] Run disjoint finite30s new and relevant existing Core/library/maintenance regressions plus scoped vet with exact offline Go1.27.2. No heavy whole-product/static/native/private/live operation; authentic owner ACL/hardlink proof only if required. Record actual exits/pass-event/distinct/skip/error counts and source-only driver/durability limits.
- [ ] Freeze three owned/complete source/evidence SHA256+lengths and patch/report, quiesce. Root primary/fresh review and combined whole-product/static/native gate follow; no package39 credit yet.

Self-review: global backup scope matches existing administrative whole-DB recovery; unchanged scoped CRUD still protects users. Pre-generation join prevents Manager inversion introduced by future reply selection. Full restore's possible stopped generation on invalid input is explicit and has a test/UI requirement. Private completed artifacts and original-only cleanup prevent partial success; actual prior DB/channel pause is delegated to existing reviewed installation, without atomicity or live/resource claims.
