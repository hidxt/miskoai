# Image validation Task1 author report

2026-10-10. Implementer: GPT-6.1 Sol Medium (`gpt-6.1-sol`, reasoning effort `medium`), no delegation. Engineering progress at freeze remains **58%**, with no package33 acceptance credit inferred. This report records implementation and Windows host scoped synthetic evidence only. Fresh independent review, primary integrated checks, native CI, live media/provider/WeChat evidence, real512MB resource measurements and final acceptance remain separate gates.

## Owned scope and implementation

Created only `internal/media/image.go`, `image_png.go`, `image_jpeg.go`, `image_gif.go`, `image_test.go` and this report. Shared docs, accepted codec/CDN/transport, dependencies, provider, CLI, channels, UI and storage were not edited. No Git, network, private data, actual database or global configuration operations were performed. The test runner's temporary synthetic fixtures and offline build caches remain under the existing project-private `.tools` roots.

`ValidateImage(ctx context.Context, input io.ReaderAt, size int64) (ImageInfo, error)` consumes immutable completed bytes synchronously, leaving caller file ownership and seek position intact. It returns only format, width, height, frame count, animation flag and conservative estimated decoder-buffer bytes. It does not retain decoded pixels, fetch, transform, transcode, encode base64, send, persist library metadata or claim provider acceptance.

The implementation refuses nil context/reader, nonpositive size, input above4MiB, dimensions outside1..8192, canvas above16,000,000 pixels and conservative decode-buffer estimates above64MiB. Signatures select PNG/JPEG/GIF; unknown signatures are unsupported. Each scanner traverses bounded structure before the exact full decoder runs; no DecodeConfig-only success path exists. Parser scratch is fixed and small (4096-byte buffered reader, 4096-byte skip/chunk scratch and small fixed headers), with no allocation controlled by an unvalidated declared length and no unbounded frame list.

- PNG: exact initial13-byte IHDR; legal depth/color/interlace;4096-chunk cap; lengths, chunk names and CRC; duplicate IHDR, noncontiguous IDAT, missing IDAT, malformed/nonterminal IEND and trailing-data refusal. acTL/fcTL/fdAT are explicitly unsupported. Estimate is `16*w*h + 2*(8*w+1) + 1MiB`, using checked buffer products/sums after dimension admission.
- JPEG: bounded full marker/segment/entropy traversal through exact EOI, byte stuffing/restart/fill handling and4096-marker cap; one SOF0/SOF1/SOF2, precision8,1/3/4 unique components and sampling1..4; reject repeated/unsupported structures. All component maxima and sampling products contribute to padded plane/conversion and progressive coefficient estimates. MCU rounding additions are safe after checked dimensions/sampling; buffer products and admission sums are checked.
- GIF: logical canvas/tables, allowed extensions/subblocks, every nonempty in-canvas descriptor,32-frame cap, exact trailer and no trailing bytes. Every frame contributes retained pixels; every interlaced frame also contributes its full original/replacement pixel buffer. D044 estimate is `sumFramePixels + sumInterlacedFramePixels + 1MiB + frameCount*16KiB`. No compositor is allocated.

Only successful full PNG/JPEG Decode or GIF DecodeAll with matching format/dimensions/frame count yields ImageInfo. Safe sentinel errors are media invalid/unsupported/limit/IO; cancellation/deadline identity is retained. A private per-call decoder seam exists solely as an internal testable boundary, with no exported mutable hook or detached worker. Standard GIF decode wraps reader errors using formatted text; the private reader remembers a fixed IO flag so the wrapper returns ErrIO without exposing/parsing raw errors.

## Pinned source and limits

The verified toolchain reports `go version go1.27.2 windows/amd64`; VERSION identifies go1.27.2. Decoder semantics were independently read from its local source, alongside the approved plan, attachment spec and root research/D044:

- JPEG `dct.go` defines64int32 coefficient blocks; `scan.go` pads MCU planes and allocates progressive coefficient arrays for every scanned component; `reader.go` can retain original planes/black pixels while allocating CMYK/RGBA conversion. MaxH/MaxV can come from non-luma components. The estimate includes all component coefficient storage even if a later scan is malformed or never arrives. Grayscale nominal sampling can overestimate the decoder's normalized1x1 sampling, which is conservative.
- PNG `reader.go` retains a full image and interlace pass images and allocates two row buffers; the conservative16-byte-per-visible-pixel term also avoids assuming immediate reclamation of earlier pass buffers.
- GIF `reader.go` retains every decoded frame, and `uninterlace` allocates a second dx*dy array before replacing m.Pix. This implementation identified the omission in the initial GIF plan; root independently confirmed it and issued D044. Summing *all* interlaced originals avoids assuming early GC between frames.

These version-specific formulas are admission estimates for decoder buffers plus conservative fixed overhead. They are not proofs of the entire Go heap, allocator/GC overlap, RSS,512MB service behavior, arbitrary future decoder versions or elapsed time. The internal2s child context is cooperative, checked in scanner/reader and before/after decode; it cannot interrupt arbitrary blocking ReaderAt or pure decoder computation. The actual job always joins before returning and releasing any future outer admission. Caller must provide immutable completed bytes and hold the downstream shared single media/parser admission for the complete operation. No such shared integration was added here.

Full decode validity follows the pinned standard decoder's accepted format semantics, including its documented GIF LZW tolerance; it is not a stricter independent compressed-stream specification or cryptographic authenticity check. GIF validation proves retained animation contents, not compositing, native sticker capability or animated delivery. APNG, GIF plain-text/unknown extensions and unsupported JPEG/PNG structures refuse honestly. Base64/JSON overlap, digest/length checks, upload/send, library quotas, GIF vision policy and real backend evidence remain downstream work. Structural/adversarial fuzz seeds and deterministic mutations ran; no prolonged random fuzz campaign, race run, benchmark or RSS measurement is claimed. No Go source was copied or adapted; structural scanners and tiny synthetic codestreams were independently written, with existing LICENSE preserved.

## Runtime TDD evidence

Earlier evidence was preserved across the scheduling HOLD/reboot and was not redone. The scaffold compiled; its runtime failures were genuine behavioral failures, not setup errors.

1. Initial `go test -json ./internal/media -run '^TestImagePNGAndJPEGCompleteValidity$' -count=1`: exit1, png and jpeg complete tiny valid fixtures refused with zero Info/ErrInvalid; parent and both named subtests failed, package elapsed10.272s. Actual tool transcript chunks `9bb94e`/`ec25c1` preserve the run output; the runtime failures are in `ec25c1`.
2. Compiled extended scaffold `go test ./internal/media -run '^TestImage' -count=1`: exit1, elapsed6.005s. All six planned groups failed: TestImagePNGAndJPEGCompleteValidity, TestImageProgressiveSamplingEstimate, TestImagePNG16BitInterlaceEstimate, TestImageGIFAllFramesBounded, TestImageCheckedArithmeticAndLimits, TestImageCancellationAndOwnership. Representative actual failure: `predecode: called=false ... err=media invalid want=media limit`. Transcript chunk `861fc4`.
3. Initial product implementation after those REDs: same focused command exit0, elapsed6.006s, transcript `72d996`. Work was then preserved on HOLD.
4. Resume expanded test setup initially had an unused bufio import: compile exit1 (`53e755`), corrected immediately. This setup failure is explicitly excluded from TDD evidence.
5. Expanded focused command before the IO correction: runtime exit1, elapsed6.025s. `TestImageReaderErrorsAreSafe/gif1` failed with `unsafe or misclassified IO: {Format: Width:0 Height:0 Frames:0 Animated:false EstimatedDecodeBytes:0} media invalid`. Transcript `5fa1ab`. The test's synthetic reader fails only on the decoder's second traversal; source confirmed GIF `%v` wrapping erased error identity. The fixed private IO flag made the complete focused suite pass, exit0/5.962s (`eb8963`).
6. Dedicated D044 regression: temporarily remove **only** the `interlacedPixels` argument from the owned GIF estimate; run `go test ./internal/media -run '^TestImageGIFInterlacedCumulativeEstimate$' -count=1`; runtime exit1/5.910s. Actual failure: `predecode: called=true info={Format: Width:0 Height:0 Frames:0 Animated:false EstimatedDecodeBytes:0} err=media invalid want=media limit` (`244756`). The seam did not call a real decoder. Three4000x4000 interlaced descriptors satisfy the old visible/retained48M estimate but need an additional48M original/replacement allowance. The exact corrected estimate argument was immediately restored. The subsequent complete expanded focused suite passed exit0/5.929s (`286db1`); final frozen GIF identity is recorded below.

RED commands emitted their raw logs to the actual tool transcript rather than filesystem log files. The exact observed failures/commands/exits are retained above; no reconstructed JSON is presented as an original RED log. The final full scoped JSON is persisted separately.

## Final assigned verification

All commands used the verified native executable with process-local offline settings:

```powershell
$env:GOTOOLCHAIN='local'
$env:GOPROXY='off'
$env:GOSUMDB='off'
$env:GOPATH="$PWD/.tools/go"
$env:GOMODCACHE="$PWD/.tools/gomod"
$env:GOCACHE="$PWD/.tools/gocache"
$env:GOTMPDIR="$PWD/.tools/tmp"
$env:TEMP="$PWD/.tools/tmp"
$env:TMP="$PWD/.tools/tmp"
& .tools/toolchains/go1.27.2-verified/go/bin/go.exe test -json ./internal/media ./internal/privatefs -count=1 > .tools/tmp/image-validation-final-tests.json
& .tools/toolchains/go1.27.2-verified/go/bin/go.exe vet ./internal/media ./internal/privatefs
```

The cwd is `C:/Users/auzasr/Documents/Projects/miskoai`, so all cache/temp environment paths resolve to absolute project-private paths. Scoped tests and vet ran independently; no escalation, permission repair, check relaxation, actual data or external requests were necessary.

Final tests: **exit0**, media elapsed12.599s; privatefs elapsed2.204s. **187 named pass,0 fail,4 skip** (counts include named parents/subtests/fuzz seeds; package completion events excluded). Media172pass/privatefs15pass. Image validation contributed **79 named passes**:22 TestImage top-level functions, their named subtests, and FuzzImageValidation plus7seed cases. Scoped vet: **exit0**, no diagnostics. JSON evidence: `.tools/tmp/image-validation-final-tests.json`, SHA256 `9de3dbf55cbfa0912cca459bb1dd0337bad20e05f3114ece943590f6701b7d39`.

Existing privatefs skips, stated precisely rather than treated as passes:

- TestPrivateJunctionTraversalWindows: host denies synthetic junction setup, Access is denied.
- TestPrivateDirNeverRepairsExisting: explicit broad DACL is tested in the Windows-specific test.
- TestPrivateSymlinkTraversal: host lacks symlink privilege.
- TestPrivateHardlinkAndEmptyProtection: host denies synthetic hardlink creation, Access is denied.

All these image top-level cases ran and passed in the final JSON:

```text
TestImageGIFInterlacedCumulativeEstimate
TestImageTinyInterlacedGIFCompleteDecode
TestImageProgressiveCompleteDecode
TestImagePNG16InterlacedCompleteDecode
TestImageGIFLaterFramesAndMalformedStructures
TestImagePNGStructureAndCompressedErrors
TestImageJPEGStructureAndEntropyErrors
TestImageReaderErrorsAreSafe
TestImageArithmeticAndExactBoundaries
TestImageMetadataMustMatchDecoder
TestImageExactStructuralEstimates
TestImageEntropyFramingAndFullDecode
TestImageBlockedReadCancellationJoins
TestImageLateCancellationAndDeadline
TestImageFiniteAdversarialMutations
TestImageCallerFileOwnership
TestImageProgressiveSamplingEstimate
TestImagePNG16BitInterlaceEstimate
TestImageGIFAllFramesBounded
TestImageCheckedArithmeticAndLimits
TestImageCancellationAndOwnership
TestImagePNGAndJPEGCompleteValidity
FuzzImageValidation
```

Coverage includes complete real tiny compressed PNG/JPEG/GIF, progressiveJPEG, Adam7RGBA16PNG and row-verified interlacedGIF; every-prefix truncation of ordinary formats; wrongCRC/correctCRC-invalid-zlib/entropy failures; APNG refusal;4096-marker/chunk caps; malformed lengths and duplicates; byte-stuffed/restart/fill framing;32vs33GIF frames; small-first/huge-later and cumulative/interlaced allocation refusal; exact4MiB input; int64 overflow and64MiB sum boundary; rounded JPEG component/padding formulas and exact GIF estimate64MiB without large decode; blocked actual decoder/read lifetimes and late cancellation; unchanged/open caller file/seek position; fixed IO errors hiding a synthetic secret canary; and finite per-byte mutations. Large fake-header fixtures never invoke the actual decoder; valid small compressed fixtures prove full decode paths independently.

## Frozen source identities

The five Go source files were unchanged during final scoped tests/vet and report preparation. SHA256:

| Owned path | SHA256 |
|---|---|
| internal/media/image.go | 49826a6d253ce7014e9087f1da847f46bf3a28a762db7417efeeb584eb7b1083 |
| internal/media/image_png.go | d8376dcb6e236295487477cce77cc6491f912103280453c91d4f95296531bdb1 |
| internal/media/image_jpeg.go | 282fd3b534f00460bd9fd5727a5104580b05638a02dec122e473ebb7020bc1a8 |
| internal/media/image_gif.go | 8a1c1f9d923834c6e99347a5093025ee76e25933fbbb31c16dae5c52d1383eae |
| internal/media/image_test.go | f5795be994e3895b137ad9d451f9898565df26acdefb7025a5975ac9824dd398 |

This report's final hash is returned separately to avoid a self-referential hash. Implementation and assigned scoped local gates are ready for fresh review; primary integration/publication and acceptance remain primary-owned. No unresolved observed local image test failures remain.
