# Restricted PDF syntax fork

This private package adapts the previously fetched and locally hash-verified `github.com/ledongthuc/pdf` source at commit `6c8c28e0e8a07a3452f9d8cf3090ed6e856d8f24`. No Go dependency or additional runtime is introduced. The source inventory supplied by the primary is `.tools/reviews/pdf-source-inventory.json`; its eight entries were independently verified before copying/adaptation. Its included `page.go`, `text.go`, and `go.mod` are not adopted here.

Copied/adapted source and exact supplied SHA256:

| File | Supplied bytes | Supplied SHA256 |
| --- | ---: | --- |
| read.go | 29742 | 9d50650d709d055f8b971eee64879fd3c43c2408390c36f271139e95d0cb457e |
| lex.go | 10727 | 5f4045230a2b7362f15efe7740fb6a8ca53f5ef2fc8fb9b7f87f6e4f86b481e0 |
| ps.go | 5322 | 197f195072bd96217c05649829ab2bcc0eaf1cb5095a8c274626bef60218fe2a |
| ascii85.go | 751 | 3f3ff0f8aac6af54a9cbca5b93a65dfb2a90d40bd5e5733c4c62fba4f169742e |
| LICENSE | 1479 | 2d36597f7117c38b006835ae7f537487207d8ec407aa9d9980794b2030cbc067 |

LICENSE is copied byte-for-byte; existing copyright/license headers in the three attributed Go sources are retained. The pinned ASCII85 helper had no copyright header; its original introductory comments are retained with an adaptation pointer. The primary must include this complete BSD 3-clause license and source inventory in the embedded binary notices BEFORE any publication of this source or binary. This file does not satisfy the separate binary-notice gate.

Changes:

- Private `parse(context.Context, io.ReaderAt, int64)` and package-private reader/value/interpreter/stack types. The ReaderAt remains caller-owned and must represent an admitted, completed immutable private input; the fork has no filesystem/network entry point.
- Remove Open, exported NewReader/Reader/Value/Interpret surfaces, debugging, recursive value formatting, password/cryptographic implementation and unrestricted Value.Text/TextFromUTF16. Encrypted PDFs refuse with one fixed unsupported error.
- A cumulative shared budget governs context, cooperative two-second deadline, attempted byte/read work including EOF, container/lookup/xref nodes, retained allocations, graph/lexer depth, xref revisions, all-stage decoded data, active streams, operands and dictionary scopes. Counts do not reset across resolutions, Prev/Extends, streams or filters.
- Xref table/stream metadata and object-stream sizes/offsets/widths are checked before allocation, multiplication or narrowing conversion. Growth reserves replacement allocations including copied prefixes and charges copy work. Active indirect identities/offsets and xref/Extends visited sets reject cycles. Incremental revisions may retain a smaller older Size under the latest admitted Size.
- Lexical token/container growth is bounded before append/allocation. EOF is remembered, attempted bytes remain metered, malformed unterminated containers/strings fail, hex odd-nibble padding is supported, and errors never format document bytes or values.
- Filter chains have explicit ownership of all actual io.Closer decoders, close on success/error/cancel/construction failure, retain EOF after successful automatic close, and share one all-stage decoded counter. Inner stages' charges are rechecked by outer stages before adding output. No goroutine/timer-worker timeout is used in production.
- Supported filters are FlateDecode and ASCII85Decode. Supported PNG-Up predictor is12 with one color and eight bits per component; a predictor row INCLUDING its marker is limited to4096bytes. Other filters/predictors and hybrid XRefStm tables are explicitly unsupported. Interpreter dictionary handling is strict and never recovers-and-continues on malformed input or cancellation.
- FIX1 validates object-stream decoder integrity through actual EOF before exposing an object. Decoded storage grows only after cumulative retained admission, including each replacement capacity and copy work. Header IDs are unique, offsets strictly ordered, current xref indices consistent, and each member is parsed within its exact bounded span. Unexpected value closers, trailing extra objects, nested definitions and embedded streams refuse safely. All decoded data, scratch, header metadata, lexers and validation share the original budget; failures close the actual decoder chain.

Ceilings: input4MiB; cumulative nodes32768; xref revisions32; graph/lexer depth64; individual token64KiB; cumulative retained token/container/allocation reservations8MiB; all-stage decoded8MiB; filter chain4; predictor row4KiB; active streams256; operands256; dictionary scopes64; attempted work32Mi operations; cooperative2seconds. Admission includes conservative scratch/capacity reservations and repeated replacement/copy allocations, so inputs below an individual ceiling may still exhaust a shared ceiling. These are engineering guards, not RSS or hard-time guarantees. A blocking caller ReaderAt cannot be forcibly interrupted; its actual call must return and be joined before releasing the application's parser admission.

Task2 remains responsible for page walking64, output128KiB, strict Unicode/fonts/CMap entries8192/code4bytes/replacement1024bytes, and the synchronous document.Parser integration. These constants are reserved here but are not a claim that extraction exists. No page.go/text.go are copied solely to compile. `name.go` is deliberately excluded: external glyph-table provenance is unresolved; no table/source/license inference is made. No PDF actions, JavaScript, URLs, embedded files, fonts or external resources are executed or fetched.

The initial independent correctness review requested changes for object-stream range isolation, unread corrupted Flate tails and unexpected value closers; FIX1 adds finite RED/GREEN regressions for those three findings. Two minor findings remain explicitly deferred to Task2 before extraction acceptance: literal newline/escape/octet compatibility, and complete `endstream`/`endobj` syntax validation. This package does not claim complete PDF syntax or text compatibility. Exact object spans and whole-stream validation may conservatively reject otherwise usable PDFs under the unchanged shared resource limits. The primary reports that the initial BSD notice/inventory embed is completed; this author does not own or certify its publication gate.
