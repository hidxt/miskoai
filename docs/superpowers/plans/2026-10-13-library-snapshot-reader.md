# Library snapshot reader implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development or executing-plans. Root integrates/reviews/Git; routine continuous development is owner-authorized.

**Goal:** Read bounded library metadata from a validated immutable SQLite snapshot without opening or migrating a writable Store.

**Architecture:** Reuse the existing read-only backup admission/manifest/integrity connection for a small optional metadata callback. Add a checked Candidate method so later bundle code obtains metadata without exposing a private path or weakening Candidate ownership. This is metadata groundwork, not bundle installation.

**Tech Stack:** Native Go1.27.2, existing pinned modernc SQLite; no dependency/runtime/schema/provider change.

**Spec:** docs/superpowers/specs/2026-10-12-private-library-design.md backup/restore section, D069, exact existing ValidateBackup/Candidate contracts.

## Global constraints

- Only gpt-6.1-sol/medium children. Actual private schema2/data untouched; no live/network/service/model/server effect.
- Preserve exact legacy1..4 and current5 manifests, existing admission/integrity and WAL/SHM refusal. Preserve existing SQLite-only backup bytes and256MiB Candidate input cap.
- Use mode=ro&immutable=1/query_only and existing one-connection/8MiB cache contract. No storage.Open, migration, chmod, repair, checkpoint or journal creation by the metadata reader.
- Maximum128 admitted library rows/64MiB declared content; SQL shape/scalar/count/sum admission before string/JSON materialization. All actual handles/connections close before return; safe errors/context cancellation only.

## Review focus

1. Exact legacy snapshots return an empty library without changing bytes/schema or creating sidecars.
2. Unknown schema/DDL and malformed or oversized library values refuse before metadata materialization. Preserve ValidateBackup's exact WAL/SHM refusal; the new metadata API and existing Candidate ownership boundary may additionally refuse rollback journals without changing the legacy public validator.
3. Read failure/cancellation/connection-close failure cannot return a successful partial catalog or raw SQL/path canaries.
4. Copied, discarded, changed or already installed Candidates cannot lend snapshot metadata.
5. Complete current snapshots preserve exact IDs/scope/state/digest/usage/metadata; this does not prove binary images exist or are valid.

### Task4b: Read-only library metadata and owned Candidate access

**Owned six files:** modify internal/storage/validate.go; create internal/storage/library_backup.go, library_backup_test.go, internal/maintenance/candidate_library.go, candidate_library_test.go, docs/superpowers/reports/2026-10-13-library-snapshot-reader.md. Other existing files/tests/rootdocs/Git read-only. Own ignored .tools/reviews/library-snapshot-reader-*.

**Interfaces:**
- Internal `validatedBackup(ctx context.Context,path string,consume func(*sql.DB,int) error) (err error)` shares the complete existing ValidateBackup flow. ValidateBackup delegates with nil callback. Reject nil/canceled context safely; callback runs only after the exact version/manifest/data/integrity gate; include actual connection Close in the returned fixed result. No admission query or policy is skipped/reordered around materialization.
- `storage.LibraryBackupSnapshot(ctx context.Context,path string) ([]LibraryAsset,error)`: on admitted versions1..4 returns an empty slice; on5 reuses existing bounded library materialization/preflight over the same immutable connection. Returns nil on any error, never a writable Store or private path. A temporary Store wrapper around the already read-only DB is acceptable to reuse LibrarySnapshot, with no Store.Open or double close.
- `(*maintenance.Candidate).LibrarySnapshot(ctx context.Context) ([]storage.LibraryAsset,error)`: owns c.mu, rejects copied/finished/unsafe Candidate through existing verify before and after the read, calls the new read-only storage API, maps unknown errors to existing safe maintenance classes and preserves cancellation. Does not close/discard/install the Candidate; caller retains ownership.

- [ ] Write compiled semantic RED after minimal new API scaffolding for current metadata equality, exact legacy empty/no-change, malformed refusal and Candidate ownership. Freeze original/scaffold/source and actual commands/results.
- [ ] Implement only the shared read-only admission helper and checked methods; preserve all existing public validation/restore behavior and error redaction.
- [ ] Test the five review-focus cases with real private synthetic SQLite/consistent snapshots/Candidates. Hash bytes, schema versions and sidecar directory entries before/after, and assert row quotas before materialization with hostile values. Verify canceled/nil contexts safely, bounded complete128 rows and rejecting129. Validate copied/discarded/changed Candidate without changing/removing replacements.
- [ ] Run finite30s inventory/disjoint new/focused storage and maintenance checks plus scoped vet; no unchanged aggregate storage/whole-module reruns. Preserve failure/owner ACL evidence, no assertion/policy weakening.
- [ ] Self-review, freeze six owned/evidence identities and complete source, return actual commands/results/limits and quiescence. No main adoption until root clearance after current checkpoint publication; fresh review/root integrated/static/native required. No39/bundle install/image/live/resource/final credit.

Self-review: both public methods and the internal helper are defined, existing validation remains authoritative, all five focus cases have exact evidence requirements, and the caller remains responsible for private content and install recovery. Accepted schema5 foundation is the dependency; no content-manager or codec source edits are required.

Compatibility ruling2026-10-13: the broad review-focus word 'journals' does not authorize changing existing ValidateBackup behavior. D070's WAL/SHM versus rollback-journal distinction remains. Isolate additional rollback-journal refusal to the new LibraryBackupSnapshot API and existing Candidate.verify; add an explicit regression for the old validator. Preserve the earlier attempted stricter shared-helper source/results separately; no actual private database policy is changed.
