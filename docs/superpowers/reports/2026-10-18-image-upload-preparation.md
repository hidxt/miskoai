# Inactive bounded uploaded image preparation — 2026-10-18

Implemented in the isolated `.tools/reviews/image-upload-preparation-oct18-src` copy, continued on 2026-10-19 after the original usage interruption. Engineering remains **78%**. This report records implementation and scoped local Windows evidence only. Package40, native CI, live provider/CDN/WeChat tests, 512MB/server evidence and final acceptance receive no credit.

## Scope and source

The exact input is `image-groundwork-combined-root-oct18-src` and its 472-entry `image-groundwork-combined-root-oct18-source-manifest.json`. Every input entry's actual SHA256 and byte length was verified before copying. The copied original472 entries are checked again in the final identity result. No main checkout file or existing isolated source file was edited. The only new product/report paths are:

- `internal/replyimage/prepare.go`
- `internal/replyimage/prepare_test.go`
- `docs/superpowers/reports/2026-10-18-image-upload-preparation.md`

Work ran as gpt-6.1-sol / reasoning_effort medium, with no child agent. The prior usage-limited interruption was preserved; the primary later reported ordinaryUsageAllowed=true and authorized same-model continuation, without reset, purchase, substitution or retry loop. No Git operation, main adoption or publication occurred.

## Behavior and contracts

`New(dataDir string, cdn *media.CDN) (*Preparer,error)` borrows the exact existing CDN and owner-admitted private directory. It constructs no HTTP client, transport, admission slot, worker or database connection. Production dependencies are crypto/rand, existing `media.Encrypt`, and a default adapter that accepts a genuine nonnil `*media.Artifact` before calling this borrowed CDN's `Upload`.

`Prepare(ctx, recipient, LocalImage, plaintext, UploadLocator)` accepts only the existing trusted Manager callback's completed immutable fully validated PNG/JPEG plaintext. It does not independently establish validation for arbitrary callers. The future Core generation must keep the original Manager/image admission and exact Store lease held across preparation and send. The helper never reacquires that admission. It never closes or retains borrowed plaintext or closes the borrowed CDN.

Admission rejects nil/canceled context, nil/typed-nil dependencies, invalid UTF8/control/blank or >16KiB recipient, bytes outside1..4MiB, malformed lowercase SHA256, inconsistent artifact size, nonstatic or non-PNG/JPEG metadata, frames other than1, dimensions outside1..8192, >16000000 pixels, and invalid existing decode estimate bounds. This occurs before reads/encryption/network work. Existing owner directory privacy checks remain in the real encryption filesystem boundary.

Two separate16-byte crypto/rand reads supply AES and filekey values, serialized as lowercase hex32. A SectionReader bounded at captured size+1 streams actual plaintext through SHA256/MD5 and existing encryption's fixed small scratch; no whole-image plaintext copy or ReadAll is used. Before a locator call, the actual consumed length must equal captured bytes, computed SHA256 must match captured SHA256, and ciphertext length must equal16*(n/16+1). MD5 is wire checksum metadata only. The extra actual byte refuses a mismatched longer borrowed source. Short reads/errors, digest mismatch or ciphertext-size mismatch cause cleanup and no external call.

There is one locator call, then one upload using unchanged CDN URL/IP/redirect/response policy and the exact returned location fields. No automatic retry, replay, send, receipt, SQL, logging or retained key/URL is introduced. Successful upload must return bounded safe nonblank metadata. Only successful work, checked owned ciphertext Close and an uncanceled context produce ImageSend with encrypted query, key and padded size. Every error returns a zero descriptor. Owned ciphertext transfers even on encrypt-with-artifact error, closes once synchronously, and remains inside the lifetime until actual Close returns. Cleanup failure is a fixed sentinel and joins cancellation where present. Remote errors are normalized to fixed safe errors while preserving errors.Is for cancellation, deadline and auth expiry. No underlying poisoned text is returned.

The unexported per-instance dependency constructor exists solely for deterministic tests; no global hook or public alternate network configuration exists. Constructor fields and real production adapter were manually reviewed.

## Verification commands and actual results

The complete executable runner is `.tools/reviews/image-upload-preparation-oct18-run.ps1`. Each run executes the verified native `.tools/toolchains/go1.27.2-verified/go/bin/go.exe`; recorded version is `go version go1.27.2 windows/amd64`. Environment is GOTOOLCHAIN=local, GOPROXY=off, GOSUMDB=off, CGO_ENABLED=0, with the existing `.tools/go`, `.tools/gomod`, `.tools/gocache`, `.tools/tmp` paths for caches and scratch. All MISKOAI_ variables and DEEPSEEK_API_KEY/OLLAMA_API_KEY are cleared by name without reading or printing values. No real credentials or private data are used.

The actual underlying command in the isolated source is:

```
go test -json -count=1 -timeout=2m ./internal/replyimage
```

GREEN and final also run:

```
go vet ./internal/replyimage
```

Actual phase invocations and retained results:

| Invocation | Named PASS | Named FAIL | SKIP | test exit | vet exit |
|---|---:|---:|---:|---:|---:|
| `image-upload-preparation-oct18-run.ps1 red` |0|43|0|1|not run|
| `image-upload-preparation-oct18-run.ps1 red2` |30|21|0|1|not run|
| `image-upload-preparation-oct18-run.ps1 green` |51|0|0|0|0|
| `image-upload-preparation-oct18-run.ps1 final` |65|0|0|0|0|

Counts include named parent/subtests; package result is separate. All runs compiled. The first RED was a fixture failure, not semantic proof: `testDir` incorrectly asked EnsureDir to accept an existing ordinary t.TempDir directory, which Windows privacy checks refused. The fixture was corrected to create a dedicated missing private child via the existing helper. No production filesystem rule was weakened. RED2 is compiled semantic RED against a scaffold that returns media.ErrInvalid; it demonstrated missing successful encryption/descriptor behavior, valid constructor, transferred artifact cleanup, auth/cancel/deadline classification and synchronous joining. Initial invalid/admission cases passed against the scaffold. The original failed logs are retained and are not relabeled as success. RED scaffold bytes were not separately frozen; the actual RED logs remain authoritative for that run.

The first exploration command also failed before executing because PowerShell does not support the unquoted brace path used for rg. Corrected explicit paths succeeded. This parse failure changed no source. Tool outputs and this report preserve the limitation; no separate raw transcript artifact was fabricated.

Final test stderr and vet output are both zero bytes. The final test and vet processes exited0. Additional final tests strengthen exact one-call stage order, nil dependency refusal, both entropy-read failures, entropy cancellation, nil/typed-nil ciphertext, cleanup-plus-cancel errors.Is, unsafe returned query metadata, genuine-production-artifact admission and four distinct real production random values across two preparations. No production behavior changed between first GREEN and final; only tests were added/strengthened.

Synthetic tests generate genuine3x2 PNG/JPEG and use the real image validator, encryption, private artifacts and decryption. They compare recovered plaintext, actual SHA256/MD5, request data, descriptor padded length and key independence. Controlled callbacks replace external locator/upload behavior; no request goes to an external network. A separate untouched production upload adapter is exercised with the borrowed already-closed real CDN, so it refuses before network and still cleans its owned ciphertext. Held real borrowed reads and held ciphertext Close callbacks demonstrate cancellation cannot return before actual work is released. Tests check borrowed plaintext is never closed, owned ciphertext is closed, failed work yields zero descriptor, no network stage precedes actual hash/length admission, no repeated side effect, and poisoned errors do not escape.

The scope explicitly excludes the full/heavy native suite. No Core.Run, general service, live provider, CDN request, real WeChat, actual private schema2 access/migration, paid call, browser work, server/systemd/arm64 runtime, memory benchmark, public action or acceptance test was performed. No race claim is made. Native CI and integrated checks remain the primary's later gates.

## Evidence and self-review

Sibling evidence prefix is `.tools/reviews/image-upload-preparation-oct18-`. It contains actual per-phase JSONL, stderr, version, exits and applicable vet output, the executable runner, baseline manifest, frozen complete source manifest, owned-three manifest, identity result, real unified additions patch and evidence manifest. Each manifest entry records actual SHA256 and byte length. The evidence manifest excludes itself to avoid recursive identity. The source manifest includes all475 final public source/report entries; the owned manifest identifies exactly the3 new files. The unified patch adds those3 paths from /dev/null and is directly generated from their frozen actual contents, without reconstruction of any existing file.

Self-review read the complete implementation/tests and existing media encryption/CDN/attachment/library/wire contracts. Checked: constructor borrowing; real adapter type assertion; no admission reacquire; actual hash/count-before-locator; random independence; zero result on cleanup/cancel; transferred-error artifact cleanup; existing metadata bounds; error redaction; no message/SQL or detached worker path; scoped test exits. No known unresolved implementation defect was found in this self-review. This is not an independent review or integrated acceptance assertion.

The implementation is inactive until primary review and explicit integration under the already approved strategy. Immutable trusted input, owner-admitted directory, Store-generation lifetime and the existing Manager/image admission remain caller obligations. Cooperative synchronous work may wait for a noncooperative ReaderAt, entropy source, external callback or Close; there is no hard time/RSS guarantee or abandoned goroutine. After freezing identities and evidence, this task is quiescent for fresh review and primary checks.
