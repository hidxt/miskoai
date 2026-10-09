# Shared lifecycle and stopped maintenance Task1

Base: `e421940c03dfce1eeae22cc1aef0f4c3508f1a36`. Implementer: GPT-6.1 Medium (`gpt-6.1-sol`, medium). Scope: the isolated `.tools/reviews/maintenance-brief.md` and the primary agent's added directory-sync requirement. No future Core/Web task bodies were read or implemented. Files are frozen for a fresh independent Medium gate; this report is implementation evidence, not final acceptance.

## Owned changes

Created `internal/maintenance/lock.go`, `backup.go`, `restore.go`, `pause.go`, `maintenance_test.go`, and the additionally authorized `sync_unix.go` / `sync_windows.go`. Modified only `internal/cli/operations.go` and `cli_test.go` for maintenance delegation and fixtures. Created this report. Other working-tree changes belong to the primary agent or other work; none were modified here. No staging, commits, global configuration changes, network requests, actual data/auth/credential access, live startup, or child delegation.

The shared exclusive `.miskoai.lock` retains the original handle, file identity and directory identity. Acquisition validates raw path spelling and the private dedicated directory. Second and stale owners are refused. Ownership checks reject missing, closed, copied, replaced and mismatched locks. Idempotent close only removes the acquired identity; a replacement is retained.

Stopped backup acquires lifecycle ownership before database metadata checks or engine open. Existing main/WAL/SHM/rollback-journal metadata is checked through privatefs with `math.MaxInt64`, without adding a physical database quota. Destination spelling and parent privacy are checked before the existing `Store.Backup` consistent-snapshot primitive. A completed snapshot is privacy-checked and immutably validated; cleanup can remove only its owned main file. Engine close and lock close errors are checked.

Restore input is capped at 256MiB plus one byte of oversize detection, streamed in bounded chunks with cooperative context checks. Random exclusive private candidates are synced and closed, immutably validated before quota/migration admission, admitted only as candidates, then normally closed with retained journals refused. Candidate paths stay internal. Discard is idempotent, checks file identity, refuses replacements/unexplained journals, and never manually deletes journals. Install requires the exact live lifecycle ownership object for the candidate directory, unchanged candidate metadata, a private target, and no unexplained target journals. Existing data is preserved under a unique private pre-restore name. Reopen/integrity/normal-close are verified; failures roll back or retain private recovery state with a constant manual-recovery error.

Stopped restore resolves the raw source spelling, refuses main/candidate aliases, checks private regular-file metadata and the source cap before streaming, compares opened/named identity/size/time before and after streaming, refuses source journals before and after the copy, and preserves the source. CLI creates a 30-second maintenance context before these operations. Success output states whether a previous database was retained and that external channels remain paused, without private filenames or raw filesystem/SQLite errors.

The fixed private pause marker is bounded to 1KiB and requires the exact known version/reason object: complete UTF-8 JSON, numeric version 1, no null, unknown/case alias, duplicate or trailing values. Creation/write/file-sync/close failures refuse installation. Existing valid markers are re-synced on retries. Installation never automatically clears the marker. Explicit clear requires verified live ownership and checks removal/directory sync.

## Directory durability refinement

The primary source review identified that syncing a marker file alone does not persist its parent directory entry on Linux. The primary authorized `sync_unix.go`, `sync_windows.go` and added failure tests. Linux/non-Windows code now opens an identity-checked private directory and checks `Sync` and `Close`. Installation syncs the directory after the pause is established and before any target rename, after preservation, after candidate installation, after normal engine close, and after each rollback rename. Existing valid-marker retries still execute the pre-install directory sync. Sync failures retain the pause and refuse durable success.

If rollback's final directory sync fails after restoring the original target, the code attempts to preserve that original identity back under its private pre-restore name, checks another directory sync, and returns the constant manual-recovery error. It never reports a failed durability operation as successful. Filesystem or device failures can still prevent a rename or sync; no synthetic test establishes power-loss atomicity.

Windows `syncDirectory` verifies private metadata only. Windows tests establish normal close/reacquisition/restart marker persistence and fault handling; they do not establish Linux-equivalent directory-fsync or power-crash durability. The real Linux fsync code still requires the primary's native Linux gate.

## RED → GREEN evidence

All runtime commands used the verified workspace Go toolchain and offline caches described below.

1. `go test ./internal/maintenance -run TestLifecycleLockOwnership -count=1`: runtime RED, lifecycle lock unavailable against an explicit not-implemented API stub. Initial full maintenance stub run also failed `TestBackupRefusesRunningOwner`, `TestRestorePauseSurvivesRestart`, and `TestPrepareRestoreSnapshotAndCancellation` for missing implementation. Subsequent maintenance run passed.
2. `go test ./internal/maintenance -run 'TestRestorePauseStrictMarker|TestCandidateChangedRefused|TestRestoreRollback' -count=1`: runtime RED accepted a quoted version and admitted a changed same-identity candidate past ownership checks. Numeric token typing and post-admission candidate metadata checks corrected both; final scoped run passed these cases.
3. `go test ./internal/cli -run 'TestBackupCLIRefusesLifecycleOwner|TestBackupRestoreRoundTrip' -count=1`: runtime RED showed the previous CLI backup opened the database while another owner held the lock, and restore output omitted the truthful retained/paused claims. Delegation and output updates corrected both; final CLI run passed.
4. `go test ./internal/maintenance -run TestPauseDurabilityFailureRefusesInstall -count=1`: runtime RED with the writer seam initially ignored. Create/write/sync/close injected failures each incorrectly installed. The real durable writer path now checks all four and the written byte count; focused GREEN passed.
5. `go test ./internal/maintenance -run TestLockRejectsMismatchedDirectory -count=1`: runtime RED admitted a mismatched directory for reconciliation. Acquired directory/file binding corrected it; final scoped run passed.
6. `go test ./internal/maintenance -run TestRestoreRefusesSourceJournals -count=1`: runtime RED accepted main-only copies of sources with WAL, SHM and rollback journals. Pre/post-copy source-journal refusal corrected all three; focused GREEN passed.
7. `go test ./internal/maintenance -run 'TestRestoreDirectorySyncFailure|TestRestoreRetrySyncsExistingPause' -count=1`: runtime RED ignored directory-sync faults and existing-marker retry ordering. The checked sync sequence corrected these; `go test ./internal/maintenance -run 'TestRestoreDirectorySyncFailure|TestRestoreRetrySyncsExistingPause|TestRestoreRollback' -count=1` passed after implementation.
8. `go test ./internal/maintenance -run TestPrepareRestoreCancellationDuringRead -count=1`: runtime RED returned the generic restore error for cooperative cancellation during a read. Classification now retains `context.Canceled`; final scoped run passed and verified candidate cleanup.

Additional real fixtures cover preserve/install/reopen/rollback faults, recovery snapshot bytes, corrupt/future/unsafe/oversize/alias refusal, candidate replacement and retained journals, source preservation, failed readers, unchanged targets, pause persistence across reacquisition, explicit reconciliation ownership, and output writer failures without undoing completed private backup/restore state. Existing CLI corrupt/unrelated/round-trip/unsafe-sidecar fixtures were preserved and run.

Two preliminary runs are not runtime RED evidence: the first command pointed at the empty `.tools/gomodcache` rather than populated `.tools/gomod` and failed offline dependency setup; the cache path was corrected with no download. The moved-directory test first encountered Windows access denial because a live child lock handle prevents directory renaming; it now explicitly skips on Windows, with the native Linux requirement recorded below.

## Final local commands and results

Environment for every successful Go run:

```powershell
$env:GOTOOLCHAIN='local'
$env:GOPROXY='off'
$env:GOSUMDB='off'
$env:GOPATH="$PWD/.tools/go"
$env:GOMODCACHE="$PWD/.tools/gomod"
$env:GOCACHE="$PWD/.tools/gocache"
$env:GOTMPDIR="$PWD/.tools/tmp"
$env:TEMP=$env:GOTMPDIR
$env:TMP=$env:TEMP
```

`& .tools/toolchains/go1.27.2-verified/go/bin/go.exe version` returned `go version go1.27.2 windows/amd64`.

Final test command:

```powershell
& .tools/toolchains/go1.27.2-verified/go/bin/go.exe test -json ./internal/maintenance ./internal/cli ./internal/storage -count=1 |
  Set-Content .tools/reviews/maintenance-tests-final.jsonl
exit $LASTEXITCODE
```

Exit 0. Counting named tests and subtests: **272 passed, 0 failed, 2 explicitly skipped**. CLI: 22 passes, 0 skips; maintenance: 54 passes, 1 skip; storage: 196 passes, 1 skip. Package results: CLI pass (13.587s), maintenance pass (21.070s), storage pass (77.153s). Skips: `maintenance.TestLockRejectsMovedDirectory` (Windows cannot perform the Linux directory-move fixture while the lock handle is live); `storage.TestExistingParentPermissionsArePreserved` (existing Unix-permission-only fixture). No package failure occurred. The earlier pre-directory-refinement scoped run was 264 passes / 0 failures / 2 skips; it is not counted as the final run.

`& .tools/toolchains/go1.27.2-verified/go/bin/go.exe vet ./internal/maintenance ./internal/cli ./internal/storage`: exit 0, no output. This is Windows scoped vet, not native Linux verification.

`git diff --check -- internal/cli/operations.go internal/cli/cli_test.go`: exit 0, no output. `gofmt -l internal/maintenance internal/cli/operations.go internal/cli/cli_test.go`: exit 0, no filenames. Formatting was applied only to owned Go files.

## Self-review, rulings and limitations

Self-review inspected the owned production paths and CLI diff after final scoped verification: raw Resolve ordering, private metadata before payload, exclusive identities, no main-file quota invention, immutable validation before migration, normal engine closes, journal refusal, source retention, cleanup identity checks, pause parsing, failure ordering, rollback recovery, and safe CLI output. No unresolved implementation issue was identified in that pass. Author self-review is weaker than the pending fresh independent Medium gate.

Ruling: Linux directory sync is a checked production operation; Windows development performs private metadata checks and makes no equivalent power-crash claim — required by the primary's added durability requirement — the cost of treating Windows evidence as stronger would be a false Linux acceptance claim.

Ruling: failed/partial pause writes remain conservative private marker state for explicit operator reconciliation, and existing valid markers are re-synced on retry — automatic repair/clear would violate the authorized paused-effects boundary — the operational cost is explicit reconciliation after a malformed marker.

Ruling: when final rollback directory sync fails, retain original recovery state and return manual-recovery guidance rather than report completed rollback — directory durability is unknown — the operational cost can be a missing active main with the original private pre-restore snapshot retained for recovery.

No manual deletion of actual or candidate WAL/SHM/journals occurs. Tests use dedicated synthetic private children of `t.TempDir`; SQLite alone owns its normal engine-created journal cleanup. Arbitrary readers cancel cooperatively between reads; a blocking reader can delay return, and no goroutine is abandoned to simulate cancellation. The privatefs boundary does not defend against a malicious process using the same OS account or previously acquired handles.

The primary owns full integrated tests, production/staged/history secret scans, publication, native Linux/race/security gates, live providers/WeChat/media/server work, and final acceptance. Those are not claimed here. No RSS/512MB, native arm64 runtime, OS power-loss atomicity, real credentials, real database migration, or live product acceptance is established by these synthetic Windows tests. No deferred minor finding was recorded.

## Fix round 1 — Important I1 backup cleanup ownership

Fresh independent review withheld specification/quality acceptance for I1: the storage backup failure branches removed the current pathname without retaining its created identity, and maintenance captured identity only after the delegated backup returned. A replacement preceding that observation could be treated as owned and deleted during failed validation. The primary read the named paths and accepted the finding. The original author self-review missed this gap; its earlier clean statement does not supersede the independent finding.

The primary additionally authorized only `internal/storage/backup.go` and new `internal/storage/backup_ownership_test.go` for a compatible fix. This round modified those two files, `internal/maintenance/backup.go`, `internal/maintenance/maintenance_test.go`, and this report. No other file was changed by this implementer. The primary's pre-fix full run (640 passes / 0 failures / 7 platform skips, full vet exit 0) finished before these production edits and does **not** verify the amended source.

The existing public `Store.Backup(ctx, destination) error` signature is preserved. New narrow `Store.BackupOwned(ctx, destination) (os.FileInfo, error)` returns identity from the checked owned open snapshot handle. Both methods use the same private implementation, retain exclusive-create handle identity before close, compare it before VACUUM, before reopen, against the reopened handle, and before success or cleanup. Failure cleanup removes a main file only when it still matches the original exclusive-create identity. Flush and permission operations use the checked opened handle. The consistent `VACUUM INTO` primitive, schema, quotas, routes and storage backend are unchanged.

Maintenance retains that returned identity before any later pathname observation, compares it before and after immutable validation, and uses the same ownership evidence for failed-validation cleanup. A changed identity is refused without deleting, chmodding or accepting the replacement. Genuinely owned failed destinations are still cleaned. All replacement/failure seams are private per-call values; no global hook or runtime dependency was added.

Verified Go 1.27.2 Windows source supplies an additional reason for opened-handle evidence: `os.Stat`/`Lstat` can defer loading file identifiers until `os.SameFile`, which can subsequently bind pathname-only metadata to a replacement. `File.Stat` captures identifier evidence from its open handle. This implementation returns the latter; `TestBackupOwnedReturnsOriginalHandleIdentity` checks that the returned evidence still identifies the original after rename/replacement.

Runtime RED command, before the product fix:

```powershell
& .tools/toolchains/go1.27.2-verified/go/bin/go.exe test ./internal/storage ./internal/maintenance -run 'TestBackupFailureCleanupRetainsReplacement|TestBackupRefusesReplacementBeforeReopen|TestBackupValidationCleanupRetainsReplacement' -count=1
```

Exit 1. Storage failed both vacuum-failure and exclusive-created-close-failure replacement-retention cases, and admitted replacement before reopen. Maintenance failed replacement before observation and during validation by deleting the unowned replacement. The same command passed both packages after the ownership fix. Additional fixtures verify cleanup of genuinely owned failure state, preservation of pre-existing destinations, returned original handle identity, replacement at the reopen boundary, refusal of a supported replacement after successful immutable validation, and cleanup of an owned invalid snapshot.

An expanded focused command (`go test ./internal/storage ./internal/maintenance ./internal/cli -run 'Backup|Restore|MaintenanceOutputFailure' -count=1`) then encountered a test-fixture Windows sharing violation when trying to rename a snapshot after a plain read/write handle was already open. That fixture was corrected to replace the path inside the reopen seam before returning the newly opened replacement handle. This models replacement between the earlier identity check and the reopen result, runs on Windows, and adds no skip or product permission relaxation. That failed focused run is not reported as a product GREEN result.

Final amended-source test command used the same verified toolchain and offline workspace environment documented above:

```powershell
& .tools/toolchains/go1.27.2-verified/go/bin/go.exe test -json ./internal/maintenance ./internal/cli ./internal/storage -count=1 |
  Set-Content .tools/reviews/maintenance-i1-tests-final.jsonl
exit $LASTEXITCODE
```

Exit 0: **284 named test/subtest entries passed, 0 failed, 2 explicitly skipped**. CLI: 22 passes / 0 skips; maintenance: 59 passes / 1 skip; storage: 203 passes / 1 skip. Package results: CLI pass (6.743s), maintenance pass (32.161s), storage pass (101.808s). Skips remain `maintenance.TestLockRejectsMovedDirectory` and `storage.TestExistingParentPermissionsArePreserved`; no new skip was introduced.

`& .tools/toolchains/go1.27.2-verified/go/bin/go.exe vet ./internal/maintenance ./internal/cli ./internal/storage`: exit 0, no output, against the amended source. Scoped `git diff --check` and `gofmt -l` checks also returned no findings/filenames. No repeated full integrated suite was run by this implementer; the primary owns integration and scans.

Fix self-review checked that no failure branch infers owned identity from the current name, replacement identities are retained before/after delegated validation, source identity evidence comes from opened handles, genuinely owned failed snapshots are still removed, raw filesystem errors are not returned, and tests assert real bytes/metadata retention rather than seam call counts alone. The files are frozen again for the primary's scoped fresh re-review against `.tools/reviews/maintenance-reviewed-src`. Specification/quality, native Linux, security scans, live evidence and final acceptance remain pending with the primary. This fix does not expand the existing same-OS-account adversary boundary or establish OS power-loss atomicity. No additional ruling or deferred minor finding was recorded.
