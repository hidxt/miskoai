# Image validation root verification — 2026-10-10

Engineering **60%** =30of50 locally accepted work packages. Package33 covers the standalone allocation-aware validator, not product attachment/vision/library integration or live/resource/final acceptance.

## Frozen identity and independent review

Six frozen files: internal/media/image.go, image_png.go, image_jpeg.go, image_gif.go, image_test.go and docs/superpowers/reports/2026-10-10-image-validation.md. Root inspected the complete production code, tests and report and verified all six SHA256 hashes in both the working tree and immutable240-file public export before and after integrated checks. The complete62152-byte six-file patch SHA256 is `4d9ff356f19cac3a8f6cdeed254cb91d4784fc356b3e750477d84c0c1a15dfb3`.

Fresh independent reviewer image_validation_review, ONLY GPT-6.1 Sol Medium (`gpt-6.1-sol`, medium), returned Spec Approved and Quality Approved, zero Critical/Important/Minor findings. Reviewer read the complete implementation, fixtures, report and pinned Go1.27.2 decoder allocation paths, including D044 interlaced GIF cumulative allocation. No reviewer test reruns, private data or external operations.

Author scoped test JSON was independently parsed:187 named pass/0fail/4 privatefs privilege/platform skips; scoped vet exit0. Historical runtime RED evidence remains tool transcripts, not fabricated persisted logs; the author report states this explicitly.

## Root commands and actual results

Root ran verified native Windows Go1.27.2 with offline dependencies against `.tools/reviews/image-validation-reviewed-src`, containing current accepted CDN plus frozen image files and no private data or readiness implementation.

- `go test -json ./... -count=1`: exit0, **961 named pass/0fail/4skip**,14 test-package pass and2 packages without tests. Completed log `.tools/reviews/image-validation-root-tests.jsonl`.
- `go vet ./...`: exit0, empty diagnostic file `.tools/reviews/image-validation-root-vet.txt`.
- Four existing skips: TestLockRejectsMovedDirectory, TestPrivateDirNeverRepairsExisting, TestPrivateSymlinkTraversal, TestExistingParentPermissionsArePreserved. These are platform/permission boundaries, never passed evidence.
- Complete production gosec Windows:65 files/12131 lines,57 warnings (9MEDIUM/48LOW),0Goerrors/0nosec, exit1 for retained warnings.
- Complete production gosec Linux amd64 static configuration:65 files/11949 lines,60 warnings (2HIGH/11MEDIUM/47LOW),0Goerrors/0nosec, exit1 for retained warnings. This is static configuration on Windows, not Linux execution.
- Normalized source/rule/details/code fingerprints versus accepted corrected CDN: **0new/0removed** for both targets. Existing Linux UID conversion HIGH dispositions remain at the unchanged64bit ABI boundary; no suppressed warnings or clean-scan claim.

Root independently parsed all final events, package outcomes, scanner statistics and fingerprints and rechecked six owned hashes. All synthetic fixtures use dedicated development paths. No real credentials/database/chat/provider/WeChat/CDN call or general service startup occurred.

## Scope and remaining obligations

Input4MiB, dimensions8192, visible16M pixels, GIF32frames and64MiB decoder-buffer estimate are conservative admission rules. PNG/JPEG full Decode and GIF full DecodeAll occur only after format-specific structural and allocation preflight; APNG refuses. D044 includes the sum of every interlaced frame's second pixel allocation. Errors are fixed, cancellation remains synchronous and caller-owned immutable completed bytes are not closed or replayed.

Estimates depend on pinned Go1.27.2 decoder allocation structure, not total heap/allocator/GC/RSS or a hard two-second guarantee. An arbitrary blocked ReaderAt/decoder cannot be forcibly interrupted. Compressed validity follows the pinned decoder's accepted semantics, including disclosed GIF LZW tolerance.

Downstream requires one outer admission spanning actual download/decrypt/validation completion, metadata digest/length checks, encoded-request overlap accounting, shared provider admission, GIF vision policy, private library quotas and channel integration. Native image-specific CI remains pending publication; exact CDN539501c/run38050566507 passed but excludes image. Paid cloud allowance spent; fresh strictACK marker unanswered; real media/reconnect/auth-expiry, arm64 execution, actual512MB workload, final security/licenses/UI/release acceptance remain open.
