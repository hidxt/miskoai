# Attachment-storage native fixture correction

2026-10-11. Author gpt-6.1-sol, reasoning_effort medium. Engineering progress **62%** unchanged. This task corrects two synthetic test-directory setups only; it does not alter storage production, privacy, schema or queue behavior. Implementation/test-fixture correction is complete; Windows verification is recorded below, corrected native Linux/race confirmation is pending root review/publication/CI. Live tests and final acceptance remain pending.

## Exact failure and diagnosis

Published source identity: `0e7dd94584fb5cdbf7f9e2c4d1e0194cb3bcb0ff`. Root-saved public native evidence: `.tools/reviews/attachment-storage-native-annotations.json` and `.tools/reviews/attachment-storage-native-jobs.json`, [CI38070812447/job114267657835](https://github.com/hidxt/miskoai/actions/runs/38070812447/job/114267657835). Unit/integration step actually failed: TestAttachmentSchemaLegacyCompatibility/1,/2,/3,/4 each reported `schema4_test.go:55: invalid storage input`, plus the parent failure, and the command exited1. These five named failures are the retained native semantic RED, not a complete whole-CI named failure count. Vet, native Linux race, vulnerabilities, history policy, Linux binary cross-build and setup-go post-step were skipped; checkout cleanup and job completion succeeded. No later gate is represented as passed.

The exact failed fixtures put their raw-SQL database directly in `t.TempDir()`. I read verified Go1.27.2 `src/testing/testing.go`1577–1640: TempDir's numbered returned child uses `os.Mkdir(dir,0o777)`. A typical Unix umask022 leaves that returned directory0755. I read unchanged storage `privatePath`: `Open` starts with privatePath and rejects an existing Unix DB parent whenever permissions have any0077 bits, without chmodding unrelated existing parents. Its permission-bit guard excludes Windows, explaining the local/native distinction by source tracing. The actual failed runner umask and resulting directory mode were not recorded. The diagnosis therefore remains a strong source-traced hypothesis until corrected native CI confirms; Windows tests cannot execute the Unix permission branch.

The second raw-SQL constructor, TestAttachmentLegacyRefusalPreservesPinnedSchema, had the same directory setup. Its expected ErrInvalid could therefore pass on Unix at the parent-permission boundary instead of exercising malformed legacy data admission. Its setup is corrected alongside the visibly failing constructor.

## Exact scoped change

Only internal/storage/schema4_test.go is changed:

- Add the runtime import.
- In TestAttachmentSchemaLegacyCompatibility and TestAttachmentLegacyRefusalPreservesPinnedSchema, replace each direct `filepath.Join(t.TempDir(),filename)` with a fresh `private` child created using `os.Mkdir(dir,0700)` and checked construction errors.
- Stat that child and assert exact0700 permissions on Unix. This conditional assertion does not skip any fixture or test on Windows.
- Join the unchanged database filename under that child and explicitly require `privatePath(path)` admission before constructing the raw-SQL database. This prevents malformed-data refusal from being satisfied only by an inadmissible parent.

No unrelated parent is chmodded. No new filesystem helper or broad refactor was introduced. Exact legacy schema construction, all version loops, backup/migration assertions, byte-identical refusal and no-sidecar assertions remain unchanged. TestAttachmentSchemaRefusalImmutable uses the established testStore private parent and is unchanged. All production storage files, original Task1 report, previous frozen manifests/copies/logs, normalization/service and root ledgers remain unchanged.

Before-correction schema4_test.go SHA256 was `f7ddbea538149e07f6652a7efa7363d8f56f3344bbe01038b6ed0527b2bd9217`; corrected SHA256 is `4eaebcb1f4b3312b1e514d37cfcda0a5bc06ad5360db1b687577b48bf857a55c`. Original Task1 report SHA256 remains `a35bf39bb531c4ff3e4a065a1031fc61f985f31a382987c31b7ada3d17eea36e`.

## Actual Windows verification

Verified toolchain `.tools/toolchains/go1.27.2-verified/go/bin/go.exe version` returned `go version go1.27.2 windows/amd64`, exit0. Executable below is that exact verified Go path; no toolchain download/network operation occurred.

Setup removes all process MISKOAI_* entries by name. It then supplies a synthetic data directory, management password and provider credentials. The configured variable names are MISKOAI_DATA_DIR, MISKOAI_ADMIN_PASSWORD, DEEPSEEK_API_KEY and OLLAMA_API_KEY.

The data directory value is `.tools/tmp/attachment-native-fixture-synthetic`. The deterministic management fixture value is `synthetic-password-123`. Both provider fixtures use the fixed value `synthetic-key`.

 GOTOOLCHAIN=local, GOPATH=.tools/go, GOMODCACHE=.tools/gomod, GOCACHE=.tools/gocache, GOTMPDIR/TEMP/TMP=.tools/tmp, GOPROXY=off, GOSUMDB=off. No inherited credential file/default private path is used. Fixtures are temporary synthetic SQL data and existing connection cleanup registrations are retained.

| Actual command suffix | Log | Complete result |
| --- | --- | --- |
| `test -json ./internal/storage -run '^(TestAttachmentSchemaLegacyCompatibility\|TestAttachmentLegacyRefusalPreservesPinnedSchema\|TestAttachmentSchemaRefusalImmutable\|TestSchema1SnapshotMigratesAndForeignZeroFails)$' -count=1` | `.tools/reviews/attachment-storage-native-fixture-targeted.jsonl` | exit0;14 named pass/0fail/0skip;1packagepass |
| `test -json ./internal/storage -count=1` | `.tools/reviews/attachment-storage-native-fixture-storage.jsonl` | exit0;246namedpass/0fail/1 existing Windows platform skip;1packagepass |
| `vet ./internal/storage` | `.tools/reviews/attachment-storage-native-fixture-vet.txt` | exit0;empty0byte log |

Named counts include parents/subtests. No artificial Windows runtime RED was created by weakening production or pretending to exercise Unix bits. Actual native RED remains saved above; corrected Windows GREEN is a separate evidence class. Corrected native Linux/race requires the primary's fresh review and published CI.

Production hash baseline: `.tools/reviews/attachment-storage-native-fixture-production-before.json`, covering all12 non-test Go files under internal/storage. After-check `.tools/reviews/attachment-storage-native-fixture-production-after.json` verifies all12 production paths/bytes/SHA256 unchanged after complete storage+vet. All12 also matched after the targeted run. Prior report hash is unchanged. LICENSE and existing content preserved.

No actual private DB/auth/credential values, startup/restore/migration, ordinary service, external network request, Git operation, scanner, full-root test/build, alternate model/effort or delegation occurred. Root owns fresh review, integrated checks, publication and exact native acceptance.


## Final freeze and limitations

Status DONE. Actual targeted exit0/14namedpass/0fail/0skip; complete storage exit0/246namedpass/0fail/1existingplatformskip; scoped vet exit0/empty0byte. The full storage skip is TestExistingParentPermissionsArePreserved (existing Windows limitation). No new test skips were added.

New owned copies: `.tools/reviews/attachment-storage-native-fixture-owned/internal/storage/schema4_test.go` and `.tools/reviews/attachment-storage-native-fixture-owned/docs/superpowers/reports/2026-10-11-attachment-storage-native-fixture.md`. New two-file freeze manifest: `.tools/reviews/attachment-storage-native-fixture-owned-hashes.json`, records path/bytes/sha256/frozen_path. Separate immutable completed-log copies under that new directory's evidence child and `.tools/reviews/attachment-storage-native-fixture-evidence-hashes.json` record the targeted/full-storage/vet evidence. Original Task1 snapshots/manifests/logs are retained unchanged. This report's own hash is recorded externally in the new manifest and handoff, avoiding a self-reference.

Windows results verify the corrected SQL fixture setup and unchanged storage assertions. They do not validate Unix permission-bit behavior, native Linux race, failed runner umask or corrected native CI. The source trace and actual native RED support this correction, with confirmation reserved for root fresh review/integrated/native publication. No unresolved Windows fixture failure remains.

## Report-only scan disposition (docfix1)

The root's initial complete261-file frozen Gitleaks scan exited1 with one generic-api-key finding at this report's original line30. Redacted metadata remains in `.tools/reviews/attachment-integrated-gitleaks.json`. Root verified the finding was deterministic synthetic fixture wording, with no real credential leak. This prose correction separates configuration names from the already documented fixed synthetic values. The original scan, report copies and two-file manifest remain retained. Corrected scanning is pending root verification; no suppression, allowlist, scanner-policy or source/test change was introduced. No test rerun is required for this documentation-only edit.