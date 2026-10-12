# Bounded PDF extraction Task2

Executed 2026-10-12 by the sole checkout writer, GPT-6.1 Sol Medium (`gpt-6.1-sol`, reasoning effort `medium`). Engineering progress remains **70%**; package38 receives no author completion credit. The accepted Task1 baseline is `3be86ef7981355c2e5fac391b055c58ed6365aee`. Its root-verified native CI38140923924/job114474456598 is separate from this Task2 work. No delegation, network, real document, private database, provider, CDN, WeChat, server or additional runtime was used.

## Interface and implementation

`pdfsafe.Extract(context.Context, io.ReaderAt, int64) (string, int, error)` is the sole exported function; `ErrInvalid`, `ErrLimit`, `ErrUnsupported` are fixed aliases. Nil/typed-nil input/context are refused; all failures discard text and pages. The existing synchronous `document.Parser.Extract` admits PDF through its original one-slot, call-entry cooperative two-second deadline and safe error translation. There is no second admission, goroutine or budget reset. An actual blocked reader retains the slot until it returns. Incoming earlier cancellation/deadline remains identifiable.

Actual indirect page identities, parents, traversal depth and declared nonnegative typed Counts are checked. Counts are compared to actual descendant pages after bounded traversal; they never size a page allocation. Actual pages are capped at64. Repeated identities/cycles refuse, and inherited resources use bounded identity traversal under the original cumulative budget. Content arrays preserve declared stream order. Text uses supported operators and deterministic explicit line/page separators, with the same128KiB output cap; no layout or graphics buffer is created. Empty/image-only extraction returns unsupported.

Font caches use indirect ID/generation or page-scoped direct identity. Supported byte paths are StandardEncoding and the verified pinned WinAnsi/MacRoman tables; composite paths are UniGB-UCS2-H or explicit ToUnicode mappings. Type0 fonts require one valid CID descendant and typed CID metadata. Differences, other font subtypes/encodings, external CMaps, forms and unrecognized operators refuse rather than silently dropping visible text. No `name.go`, glyph-name mapping dependency, OCR, actions or external resource access exists.

CMap code spaces are1..4bytes, disjoint and unambiguous across lengths. Mapping counts/range expansion are capped at8192; replacements are at most1024UTF16 bytes. Unknown/truncated source codes, duplicates, reversed/mismatched ranges, malformed UTF16/surrogates and replacement overflow fail. Unicode, comparisons, ranges, temporary/replacement allocations and output share the original work/retained/decode budget. Capacity growth reserves new storage and copied prefixes before allocation. Output UTF8 is checked before final success and cancellation is checked again afterwards.

Task1 M1 is addressed: raw CR and CRLF normalize to LF, unknown escapes ignore the backslash, escaped line breaks disappear, and octal keeps the low eight bits. Unterminated strings remain malformed. M2 is addressed: lexer-created streams carry a private origin marker; every actually resolved stream validates declared body bounds, preceding EOL, `endstream` and `endobj` before consumption. Image streams receive the same syntax validation even though text extraction does not decode images. Direct decoder test seams are explicitly distinct from parsed objects. Complete actual decoder EOF and the previously accepted object-stream member spans remain intact. Content dispatch bypasses the PostScript dictionary interpreter machinery, so unsupported content operations and trailing operands cannot disappear silently.

## Verification and preserved failures

Native Windows verified Go1.27.2, offline `GOTOOLCHAIN=local`, `GOPROXY=off`, `GOSUMDB=off`, `.tools/go`, `.tools/gomod`, `.tools/gocache`, `.tools/tmp`. All Go test invocations use `-count=1 -timeout=15s -json`; no unrestricted cyclic/OOM baseline was executed. Final commands:

```
.tools/toolchains/go1.27.2-verified/go/bin/go.exe test -count=1 -timeout=15s -json ./internal/pdfsafe ./internal/document
.tools/toolchains/go1.27.2-verified/go/bin/go.exe vet ./internal/pdfsafe ./internal/document
.tools/toolchains/go1.27.2-verified/go/bin/go.exe test -count=1 -timeout=15s -json ./internal/pdfsafe -run '^FuzzPDF'
```

Final scoped tests: **340 named passes,0 failures,0 skips**,2 package passes, exit0. Scoped vet: exit0, empty diagnostics. Fixed fuzz seeds: **18 named passes,0 failures,0 skips**, exit0; this is seed execution, not a randomized fuzz campaign. Logs use only new `pdf-extraction-task2-*` paths.

| Evidence prefix | Named pass | Named fail | Exit | Interpretation |
| --- | ---: | ---: | ---: | --- |
| feature-red | 0 | 4 | 1 | Before feature code: actual old Parser refuses supported Latin, Chinese and double quote. |
| core-red | 0 | 3 | 1 | Finite raw-newline mismatch and valid escaped string typed panic; original preserved. |
| core-red2 | 2 | 6 | 1 | New isolated HEAD core, finite panic caught by test; literal and actual stream closing assertions fail. |
| feature-red2 | 0 | 43 | 1 | Later expanded tests against a new exact HEAD archive; test-only bridge calls the actual old document.Parser. No simulated extractor. |
| cmap-dispatch-red | 0 | 6 | 1 | Same old Parser baseline rejects mapped text instead of providing strict CMap result/limit codes. |
| font-structure-red | 0 | 2 | 1 | Candidate incorrectly accepted a Type0 font without descendants; corrected after named RED. |
| green1 | 211 | 2 | 1 | Direct decoder fixture IDs were mistaken for parsed stream origins; corrected with lexer-only origin marker. |
| green2 | 263 | 0 | 0 | Initial supported feature/core regression. |
| green3 | 332 | 2 | 1 | Test incorrectly treated an odd hex nibble as an odd UTF16 byte count; PDF legitimately pads the nibble. Fixture corrected from `<004>` to one-byte `<00>`. |
| green4 | 334 | 0 | 0 | Expanded bounds/Unicode/admission regressions. |
| green5 | 335 | 0 | 0 | Complete CID fixture structure and resolved stream syntax checks. |
| final-tests | 340 | 0 | 0 | Final current scoped source including image-stream syntax, EOL and indirect Length checks. |

Expanded baseline REDs ran after initial implementation against separately exported unchanged HEAD sources. They demonstrate missing pre-feature dispatch and are not a claim that every detailed guard was individually observed failing before implementation. Original baseline/core/failed-GREEN logs were never overwritten. New archive/source directories retain the exact baseline and test-only bridge for review; root immutable Task1 freezes are untouched.

Named coverage includes exact128KiB/+1 output,64/65 actual pages, indirect/direct cross-page F1 collision, resource inheritance, content array order, double quote/TJ, Unicode surrogate success/failures,8192 mappings/8193 expansion,1024/1026 replacement bytes, four-byte and variable-length codes, ambiguous code spaces, duplicate/truncated/unknown mappings, scanned/encrypted/unsupported inputs, page/parent/indirect cycles, before-allocation retention/work refusal, valid literal semantics, malformed parsed stream closure and no partial prefix from malformed image stream syntax. The actual held PDF reader cancellation test verifies TXT/Markdown/DOCX/PDF all share occupied admission, then releases and joins before slot reuse. Existing TXT/Markdown/DOCX and Task1 tests are retained.

## Ownership, attribution and handoff

Changed/new owned files only: `internal/pdfsafe/{extract.go,page.go,text.go,extract_test.go,font_test.go,cmap_test.go,lex.go,read.go,ps.go,UPSTREAM.md}`, `internal/document/{document.go,document_test.go}`, this report. Budget constants, ASCII85 source, original core tests and LICENSE are unchanged. No root documentation or other agent's files were edited. The core modification is limited to literal semantics, actual parsed stream extent checks and a strict content-dispatch seam.

Verified source cache identities: page.go25471bytes SHA256`cf6a63204e6b151673a7dd879a598ca9c450be064db808ac88a7907ecfdbb86a`; text.go7789bytes SHA256`dd62904ec5a43e4ab1027c33f33ad44fba6b011390cfd40bf5129d9d5e886781`. Copyright headers and the exact BSD1479-byte LICENSE remain; UPSTREAM records the retained table/semantic scope. Root owns embedded source inventory/publication/final license checks.

Freeze manifest `.tools/reviews/pdf-extraction-task2-manifest.json` records owned path/SHA256/bytes and exact copies under `pdf-extraction-task2-frozen-src`, plus original evidence and copies under `pdf-extraction-task2-frozen-evidence`. Baseline source manifests separately record the new RED directories. Freeze is for fresh independent review/root integration, not acceptance.

Limitations: cooperative deadlines require actual ReaderAt progress; conservative shared retention/work may reject PDFs below an individual cap. Supported text is source order, not reconstructed visual reading order. Unknown operations/forms, custom glyph Differences, unsupported fonts/filters, external CMaps, vertical composite encoding and OCR refuse explicitly. No full suite/static/scanner/race/native Linux build/CI, randomized fuzz campaign, live file delivery, actual arm64 execution,512MB measurement or final release/security/license/UI acceptance was performed by this child. Application claimed-file/model integration is a later dependent task.

## FIX1 — cumulative CMap lifetime gate, 2026-10-12

Primary Important I1 invalidated the initial Task2 acceptance candidate: per-font8192 checks reset the mapping allowance at each font. The primary's finite two-font/two-page proof observed8192 success but8193 also returned successful text/pages. Initial scoped340 passes above did not cover this aggregate property. Its original13 owned freezes,32 evidence freezes and327 baseline identities remain unchanged, as does original manifest SHA256`8cc89fcd4e2bab0b1dfb215ea8d5c376be0120767acae784caa3d2fe36e2fe75`. No package38 credit or final approval follows from either original or corrected author tests.

Resumed by the same sole writer, ONLY GPT-6.1 Sol Medium; engineering **70%**. Narrow changes are `budget.go`, `text.go`, `cmap_test.go`, `UPSTREAM.md` and this report. The budget now owns separate cumulative mapping/code-space counts, each using the unchanged8192 ceiling. Code-space declarations do not consume the exact8192 mapping allowance. bfchar declared counts and bfrange expansion lengths check the remaining lifetime capacity before replacement/derived allocation; actual mapping and code-space insertions increment once. Existing indirect font-cache hits reuse their parsed font, while distinct fonts/actual reparses consume the same cumulative budget. All original numeric, Unicode, stream, page, work/decode/retention/output and admission guards remain.

Finite TDD used the primary proof's semantics and synthetic same-page/two-font variant before code changes. `TestPDFCMapLifetimeFix1` also covers multiple blocks of one font, exact8192, cached same indirect font reused under a renamed page resource, and range preflight before a missing replacement. A separate code-space test verifies the exhausted code-space budget refuses before slice allocation/append without consuming mapping capacity.

| New FIX1 evidence | Named pass | Named fail | Exit |
| --- | ---: | ---: | ---: |
| fix1-red | 5 | 3 | 1 |
| fix1-red2 | 5 | 4 | 1 |
| fix1-green | 349 | 0 | 0 |
| fix1-final-tests | 350 | 0 | 0 |
| fix1-seeds | 18 | 0 | 0 |

RED failures are both cross-font8193 successes instead of `ErrLimit`, plus the parent; RED2 additionally observes `ErrInvalid` rather than global `ErrLimit` when a second CMap's range tries to read a missing replacement after8192 prior mappings. No panic/watchdog/OOM occurred. Final scoped document/pdfsafe command remains `test -count=1 -timeout=15s -json ./internal/pdfsafe ./internal/document`:350 named passes,0 failures,0 skips,2 package passes,exit0. Final scoped `vet ./internal/pdfsafe ./internal/document` exits0 with empty diagnostics. Fixed `test -count=1 -timeout=15s -json ./internal/pdfsafe -run '^FuzzPDF'` seeds18 pass/0 fail/exit0. Offline native Windows verified Go1.27.2 only; original logs are not overwritten.

New `.tools/reviews/pdf-extraction-task2-fix1-manifest.json` freezes all14 current source/report identities (budget.go added), plus only new `pdf-extraction-task2-fix1-*` evidence under separate FIX1 freeze directories. The original Task2 manifest/frozen/evidence are preserved. Fresh independent review and primary full integration/scans/publication/native gates remain pending; no live/private/WeChat/512MB/final-product work or acceptance is claimed. Writer is quiescent after corrected freeze.
