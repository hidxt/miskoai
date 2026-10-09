# Runtime configuration Task2 implementation report

2026-10-09. Implementer: gpt-6.1-sol, reasoning effort Medium. Base supplied by root: 85d60c8b323c97ab10cf343862ec0c84edba02dc. Implementer source is frozen for fresh independent Medium review. No staging or commits.

## Scope and implementation

Read AGENTS.md, REQUIREMENTS.md, SECURITY.md, ARCHITECTURE.md, TASKS.md, MEMORY.md, DELIVERY.md and the authoritative isolated `.tools/reviews/runtime-config-brief.md`, plus applicable Superpowers TDD, test-quality, systematic-debugging, subagent-development and verification instructions. No future full plan or unrelated private payload was read.

- Added value-only Settings for the eight canonical nonsecret fields. Legacy six-field and recognized partial settings merge absent defaults. Text and vision both default to deepseek-flash. Context defaults are 24576 bytes / 32768 estimated tokens; bounds and existing endpoint constructors are reused without HTTP calls.
- LoadSettings reads at most 16 KiB through privatefs after privacy verification. Shared strict object parsing rejects wrong roots/types/null, duplicate decoded keys including escaped duplicates, unknown fields, case/Unicode-fold aliases, trailing JSON, invalid UTF-8 and malformed UTF-16 escape pairs. Valid escaped strings retain their decoded values exactly.
- Load distinguishes absent data/settings from unsafe existing parents/files. Missing-path checks do not create directories, inspect all existing ancestors, refuse symlink/irregular objects and changed resolved paths, and reject Windows aliases before filepath.Abs can normalize them. Existing dedicated directories require privatefs.CheckDir; existing payloads are read only through privatefs.Read.
- Explicit environment presence overrides persisted values. Empty/invalid string or integer overrides error. Config captures independent desired/effective Settings values and exposes only nonsecret override-name/boolean metadata. Credentials, data directory and admin password are excluded from Config JSON.
- Explicit MISKOAI_CREDENTIALS_FILE reads the exact uppercase DEEPSEEK_API_KEY/OLLAMA_API_KEY schema within 8 KiB. Neither api-keys.json nor a secrets path is discovered. Present valid environment keys override file keys; both present valid keys avoid any file payload read. Explicit empty path/key/password is rejected. Keys are nonblank valid UTF-8, <=1024 bytes and contain no ASCII controls. Missing sources permit diagnostics. Admin diagnostic validation allows short valid passwords while the future serve 16-byte readiness gate remains separate.
- LoadAuthorization uses only weixin-auth.json, four exact existing keys bot_token/account/allowed_user/base_url, 64 KiB cap, token 16 KiB/scope 256-byte bounds, and official Weixin endpoint validation. Missing has its own ErrAuthorizationMissing; invalid existing input returns safe constant ErrAuthorization. Authorization is excluded even from its own public JSON.
- InitSettings preserves exclusive no-overwrite creation. SaveSettings validates/serializes before payload writes, creates a private random same-directory temporary, writes/syncs/closes, rechecks the private destination, and calls os.Rename. Failed validation, unsafe destinations and an observed Windows rename failure retain old bytes; failed-save temporary files are removed.
- CLI init persists only Settings through InitSettings. The WeChat live probe changes only the loading/conversion boundary; fixed marker, one-send, dedup/state behavior remain untouched. Invalid authorization refuses before creating/opening the database or producing output.

## RED and intermediate observations

All commands used the verified native Windows Go1.27.2 and offline workspace caches. Common PowerShell environment preamble:

```powershell
$env:GOTOOLCHAIN='local'
$env:GOPROXY='off'
$env:GOSUMDB='off'
$env:GOPATH="$PWD/.tools/go"
$env:GOMODCACHE="$PWD/.tools/gomod"
$env:GOCACHE="$PWD/.tools/gocache"
$env:TEMP="$PWD/.tools/tmp"
$env:TMP=$env:TEMP
```

Meaningful runtime RED against old code, before production edits:

```powershell
& .tools/toolchains/go1.27.2-verified/go/bin/go.exe test ./internal/config -run 'TestStoredSettingsAreRuntimeConfiguration|TestPresentEmptyRuntimeValuesReject' -count=1
```

Exit1: stored model was ignored (`saved-model` expected, `deepseek-flash` actual), and explicitly empty output-budget/key/credential-path/admin-password values were accepted. Five failing assertions covered missing runtime behavior rather than undefined APIs.

The first implementation passed all three scoped packages. Expanded coverage then revealed two test-fixture errors: Windows Setenv rejects NUL and rewrites invalid UTF-8 through UTF-16 before the loader sees it. Replaced those environmental cases with private JSON fixtures so the production parser receives the intended bytes; controls supported by environment APIs remain environment tests. No production weakening.

A later independent regression RED:

```powershell
& .tools/toolchains/go1.27.2-verified/go/bin/go.exe test ./internal/config -run TestSettingsMissingWindowsAliasesAreUnsafe -count=1
```

Exit1: `missing.` was accepted as absence. Root cause: Windows filepath.Abs normalizes trailing dots before the initial lexical check. Fixed raw-path checks in Load, the absent-file helper, InitSettings and SaveSettings before normalization/creation. This is separately covered from the actual existing-parent privacy checks. A subsequent expanded run still showed the same alias failure before the raw-path refinement; final run below is clean.

## Final scoped evidence

Final covering files: internal/config/config_test.go, settings_test.go, credentials_test.go; internal/cli/cli_test.go and weixin_probe_test.go; existing privatefs/CLI coverage.

```powershell
& .tools/toolchains/go1.27.2-verified/go/bin/gofmt.exe -w internal/config
& .tools/toolchains/go1.27.2-verified/go/bin/go.exe test ./internal/config ./internal/cli ./internal/privatefs -json -count=1 > .tools/reviews/runtime-config-tests.json
& .tools/toolchains/go1.27.2-verified/go/bin/go.exe vet ./internal/config ./internal/cli ./internal/privatefs
```

Final test command exit0: **95 passed test/subtest entries, 0 failed, 5 explicit skips**. All three package terminal results passed. Final vet exit0, no output. Scoped `git diff --check` exit0, no output.

Skipped on this Windows host:

- TestPrivateJunctionTraversalWindows: host denies synthetic junction creation.
- TestPrivateDirNeverRepairsExisting: Unix permission semantics only.
- TestPrivateSymlinkTraversal: Unix-only symlink fixture.
- TestPrivateHardlinkAndEmptyProtection: Unix-only hardlink/mode fixture.
- TestConfigurationRejectsSubstitutionAndPreservesTarget: host denies synthetic junction creation.

Windows host coverage passed for broad-ACL directory/file rejection, owner-only fixture reads, raw alias absence/init/save refusal, exclusive Init and successful Save replacement. TestSaveSettingsFailedReplacementKeepsOldWindows held an os.Open handle denying delete sharing; Save failed at replacement, old bytes remained exact, and private temporary cleanup completed. Windows os.Rename atomicity is **not claimed**: Go explicitly gives no non-Unix atomic guarantee. Native Linux targets use same-directory atomic rename; native verification remains root-owned/pending.

Synthetic canary assertions passed in Config/DesiredSettings/EffectiveSettings/override serialization, settings/init output, malformed private-input errors and broad-file errors. Public messages never included the synthetic-secret-canary. Credential/key/auth payloads appeared only in synthetic test fixtures; no actual workspace data/settings/auth/API-key/database was read by these tests. Existing unknown-command CLI test now points to a synthetic absent directory to avoid default workspace loading.

## Changed files and self-review

Exactly owned implementation/fixture files:

- internal/config/config.go
- internal/config/config_test.go
- internal/config/settings.go (new)
- internal/config/settings_test.go (new)
- internal/config/credentials.go (new)
- internal/config/credentials_test.go (new)
- internal/cli/operations.go
- internal/cli/weixin_probe.go
- internal/cli/cli_test.go
- internal/cli/weixin_probe_test.go
- docs/superpowers/reports/2026-10-09-runtime-config.md (this report)

Self-review checked the frozen source and owned diff: validation before I/O, complete input caps, exact decoded key membership/duplicate detection, surrogate preservation, safe constant errors/zero values on failure, environment presence precedence, private Config/Authorization JSON exclusions, stable Settings snapshots, exclusive init, sync/close before replacement, unsafe-target refusal and failed-save cleanup. No edit to privatefs, provider, storage, service, root ledgers, LICENSE, git or actual private payloads. Root owns its simultaneous documentation changes; they are excluded from this task's change list.

A related observation was sent to root: privatefs.dedicated currently normalizes before its own Windows alias check; Task2 guards its own raw inputs, and this report does not claim to have modified the already-reviewed privatefs boundary.

Implementation: Task2 implemented and frozen pending fresh independent review. Local tests: scoped Windows evidence above. Native CI: not run by this worker; preceding project checkpoint c17c6ea remains prior evidence. Live provider/WeChat: none run. Final acceptance: pending root review/integration/native gates and broader Core/Web/media/live/512MB/release work. No scope-policy changes, network requests, live probe, private-data migration, staging, commit or push.

## Shared Resolve follow-up and final freeze

Root resumed this same gpt-6.1-sol Medium implementer after fresh independent Medium approval of the shared privatefs.Resolve correction (0 findings), root production-diff inspection, and root privatefs15pass/0fail/4skips/vet0. That prerequisite is root-provided evidence, not this worker's assertion.

Replaced the task-local copied validMissingPath validator with privatefs.Resolve in Config.Load, absentPrivateFile, InitSettings and SaveSettings. Resolve validates raw spelling before absolute normalization and supplies only a lexical absolute path. The missing-path ancestor inspection and existing dedicated directory/file privacy checks remain in place; no permission or symlink safety is inferred from Resolve alone. Private credential reads already use the shared privatefs boundary. Model, password, key, strict JSON and environment-presence rules remain unchanged.

Added TestDefaultAndRelativeDataPathsUseSyntheticCWD. It changes the test process working directory temporarily to a fresh synthetic test directory and restores it, unsets MISKOAI_DATA_DIR for the default ./data case, validates ./data and Windows .\data, then initializes/saves/reloads settings using those relative spellings. No actual workspace default data path is accessed. Meaningful RED before the resolver conversion:

```powershell
& .tools/toolchains/go1.27.2-verified/go/bin/go.exe test ./internal/config -run TestDefaultAndRelativeDataPathsUseSyntheticCWD -count=1
```

Exit1: valid .\data was refused with `invalid data directory` by the copied Windows validator. After conversion, the relative and absent-environment default cases pass, while the existing raw Windows alias refusal cases continue to pass.

Final commands reused the offline native Go1.27.2/cache/TEMP preamble above:

```powershell
& .tools/toolchains/go1.27.2-verified/go/bin/gofmt.exe -w internal/config/config.go internal/config/settings.go internal/config/settings_test.go
& .tools/toolchains/go1.27.2-verified/go/bin/go.exe test ./internal/config ./internal/cli ./internal/privatefs -json -count=1 > .tools/reviews/runtime-config-tests-resolve.json
& .tools/toolchains/go1.27.2-verified/go/bin/go.exe vet ./internal/config ./internal/cli ./internal/privatefs
```

Test exit0: **104 passed test/subtest entries, 0 failed, 5 explicit skips**; all three terminal package results passed. The count includes root's newly reviewed shared-resolver tests plus this relative-path regression. Vet exit0/no output. Owned-source git diff --check exit0/no output. Skips are the same five named in the preceding report: two host-denied junction fixtures and three Unix-only fixtures. Existing synthetic public canary, strict JSON, credential precedence, private ACL, exclusive Init, Windows failed replacement preservation and cleanup checks all ran again and passed.

Only this worker's config.go, settings.go, settings_test.go and owned report changed during this follow-up. No privatefs or root-document edits, staging/commits, private-data access or external/live calls. All copied validMissingPath references are removed; four explicit shared Resolve call sites remain in config.

Historical native CI: root reports exact85d60c8 checkpoint succeeded. That published baseline is preceding native evidence; it does **not** establish native CI/race or final acceptance for the new shared Resolve/config source. Current source requires root fresh independent Medium review, integrated checks and later native gate. Live provider/real WeChat/512MB/server/final release evidence remains unchanged and outside this worker's task. Task2 source/report are now frozen again for review.
