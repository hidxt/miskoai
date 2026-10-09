# Guarded PDF Extraction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans. Two serial independent review gates, followed by root source/license/security/native/resource gates.

**Goal:** Extract supported text PDFs through a guarded pure-Go parser with honest unsupported/scanned results and bounded work.

**Architecture:** A restricted attributed internal fork of the researched pin adds one shared budget throughout parser growth, resolution, decompression and text decoding. Only a bounded extraction function is exposed to document.Parser; raw reader/value/interpreter internals remain package-private. The existing document parser owns the single admitted worker and total cooperative deadline.

**Tech Stack:** Go1.27.2 stdlib; pinned ledongthuc/pdf6c8c28e0e8a07a3452f9d8cf3090ed6e856d8f24 source under retained BSD3 notice, no remote runtime/new module dependency.

**Spec:** docs/superpowers/specs/2026-10-09-attachments-design.md and docs/research/pdf-bounds-design.md. TXT/DOCX parser/admission gates precede application integration.

## Global Constraints
- Children only GPT-6.1 Medium/Low, no delegation/network/actual files/live provider/channel calls. Root supplies verified source/license; do not fetch another upstream version.
- Input4MiB/pages64/all-stage decoded8MiB/outputUTF8 text128KiB; one admitted parser and cooperative2s context, no abandoned timer goroutine/hard deadline/RSS guarantee.
- Shared parser ceilings:32768 object/xref/container nodes;32 xref revisions; graph/lexer/page depth64; individual token64KiB; aggregate retained token/container payload8MiB; filters4; predictor row4KiB; active streams256; operands256; dictionary scopes64; CMap entries8192; each code<=4bytes, each mapped UTF16 replacement<=1024bytes; global work32Mi operations, decremented on attempted reads (including EOF), resolution, interpreter actions and CMap comparisons/expansion.
- All guards before allocation/append/metadata multiplication. No budget resets across page, indirect resolution, object-stream chain or filter stage. Safe constant errors, no raw bytes/value formatting/debug output.
- No PDF JavaScript/actions/URL/embedded-file execution, password guessing, external fonts or network. Encryption, unsupported filters/fonts/glyph differences and scanned/image-only PDF are honest unsupported results; never fabricate text.
- Omit the unresolved external glyph name table name.go; standard supported encodings/UniGB and explicit ToUnicode remain available with strict Unicode checks. Unsupported Differences names refuse unless separately verified by root; do not publish that table's source or infer provenance from package license.

## Review Focus
- Tiny /Size,/W,/Columns metadata and sparse offsets cannot allocate unchecked memory or overflow.
- /Prev,/Extends,page-parent/indirect cycles fail within one shared visited/depth/work budget.
- Nested decompression or zero-progress/EOF lexer/interpreter loops cannot reset budget, leak decoders or suppress cancellation.
- Reused font names across pages, malformed UTF16/CMaps and valid double-quote text operator preserve exact supported text or return explicit failure.
- Exhausted/canceled parsing keeps actual worker admission until it returns; no successful partial extraction or private output/error leak.

## Task1: Guarded parser core and source boundary — Medium
**Files:** create internal/pdfsafe/{budget.go,read.go,lex.go,ps.go,ascii85.go,core_test.go,lex_test.go,filter_test.go,LICENSE,UPSTREAM.md}. Root supplies exact ignored .tools/pdf-research/src files/license (SHA inventory), no network. Report docs/superpowers/reports/2026-10-09-pdf-core.md. Root owns embedded binary notice integration before any publication.

**Interfaces:**
- Package-private `parse(ctx context.Context,input io.ReaderAt,size int64)(*reader,error)` and shared `budget`; all value/reader/stack/interpreter types and entry points package-private. No raw exported API. Syntax guard failures return or propagate a typed safe private budget panic caught once by parse/extraction; interpreter recovery must rethrow budget/cancel instead of silently continuing. Never recursively format attacker-owned values.
- Before first allocation validate input size, xref node/revision/offset/width/dictionary/array counts, checked subtraction/multiplication and aggregate retained bytes. Visited offsets/object IDs cover Prev/Extends/indirect resolution; no uint/int conversion before proving bounds. Every lookup resolves through same budget and recursion depth.
- Context-aware ReaderAt/byte-reader decrements attempted-read work even at EOF. Lex token/container guard before append; no synthetic EOF whitespace loop. ASCII85 reader rejects zero-progress invalid streams. All-stage decoder accounting includes intermediates and predictor rows; retain/close actual filter chain on every success/error/cancel. Reject unsupported/encrypted filters safely.
- Fork retains upstream headers/license and pin/changes in UPSTREAM.md; remove unused file-path Open/debug/formatting helpers rather than retaining unrestricted entry points. Production file input arrives only from admitted immutable private spool; test files are synthetic generated bytes.

- [ ] Write runtime RED tests against the exact unguarded pinned source baseline: TestPDFSizeWidthAndPredictorBeforeAllocation, TestPDFPrevExtendsIndirectCycles, TestPDFEOFHexAndASCII85Progress, TestPDFFilterAggregateAndClose, TestPDFBudgetCancelNotRecovered. Unsafe huge allocations are tested via narrowly bounded declared values or allocation seams, never actually request unbounded RAM. Missing guards must fail named assertions, not compile setup.
- [ ] Implement shared guard source changes in owned files; preserve valid minimal xref table/stream/object stream/Flate samples with exact ranges. Per-doc active streams bounded and closed on errors; no detached work.
- [ ] Run pdfsafe tests/vet and bounded-duration fuzz seeds using verifiedGo1.27.2/offline caches. Record RED/GREEN, before-allocation evidence/work/close/cancel limits; freeze for fresh Medium resource/security review/root gate. No application PDF support claim yet.

## Task2: Bounded page/font text and document integration — Medium
**Files:** create internal/pdfsafe/{extract.go,page.go,text.go,extract_test.go,font_test.go,cmap_test.go}; modify guarded core only for narrowly required budget propagation, exact ownership agreed before edits. Modify internal/document/{document.go,document_test.go} only for PDF dispatch/fixtures after prior parser gate. Report docs/superpowers/reports/2026-10-09-pdf-extraction.md.

**Interfaces:**
- Sole exported parser API `Extract(ctx context.Context,input io.ReaderAt,size int64)(text string,pages int,err error)` checks same core limits and discards all output on error. document.Parser calls it synchronously under its one existing admission/deadline; no second worker or budget reset. Translate safe pdfsafe errors to document sentinel codes without raw error formatting.
- Walk actual page tree<=64 with visited IDs/depth64; declared Count is validated but does not permit unbounded traversal/allocation. Only supported text operators; no graphics/content/outline output buffer. Output builder checks128KiB before every append, including separators and Unicode decoding. Empty/image-only is explicit unsupported OCR, not successful empty/fabricated text.
- Font identity keyed by actual indirect object identity plus page scope where direct, not resource-name alone. Supported standard byte encodings/UniGB-UCS2-H/explicit ToUnicode CMaps use strict UTF16 pairing/codepoint validation and bounded replacements. Missing/unrecognized glyph encodings refuse honest unsupported; no arbitrary replacement characters presented as correct extraction. Do not adopt name.go external table.
- CMap counts<=8192, codes1..4bytes, replacement<=1024bytes and aggregate8MiB retained-data budget checked before append/expansion; global work metered comparisons and range operations. Reject overflow/mismatched ranges and unbounded derived array generation. No broad unrestricted Value/Interpret export.
- Correct double-quote operator's three operands without falling into one-operand validation; preserve source-order supported text semantics. Keep full parser error/recovery constant, no debug/raw value print.

- [ ] RED TestPDFValidLatinChineseAndFontIdentity, TestPDFDoubleQuoteOperator, TestPDFMalformedUnicodeAndCMapBounds, TestPDFPagesOutputAndNoPartial, TestPDFDocumentAdmissionCancellation. Generated supported Latin/Chinese PDFs, reused F1 on different font objects, ToUnicode multi-code/ranges, legitimate double quote, pages64/65, output128KiB/+1, malformed UTF16 and actual cancel/work exhaustion. Scanned/encrypted/unsupported font fixtures must produce explicit unsupported code, not claimed extracted text.
- [ ] Implement bounded extraction and synchronous document hook; retain root-reviewed core guard guarantees. Import cycle check: document imports pdfsafe, pdfsafe never imports document/private data/provider.
- [ ] Run document/pdfsafe tests/vet and seeded bounded fuzz, freeze for fresh Medium source/security/Unicode/resource review/root integration. Root adds BSD source/binary inventory, full scanners/native race/Linux builds and actual512MB workload before acceptance. No live PDF/vision/WeChat tests under spent cloud allowance.

## Root gate and self-review
Research was a candidate only; adopting code requires both independent gates, root full-source manual review, complete notice inventory and compatible native/resource results. Do not publish source/binary until notices integrated. TXT/DOCX/transport/excerpt/sticker have separate plans. Every Review Focus is covered by named tests. Core and extraction share the same checked budget, not an unsafe wrapper around raw upstream APIs. Explicit subset errors preserve requested safety/resource route; any inability to meet functional/resource goals is reported for root decision rather than bypassed. Owner's standing autonomy supplies implementation method; no routine extra approval pause.
