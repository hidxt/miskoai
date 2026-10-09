# PDF bounded extraction research

2026-10-09. Read-only Low source research and Medium bounded-design review; no parser adopted or runtime tests run. Root records the received evidence for recovery. No PDF dependency/license change is implemented yet.

Candidate: ledongthuc/pdf at commit6c8c28e0e8a07a3452f9d8cf3090ed6e856d8f24, BSD-3-Clause, Go1.24.1, stdlib-only. Source review identified CJK UniGB support, but this does not establish complete Chinese-PDF compatibility or512MB safety.

## Findings in primary source

Pinned files read.go, lex.go, ps.go, page.go and ascii85.go require cross-cutting budget checks. ReaderAt input size alone does not bound metadata-driven allocation: xref /Size and /W, predictor columns, object stream resolution and CMap work can expand. /Prev or /Extends chains and indirect/page-parent cycles require visited offsets and active object IDs. Whole GetPlainText buffers all pages; page extraction also buffers output. Existing lexer nesting cap1000 is real but insufficient for indirect graphs.

Lexer EOF in readHexString can repeatedly accept synthetic whitespace; metering must include readByte attempts even at EOF. ASCII85 filtering can produce zero bytes without EOF for invalid input, so progress/work must be bounded. ps.readObjectRecover catches non-runtime panics and continues; typed budget/cancel errors must propagate rather than silently recover. Every filter stage needs a cumulative decode budget; the last stage alone misses nested decompression expansion. Per-page font caching must key by actual font object identity to avoid shared resource-name collisions.

## Proposed guarded internal fork

An attributed internal fork with guards throughout parser allocation/growth is a candidate, not a tiny safe wrapper or an accepted dependency. Limit input4MiB, actual pages64, cumulative decoded bytes8MiB, extracted UTF-8 text128KiB, one admitted worker and cooperative context2s. These are proposed engineering bounds, not a hard elapsed deadline or RSS guarantee. Keep the worker occupied until parser returns; do not abandon an uninterruptible parsing goroutine after timer expiry.

Before allocation/append/expansion use checked arithmetic and one shared document budget:32768 xref/object nodes,32 xref revisions, graph/lexer depth64, token64KiB, bounded aggregate token/array/dictionary entries, filters4, predictor row4KiB, open streams256, operands256, dictionary scopes64, CMap count/expansion/comparison limits and a global work meter. Close decompressor chains. Never reset budgets across object resolution. Public extraction API must not expose unrestricted raw Value/Interpret internals. Safe errors must not recursively format attacker-owned cyclic objects.

## Required evidence before adoption

Synthetic adversarial tests: huge /Size and sparse indices, /W overflow, /Prev and /Extends cycles, page-parent cycles, giant predictor rows, nested Flate expansion, malformed EOF hex, operand/dictionary growth, CMap count/replacement expansion, repeated resolution and cancellation in every phase without a leaked worker. Valid Latin/Chinese PDFs, per-page font-name reuse and upstream fixtures need tested declared-range extraction. Native Linux fuzz/race and actual512MB workloads remain separate gates.

No alternative Go PDF library was proven to expose all required budgets during this research. A guarded internal fork remains the proposed route within the user's pure-Go requirement; full audit/benchmarks and license retention must precede functional claims.

## Root primary-source verification

Root fetched the exact pinned upstream go.mod, LICENSE, read.go, lex.go, ps.go, page.go and ascii85.go into ignored bounded development cache, with per-file size/SHA256 recorded privately. Primary module is Go1.24.1 with no require block; the BSD-3-Clause license requires source and binary notice retention. This verifies the inspected pin, not parser adoption or safety. [Pinned module](https://raw.githubusercontent.com/ledongthuc/pdf/6c8c28e0e8a07a3452f9d8cf3090ed6e856d8f24/go.mod), [pinned license](https://raw.githubusercontent.com/ledongthuc/pdf/6c8c28e0e8a07a3452f9d8cf3090ed6e856d8f24/LICENSE).

Root independently inspected concrete parser risks: read.go224-236 follows Prev without a visited/cycle guard;254-258 allocates xref from unbounded Size;308-330 accumulates widths and allocates without application cap;755-785 resolves object-stream Extends repeatedly;867-884 constructs predictor buffers from metadata. Reader filter chains return io.NopCloser, so guarded adoption must retain/close underlying decompressors and meter each decoded stage. lex.go68-73 synthesizes newline at allowed EOF and184-198 loops whitespace inside hex strings, requiring attempted-read work checks even at EOF. ps.go167-178 discards non-runtime panic values, so typed budget/cancellation panics must propagate. page.go64-81 aggregates whole output and caches fonts by resource name across pages; guarded extraction must budget before append and key font identity. These findings confirm the prior design's main concerns from actual pinned source.

Additional correctness concern: page.go619-629 handles the double-quote text operator with a three-argument check, then falls through a one-argument check and rejects it. A future fork must preserve valid PDF text operator semantics with an explicit regression, not adopt upstream output behavior blindly. No PoC/runtime/fuzz/native/resource test has been run; unrestricted raw Reader/Value APIs are not accepted application interfaces. Guarded implementation remains pending.
