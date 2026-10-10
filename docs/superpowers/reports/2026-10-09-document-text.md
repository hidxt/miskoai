# Document Task 1: bounded TXT and Markdown

Engineering progress remains **64% (32 of 50 packages)**. This is an implementation and scoped Windows local-test handoff, not package36 acceptance. Fresh independent review and root integration/scanners/native CI are pending. Live document/WeChat tests, actual server RSS and final acceptance are unverified.

Author harness: **gpt-6.1-sol, reasoning_effort medium**. No child delegation, Git operation, private input, database, provider call, network operation, service startup or dependency change was used.

## Changed files and interface

Only these product/report files were created:

- `internal/document/document.go`: `Format` with TXT/Markdown/DOCX/PDF constants; `Result{Text, Pages, Format}`; `New() *Parser`; `(*Parser).Extract(context.Context, Format, io.ReaderAt, int64) (Result, error)`; fixed exported error categories and shared private admission/read/work/output helpers.
- `internal/document/text.go`: fixed-chunk strict TXT/Markdown extraction.
- `internal/document/document_test.go`: validation, cancellation, serialization and cooperative deadline tests with joined synthetic fixtures.
- `internal/document/text_test.go`: UTF8/BOM, output/input bounds, completeness and adversarial ReaderAt cases.
- This report.

Caller contract: reuse one initialized Parser per future Core. Input is an immutable completed private local file with truthful exact size; filenames/URLs never become paths. ReaderAt must be nonblocking or independently honor cancellation. This package cannot establish immutable spooling or truthful backing-file size on behalf of its caller. Extracted text remains external evidence; caller owns provenance and model-visible range labels.

TXT/Markdown preserve whitespace, line endings, Markdown and HTML-looking text verbatim. One initial UTF8 BOM is stripped; subsequent BOMs are preserved. Invalid/truncated UTF8, UTF16/32 BOMs and NUL fail with ErrInvalid. Empty and Unicode-whitespace-only supported input fail with ErrEmpty. Nonempty success returns one page and the supplied supported format. DOCX/PDF and unknown formats return ErrUnsupported before payload reading. Nil context/input (including typed nil), uninitialized/nil Parser, negative size and input over4MiB refuse before reading.

Input cap is4MiB; complete extracted UTF8 output cap is128KiB. Exactly128KiB succeeds, including input with one extra stripped BOM;128KiB+1 fails, with zero Result. Parsing uses a4099-byte fixed buffer containing a4096-byte read chunk plus at most3 pending UTF8 bytes. Output grows only within the128KiB content bound; successful conversion to string makes a bounded output copy. Go slice capacity growth has bounded slack and this is not an RSS measurement. No entire input buffer, archive/XML/PDF processing or detached parser/I/O goroutine is introduced.

One admission slot belongs to each Parser. A two-second cooperative deadline starts at Extract call entry and covers both waiting and admitted work. Context is checked before and after synchronous reads and bounded output work. An arbitrary blocked ReaderAt cannot be interrupted by the parser: the call may exceed two seconds and retains admission until actual ReadAt returns. Cancellation discards all output, uses fixed safe text, and matches both ErrCanceled and context.Canceled/context.DeadlineExceeded through errors.Is. Reader errors/counts are classified without returning reader text, paths or payload canaries.

## Actual scoped verification

Environment setup was copied only from `.tools/reviews/attachment-integrated-root-checks.ps1`; that full-root script was not executed. Each Go command used `.tools/toolchains/go1.27.2-verified/go/bin/go.exe`, with workspace-local GOPATH/GOMODCACHE/GOCACHE/GOTMPDIR, GOTOOLCHAIN=local, GOPROXY=off and GOSUMDB=off. MISKOAI_* environment entries were removed by name without reading/outputting their values; explicit synthetic data/admin/provider settings were supplied. `go version` returned `go version go1.27.2 windows/amd64`.

Actual commands:

```powershell
& .tools/toolchains/go1.27.2-verified/go/bin/go.exe test -json ./internal/document -count=1
& .tools/toolchains/go1.27.2-verified/go/bin/go.exe vet ./internal/document
```

The initial compiling unavailable stub returned ErrUnsupported from Extract. RED test command exited1: **3 named pass,26 named fail,0 skip**, package failed. Failures were semantic unavailable-functionality assertions (including expected extraction/validation/cancellation), not syntax/import failures. Stub API and all initial tests compiled. Preserved `.tools/reviews/document-text-red.jsonl` and `document-text-red-exit.txt` contain the actual output/exit.

Minimal implementation initial GREEN exited0: **29 named pass,0 fail,0 skip**, package passed; scoped vet exited0 with an empty log. Preserved `document-text-green.jsonl`, `document-text-green-exit.txt`, `document-text-vet.txt` and `document-text-vet-exit.txt`.

An additional guard test then covered the already-implemented total deadline through both actual admission waiting and a subsequent delayed read; the held fixture cleanup was simplified to track joined completion. Final full document-package run exited0: **30 named pass,0 fail,0 skip**, one package passed. Final scoped vet exited0,0-byte output. Preserved `document-text-final-green.jsonl`, `document-text-final-green-exit.txt`, `document-text-final-vet.txt` and `document-text-final-vet-exit.txt`. There are no tests skipped or failures omitted from these scoped runs.

Named top-level final passing tests:

- TestParserValidationBeforeRead: oversized4MiB+1/negative/nil/malicious format/DOCX/PDF/pre-canceled calls refuse without reads.
- TestParserAdmissionAndCancel: a held synthetic read stays active beyond its parent deadline; canceled and parser-deadline waiters cannot read; actual release is followed by joined cancellation and reusable admission.
- TestParserDeadlineIncludesWaitAndRead: admitted read after waiting still uses the original call-entry deadline, and all synthetic workers are joined before assertions.
- TestParserCooperativeDeadline: a synchronous late-returning read cannot produce success, and cancellation matches context errors.
- TestTextUTF8SplitAndBOM: all splits of2/3/4-byte runes across4096-byte boundaries; initial/repeated/midtext BOM; UTF16/32, invalid/overlong/surrogate/truncated UTF8 and NUL; verbatim markup/spacing.
- TestTextExactBounds: exact128KiB,128KiB+1, one stripped BOM, admitted4MiB producing output refusal, zero and Unicode whitespace input.
- TestParserNoSuccessfulTruncation: short EOF/short nil/negative/oversized counts and fixed-safe raw-reader failure; full final read with EOF remains valid.

The fixture cleanup is registered before goroutines/assertions; it cancels/releases and joins outstanding work. No elapsed measurement is claimed as a hard timeout guarantee.

## Freeze and remaining gates

The five owned files are copied with their relative paths under `.tools/reviews/document-text-owned-src`. `.tools/reviews/document-text-owned-hashes.json` records SHA256 and bytes for each original/frozen copy. Test/vet logs and actual exit files are copied under `.tools/reviews/document-text-evidence`; its manifest records SHA256/bytes. Those exports are review artifacts, not extra product files. Root must independently read source/diff, verify hashes and run integrated checks; author assertions do not satisfy that gate.

No ZIP/XML/PDF/Core/agent/channel/storage/media/provider/config wiring was changed. Later tasks may reuse the private helpers only after this serial review/root gate. No full-root suites, scans, native Linux/race/CI, live providers, real WeChat/documents, private migration, server deployment,512MB result or final acceptance claim is made here. No unresolved local test failure was observed; review and integration remain outstanding.
