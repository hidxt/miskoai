# Isolated shared document lifetime preparation — 2026-10-12

Engineering progress remains **70%** (35/50). Author: ONLY `gpt-6.1-sol`, reasoning effort `medium`; no delegation. This is isolated Task1 preparation under the 2026-10-12 plan exception, with no main product activation or completion credit.

Implemented `NewProcessor(dataDir string, cdn *media.CDN, parser *document.Parser) (*ImageProcessor,error)` and `(*ImageProcessor).WithDocument(context.Context, storage.Attachment, func(document.Result) error) error`. Construction preserves `NewImageProcessor`, borrows the supplied parser/CDN, and reuses the existing persistent image slot. The sole image.go source changes are the document import and parser field; the complete WithImage body is byte-identical to the accepted source. No provider/storage/parser/channel/service/route/cap changes.

Descriptor/kind/basename extension/key validation precede fetch. TXT/MD/DOCX/PDF extension hints are case-insensitive; slash/backslash, missing basename and unknown extensions refuse safely. Optional canonical Length must equal completed plaintext size. Every artifact is scanned synchronously in fixed4096-byte chunks before parsing; optional MD5 is compared against the complete scan. A full final read with EOF is accepted, while short/dishonest/error/intermediate-EOF reads refuse. MD5 is compatibility metadata only and is not authenticity. Expected raw gosec MD5 warnings remain unsuppressed for primary static triage; no author static scan was run.

The borrowed real Parser.Extract remains synchronous under its unchanged cooperative2s parser budget. Complete nonempty UTF8 text, exact selected supported format, <=128KiB output and pages1..64 are checked before the consumer. The slot stays occupied through actual download/read/parse/consumer/Close return. Every returned artifact, including error-with-artifact, closes once before release; safe fixed errors preserve cancellation and joined cleanup classification. No goroutine, pool, arbitrary path/URL fetch, raw error/body/name/key export or whole-file production copy was added. Caller must supply prior authorization/durable claim, immutable completed private input, cooperative actual operations and a callback finishing model work without retaining its borrowed result.

Owned canonical files, all only under `.tools/reviews/document-lifetime-work-src`:

- `internal/attachment/document.go` (new)
- `internal/attachment/document_test.go` (new)
- `internal/attachment/image.go` (two source additions)
- `docs/superpowers/reports/2026-10-12-document-lifetime.md` (this report)

Tests use generated tiny TXT/Markdown/restricted DOCX through the real shared parser and real private completed media artifacts; no network or private user data. They cover metadata/preflight no effects, checksum before parse, output completeness/bounds, safe errors, real cleanup, borrowed parser/shared admission, both next image/document blocked through held consumer and Close, and actual cancellation/join during download, preparse scan, parser read, consumer and Close. Cleanup/release/join is registered before held-work assertions. Supported PDF functional testing is explicitly deferred until the main guarded extraction gate; no test freezes PDF as permanently unsupported and no fake PDF success is claimed.

Toolchain is the verified native `.tools/toolchains/go1.27.2-verified/go/bin/go.exe`, observed `go version go1.27.2 windows/amd64`. Commands ran with cwd `C:/Users/auzasr/Documents/Projects/miskoai/.tools/reviews/document-lifetime-work-src`, GOTOOLCHAIN=local/GOPROXY=off/GOSUMDB=off, root `.tools/go`, `.tools/gomod`, `.tools/gocache`, `.tools/tmp`; MISKOAI_* removed by name without printing values. Synthetic keys only.

Actual tests and evidence (every selector is against `./internal/attachment`, `-json -count=1 -timeout=15s`):

| Prefix after document-lifetime-parallel- | Exact selector | Named pass/fail/skip | Exit |
|---|---|---|---|
| red | `TestDocument(Supported\|Preflight\|Failure)` | 17/21/0 | 1 |
| red-lifetime | `TestDocumentAdmission` | 0/1/0 | 1 |
| green | no selector, whole attachment package | 78/0/0 before watchdog, incomplete | 1 |
| green-document | `^TestDocument` | 56/0/0 | 0 |
| green-image-a | `^TestImage(Formats\|Preflight\|Downstream\|Canonical)` | 34/0/0 | 0 |
| green-image-b | `^TestImage(Admission\|Canceled\|Held\|Known\|Constructor)` | 18/0/0 | 0 |
| green-document-final | `^TestDocument` | 58/0/0 | 0 |

RED used a compiling minimal unimplemented constructor/method stub, rather than compile failure. The finite semantic failures showed supported inputs lacked callbacks/cleanup. The lifetime RED ended at its finite barrier (package5.675s), not a Go watchdog. The original RED source was not frozen contemporaneously; original actual RED events/exits are retained without reconstructing evidence. Additional guard/constructor/parser-read assertions were added after initial GREEN and are covered by the final document run.

The unselected attachment package command hit its required15s watchdog during existing image tests (`TestImageKnownErrorsSanitizedAndCleanupFailure/media_limit`); package elapsed16.944s, actual exit1. This is NOT a full attachment suite pass. Primary authorized disjoint finite selector shards instead of extending deadlines. Final58 document + image34 + image18 gives110 distinct named test events across those three selectors, each exit0/zero failures/skips; it is scoped shard evidence, not a whole-suite claim. Earlier successful56-document result is retained separately and not double-counted. The tests/production from the image shard runs are unchanged in the final snapshot except newline convention restoration of image.go; document-only additions do not change image test source or behavior.

`go vet ./internal/attachment` ran twice (vet and vet-final prefixes), each actual exit0 and empty stdout/stderr. No full/static/race/native checks were run by this author. A preliminary root-cwd `go fmt ./.tools/reviews/document-lifetime-work-src/internal/attachment` failed exit1 because the main module does not contain the hidden nested module; its original diagnostics are preserved. Formatting was then performed with the verified gofmt binary on the three owned Go files only. No original log was overwritten.

Scope verification proves main attachment/image.go matches the accepted313-file baseline, all three new main owned paths remain absent, every unowned isolated accepted file is exact except the intentionally refreshed constraint docs, and the entire WithImage body is exact. Main PDF WIP belongs to the other writer and is neither read as accepted evidence nor modified here. LICENSE and accepted content remain intact. Frozen source/report and evidence copies/manifests carry canonical repo-relative paths, actual absolute isolation paths, byte lengths and SHA256 hashes.

All Go command sessions exited and all finite held fixture workers joined. No process or worker from this task remains active. Main product/root docs/Git were not edited. Primary integration, fresh review, supported PDF fixture, full/root/static/secret/native gates remain required. Accepted baseline1497 local named passes and12 native CI steps are separate prior evidence; this preparation supplies no new native CI, live WeChat/file delivery, private schema migration,512MB/resource/arm64 execution or final acceptance.
