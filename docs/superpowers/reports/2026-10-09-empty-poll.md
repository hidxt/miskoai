# Empty raw poll storage refinement

2026-10-09. Implementation follows the root ruling in `.tools/reviews/empty-poll-brief.md`; storage/privacy/authentication policy was not independently selected by the Low implementation agent.

## Changed files and behavior

- `internal/storage/inbox.go`: remove only the zero-length input rejection; bind raw body through `COALESCE(?,X'')` so nil and empty byte slices persist as exact zero-byte BLOB evidence.
- `internal/storage/migrations.go`: remove only the zero-length receive-row rejection. BLOB type, maximum size, metadata validation and every other admission rule remain.
- `internal/storage/empty_poll_test.go`: synthetic regression tests for nil/empty persistence, SQL BLOB type and zero length, pending reads, close/reopen, quarantine/account blocking, prior cursor retention, completed snapshot backup/validation/restored reopen, schema version 2, global four-frame capacity and invalid body types/sizes.
- This report. No schema DDL or version change.

## RED evidence

Production code was unchanged when the tests were first run. The first attempted command set GOCACHE and temporary directories but omitted GOMODCACHE; Go attempted its default dependency download/cache path and failed with access denied before executing tests. This setup failure is not RED evidence. Subsequent commands used the existing workspace module cache with GOPROXY=off and GOSUMDB=off.

Environment for actual runs (PowerShell):

```powershell
$env:GOCACHE="$PWD\.tools\gocache"
$env:GOMODCACHE="$PWD\.tools\gomod"
$env:GOPROXY='off'
$env:GOSUMDB='off'
$env:GOTMPDIR="$PWD\.tools\tmp"
$env:TEMP=$env:GOTMPDIR
$env:TMP=$env:TEMP
```

Command: `go test ./internal/storage -run '^TestEmptyPoll' -count=1`.

First actual RED: exit 1, package 6.315s. Nil and empty lifecycle subtests both failed at RecordPoll with `invalid storage input`; capacity test failed recording its first empty frame.

After adding an independent preexisting schema 2 zero-BLOB snapshot fixture, the same command again exited 1, package 6.379s. Nil and empty RecordPoll failures persisted. `TestEmptyPollSchema2SnapshotAdmission` failed reopening the backup with `invalid storage input`; ValidateBackup already accepted that completed snapshot before the refinement. Body upper-bound/non-BLOB tests passed in RED, documenting retained behavior rather than newly missing behavior.

## GREEN and regression evidence

After the exact three-line product change and gofmt on the new test file:

- `go test ./internal/storage -run '^TestEmptyPoll' -count=1`: exit 0, `ok github.com/hidxt/miskoai/internal/storage 6.756s`.
- `go test ./internal/storage -count=1`: `ok github.com/hidxt/miskoai/internal/storage 57.695s`.
- `go vet ./internal/storage`: no diagnostics, exit 0. The storage regression test and vet ran sequentially in one PowerShell session, which completed with exit 0.
- `git diff --check -- internal/storage/inbox.go internal/storage/migrations.go internal/storage/empty_poll_test.go`: exit 0, no diagnostics. Tracked production diff reviewed: only the three specified lines changed.

Existing storage tests additionally cover inbox count/byte limits and cursor rollback, normal raw-frame capacity, account/scope isolation, receive admission limits, facts/messages quotas and backup/schema/WAL validation. New tests specifically prove zero-byte payloads still occupy all four global frame slots and quarantined empty evidence blocks both its original user and another user on the same account. SQL NULL remains rejected by the existing NOT NULL schema; empty text/integer and oversized BLOB rows are rejected before PendingPoll materializes them.

## Boundaries and freeze

All data and cursors are synthetic; fixtures use private child directories (0700) and completed snapshots (0600). Completed snapshot restoration here means validating, copying to a fresh private child, and opening it through storage admission. It does not exercise the CLI restore command or assert its replacement/rollback behavior. The test never calls ResolvePoll for empty malformed data; runtime decode/quarantine integration belongs to the root's separate task.

Windows host evidence only. No native Linux, Windows race, live provider, real WeChat or 512MB/RSS acceptance is claimed. Full repository tests are left to the root integration gate under the explicit scoped-test instruction. No credentials/private databases were read; no service was started; no staging, commits, pushes or delegation occurred. The initial default-cache setup mistake described above is disclosed; all actual test runs thereafter had module networking disabled.

Owned files are frozen for independent Medium review. No unresolved implementation issue was observed in the scoped checks.

## Review fix round 1: precise SQL NULL assertion

Fresh Medium review approved the product refinement with one Minor test observation: the SQL NULL case accepted any insertion error as evidence of NOT NULL enforcement. Root directed a minimal test-only correction.

`internal/storage/empty_poll_test.go`, lines 149–157, now requires insertion failure with the exact safe synthetic driver constraint text `constraint failed: NOT NULL constraint failed: poll_frames.body (1299)`. An unrelated SQLite error or a successful NULL insertion fails the test. Other invalid body cases retain their existing PendingPoll admission assertions. No imports, product lines or policy changed.

The actual affected test name is `TestEmptyPollKeepsBodyBoundsAndTypes`, rather than the provisional `TestEmptyPollRejectsInvalidBodies` name in the review handoff. With the same workspace caches/temp and GOPROXY/GOSUMDB disabled, `go test ./internal/storage -run '^TestEmptyPollKeepsBodyBoundsAndTypes$' -count=1` exited 0: `ok github.com/hidxt/miskoai/internal/storage 6.143s`. The full storage suite/vet were not repeated under the root's focused fix-round instruction; no imports were added. Product changes remain frozen/approved; cross-task service/CLI/root integration gates remain as stated above.
