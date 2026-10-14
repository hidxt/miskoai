# Complete library bundle CLI local implementation

2026-10-14, original implementer **gpt-6.1-sol / reasoning_effort medium**. Engineering progress remains **74% = 37/50** according to `docs/superpowers/project-progress.md`. This is a scoped synthetic Windows implementation/test handoff, awaiting primary review, fresh review and combined integration. It earns no package39 completion, native CI, live provider/WeChat, real server/arm64 runtime/512MB, power-crash or final acceptance credit.

## Scope and exact dependency

Read the project instructions, requirements/security/architecture/current task-memory-delivery records, decisions/testing guidance and `docs/superpowers/plans/2026-10-13-library-bundle-cli.md`. The primary supplied `.tools/reviews/library-bundle-core-fix1-root-reviewed-src` and its source manifest. All **422 dependency files** were checked for SHA256 and length before copying: **zero mismatches**, recorded in `dependency-verification.json`. The isolated working prefix is `.tools/reviews/library-bundle-cli-oct14-author`. The basis was never replaced with later primary Web/UI/schema work.

Only four deliverable paths are owned:

- `internal/cli/cli.go`: help and exact three-argument dispatch only.
- `internal/cli/library_bundle.go`: stopped complete-library maintenance and per-invocation failure seams.
- `internal/cli/library_bundle_test.go`: actual local lifecycle, media/database and failure fixtures.
- `docs/superpowers/reports/2026-10-13-library-bundle-cli.md`: this report.

Main checkout, accepted dependency, Core/maintenance/privatefs/config/operations files, other workers' sources, modules, LICENSE and notices were not modified. Primary owns adoption, project ledgers, independent review, combined checks and Git. No Git, network, private-data or live action was performed by this child.

## Resulting commands and lifecycle

`miskoai backup --library PATH` and `miskoai restore --library PATH` use the existing loaded configuration and actual `core.New(cfg, core.Dependencies{})`, then only actual `PrepareLibraryBundle` / `RestoreLibraryBundle` / `Close`. Existing two-argument `backup PATH` and `restore PATH` dispatch and `operations.go` bytes remain unchanged. Their SQLite format and 256MiB limit remain distinct from explicit all-scope version1 bundles limited to **321MiB**.

The complete mode inherits Core's existing valid administration-password requirement, with no bypass, dummy substitution or constructor-policy change. A missing account/provider-readiness state does not start an external operation. Constructors build local dependencies; neither command calls Core.Run, Web.New, a listener, generation startup or any provider/channel/CDN probe. Core's actual lifecycle lock refuses an active owner.

Backup admits an existing private parent and creates the destination exclusively before requesting the actual completed download. It records original identity, verifies fixed download filename and positive bounded size, copies using one 32KiB scratch buffer with cancellation checks, exact length and final EOF, then joins actual Download.Close. Sync, file Close, directory synchronization and final owner-private identity/size/metadata checks precede completion. A final read returning bytes plus a non-EOF error is refused rather than hidden by ReadFull/CopyN. On failure it closes resources and removes only a proven newly created private original, then synchronizes its parent. Replacements/uncertain identities are preserved with an observable safe cleanup error. A completed artifact is retained when later controller Close or output reporting fails.

Restore opens and admits the actual original private, regular, nonlinked, positive, at-most-321MiB source before Core construction or its storage effects. A cooperative bounded reader preserves final-byte errors and caps consumption to the admitted size plus one EOF probe. After actual Core restoration it rechecks the same handle and named original identity, size, mode and mtime, and joins actual source/Core Close on every branch. Cancellation occurring during a successful deferred cleanup remains observable. Once installation has occurred, reporting/close/source-identity failures preserve the effect and return a safe reconciliation error; they do not retry or undo restoration.

Success prints the actual Core `prior_retained` and `channel_paused` booleans and requires manual operator reconciliation. Help states the 321MiB cap, stopped-service/configuration requirement and that full restore stops generation before validation and may leave it stopped on failure. The stopped CLI itself starts no generation. There is no automatic resume/retry, image-clearing or multi-file atomicity promise. OS/source/private paths and arbitrary seam error text are never returned by the new command's safe errors.

## Actual verification and failures retained

All Go commands used exact `.tools/toolchains/go1.27.2-verified/go/bin/go.exe`, native Windows, `GOTOOLCHAIN=local`, `GOPROXY=off`, `GOSUMDB=off`. The established independent caches were `GOPATH=.tools/go`, `GOMODCACHE=.tools/gomod`, `GOCACHE=.tools/gocache`, and `GOTMPDIR/TEMP/TMP=.tools/tmp`, matching the supplied root Core runner and current TASKS recovery instructions. Environment names beginning MISKOAI and DEEPSEEK_API_KEY/OLLAMA_API_KEY were cleared by name before synthetic key placeholders were installed; values were never printed. Test configurations use only fresh synthetic directories/passwords/tokens.

The first meaningful compiled RED, `go test -json -count=1 -timeout=30s ./internal/cli -run '^TestLibraryBundleCLISemanticRoundTrip$'`, first proved actual Core.New, PrepareLibraryBundle, complete artifact read/Close and Core.Close, then failed the missing CLI dispatch with `unknown command; run miskoai help`: **one named failure, exit1**. Complete exact preimplementation source is retained as `red-src`, with its manifest. Initial empty roundtrip GREEN then passed.

Failures before final source are retained rather than replaced:

- Initial environment setup referenced nonexistent `.tools/go/tmp`; no Go test started. The empty redirected file and command-output note are retained; it is not a semantic RED or pass.
- Expanded-test compilation used the wrong envelope field name `Size`; actual build-failure JSON remains in `green-allscopes-attempt1.jsonl`. Correct field is `Bytes`. There was no exact complete source freeze before this typo correction; this evidence limitation is explicit.
- First expanded scoped run: all-scope and six restore subcases failed because the fixture wrongly uploaded with `Enabled=true`. Source inspection found actual Upload deliberately requires initially disabled metadata; the fixture was corrected to valid disabled synthetic metadata, without changing assertions or production policy. The source is preserved as `attempt1-src` and all original logs remain.
- Restricted-token hardlink fixture creation returned Access denied. It was not skipped and no ACL/check/assertion was relaxed. The finite synthetic runner subsequently used approved narrowly scoped `require_escalated` execution under the actual Windows owner; the actual hardlink refusal then passed.
- Normal-owner attempt2 had one real CLI failure: `TestLibraryBundleCLIPrecanceledAndCloseCancellation` reported `source cleanup cancellation hidden <nil>`. The deferred final cancellation join ran only when another error already existed. Both backup and restore now always join final `ctx.Err()` after actual cleanup, before the command's own deferred cancellation. Exact failing complete source is retained as `attempt2-src`, with original logs/exits and manifest. The corrected source passes the same assertion while retaining actual installed state.
- A multi-file patch application refused an unmatched expected line before applying changes; corrected explicit patches followed. No test/pass evidence is attributed to that refusal.
- A primary-reported quota interruption/resume was preserved in the conversation; before completing this handoff the child audited already-completed actual final logs/process output and did not repeat those scoped proofs or change model/effort/dependency.

Final command is `.tools/reviews/library-bundle-cli-oct14-author/run-scoped.ps1`, whose exact bytes and per-command logs/exits are frozen. It runs each new top-level selection separately with `go test -json -count=1 -timeout=30s ./internal/cli -run '^TestLibraryBundleCLI<selection>$'`: AllScopesCurrentImagesReusedID, SourceAdmissionBeforeCore, ActiveLockAndExistingDestination, ConfigRequirementAndSyntax, BackupFailuresAndCompletedRetention, RestoreFailuresAndEffectRetention, FormatConfusionAndLegacy, HeldDownloadJoinsBeforeReturn, HeldSourceJoinsBeforeReturn, PrecanceledAndCloseCancellation and SemanticRoundTrip. The eleven invocations produced **35 named/distinct PASS, zero FAIL, zero SKIP**, and all actual test exits0.

The relevant existing CLI selection is:

```text
go test -json -count=1 -timeout=30s ./internal/cli -run '^Test(BackupRestoreRoundTrip|RestoreRejectsUnrelatedSQLite|BackupCLIRefusesLifecycleOwner|MaintenanceOutputFailureRetainsCompletedState|BackupRejectsUnsafeSidecarBeforeMutation|RestoreRejectsCorruptBackupWithoutReplacingData|RunContextRejectsInvalidContextBeforeConfig|VersionAndUnknownCommand)$'
go vet ./internal/cli
```

Existing regressions produced **10 named/distinct PASS, zero FAIL/SKIP, exit0**. Scoped vet returned **exit0, zero-byte diagnostics**. Final combined scoped evidence is therefore **45 named/distinct PASS, zero FAIL/SKIP across12 actual test-package invocations**, all13 test/vet selections exit0, actual runner process exit0. This is a union of finite scoped invocations, not a whole-module suite.

Successful integration fixtures use the actual default Core, SQLite, library Manager, shared local Processor, real PNG/animated GIF, private filesystem and lifecycle lock. They restore two scopes into a destination with a reused ID and different old digest, reopen database and actual media through Core, inspect exact bytes and persistent pause state, then export/reimport again to prove rollback-only old content is excluded. Empty and actual schema4 bundles are covered. Failure fixtures wrap actual original handles/controller lifecycle, never fabricate a successful install callback. They cover preconstruction source refusal (missing/empty/dir/oversize/hardlink), malformed syntax/password requirement, format confusion/unchanged old SQLite mode, active lock/existing destination, write/short-write/sync/Close/directory-sync/download-Close/final-read errors, output/controller-Close after completed effects, replacement/mtime checks, cancellation and held I/O. Held-source/download unblock and join cleanups are registered before assertions.

## Frozen handoff and practical limits

`freeze.py` creates the complete final425-file source copy, four-owned manifest, exact unified owned diff, complete RED/failed-attempt source manifests, unowned identity verification, parsed test summary and independent copied evidence manifest, all with SHA256 and byte lengths. The supplied source421 unowned files remain exact. Report identity is external to this report in the four-owned manifest to avoid self-reference. Final deliverable is `final-src`; original/failing sources and evidence remain in the isolated prefix. After freeze the child quiesces and makes no further source/evidence edits.

Privatefs does not defend against a malicious process running as the same OS user; ordinary source/destination replacements are checked and preserved/refused. Borrowed I/O and output writers must cooperate; cancellation checks and bounded contexts do not establish hard OS blocking-time/RSS guarantees. The Windows directory-sync implementation verifies private metadata, not native Linux directory-fsync/power-cut durability. A source Close error can happen after installation, so an error may accompany a completed effect and requires reconciliation. Main source adoption, primary independent combined tests, fresh review, full static/secret/license gates, public/native checkpoint, live acceptance and 512MB targets remain outside this child handoff.
