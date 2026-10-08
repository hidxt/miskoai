# DOCX admission and extraction bounds: design evidence

Research date: 2026-10-09. Installed toolchain: `go version go1.27.0 windows/amd64`; GOROOT: `C:\Program Files\Go`. This is a proposed design, not an implemented or accepted parser. No network calls, private-file reads, runtime parser tests, Linux execution, hard deadline proof, or RSS measurements were performed. No dependency is proposed. Native Go code can ship in the single Linux binary; source inspection on Windows is not Linux acceptance evidence.

## Installed standard-library evidence

Paths and line numbers below refer to the installed Go 1.27.0 source, not an independently downloaded release.

- `C:/Program Files/Go/src/archive/zip/reader.go:118`: NewReader has no application file-count or metadata-budget option. Line 132 preallocates based on archive-size plausibility. Lines 145–157 read central headers until a bad header and compare entry counts modulo 65,536, without confining parsing to the declared central-directory size.
- `reader.go:382`: central-header parsing allocates filename, extra and comment bytes before returning a File. Checking entries after NewReader is too late for an admission allocation cap.
- `reader.go:544`: EOCD search uses at most a 65KiB buffer; it accepts ZIP64 sentinels and adjusts archive base offsets. The strict application subset should reject ZIP64, multipart archives and offset ambiguities.
- `reader.go:338`: findBodyOffset verifies a local signature and reads local name/extra lengths, but does not reconcile local method, flags, filename and sizes against central metadata.
- `reader.go:294`: actual decoded size is checked during reads; exact length and CRC are checked at decompressor EOF. Close only closes the decompressor. At line 324, CRC comparison without a descriptor is skipped when the declared CRC is zero. An independent CRC comparison must include zero.
- `C:/Program Files/Go/src/encoding/xml/xml.go:275`: Token maintains element and namespace stacks without an application depth cap. Application checks after Token cannot prevent allocation of the start token and its attributes.
- `xml.go:996`: text buffering has no application token-size setting. Attribute lists and namespace state also grow before Token returns.
- `xml.go:1098`: Strict=true with Entity=nil does not expand custom named entities. Predefined/numeric entities remain supported. DTD directives do not install entity definitions; reject directives anyway. Keep CharsetReader=nil to reject unsupported encodings.

## Proposed restricted ZIP admission

Use an immutable bounded byte slice or private spool exposed through ReaderAt. Enforce 4MiB during upload/spooling. Do not validate a mutable external file and then parse its changed contents.

Before calling zip.NewReader:

1. Search at most 65,557 tail bytes for an unambiguous EOCD with its comment ending exactly at input EOF. Require one disk, matching per-disk/total counts, no ZIP64 sentinels or locator, and centralOffset+centralSize equal to EOCD offset. Reject nonzero archive base offsets and prepended data.
2. Walk central records with a fixed 46-byte buffer and overflow-safe subtraction/addition. Proposed additional limits: 256 entries, 256KiB central directory, 512-byte filenames, 4KiB extra fields, 1KiB per-entry comments. Require exact count and exact central-directory boundary.
3. Reject ZIP64 extra field 0x0001, multipart disk-start fields, unsupported compression and unsupported flags. Permit Store/Deflate; permit descriptor bit 3, UTF-8 bit 11, and Deflate option bits 1–2 only for method 8. Reject encryption, strong encryption and masked-header flags.
4. Require valid UTF-8 names and slash separators. Reject absolute/drive paths, NUL, backslashes, empty components, dot/dot-dot components, duplicate canonical names, symlinks and other special-file modes. Allow empty ordinary directory entries. Never extract to disk or recursively parse nested archives.
5. Sum advertised uncompressed lengths of all entries, including unused images/objects, against 8MiB using subtraction to avoid overflow. This intentionally supports a limited DOCX subset.
6. Validate local 30-byte headers, filenames, extra fields, flags and methods. Require all local/data/descriptor ranges before the central directory, inside input bounds and nonoverlapping; reject repeated local offsets. Without descriptor bit 3, require local CRC/sizes to match central metadata. With bit 3, explicitly validate non-ZIP64 12/16-byte descriptors and central agreement.
7. Call zip.NewReader only after preflight. Cross-check returned count, names, sizes and offsets against the accepted manifest. Select exact fixed XML part names; do not follow arbitrary relationships or remote URLs.

Stream decoded members under a shared 8MiB actual-decoded-byte counter. Read to actual EOF; compare actual length and independently computed CRC even when expected CRC is zero. Validate all members if claiming whole-archive integrity; otherwise describe integrity as selected-member integrity. Reject on limits; do not return successful truncated extraction.

Decoded EOF through File.Open does not establish consumption of every advertised compressed byte. For strict compressed exhaustion, use OpenRaw and flate.NewReader on a bounded reader implementing io.ByteReader, count consumed bytes, and require exact advertised compressed consumption. Compute size and CRC independently. Store entries require compressed/uncompressed sizes to agree. This is extra implementation work with no dependency.

## XML and extraction budgets

Use strict Decoder.Token with no custom entities or charset conversion. Validate the document root and explicit permitted WordprocessingML namespaces. Extract text from w:t and supported tabs/breaks; reject directives. Enforce 128KiB UTF-8 output and 64 logical page breaks; define page-break recognition precisely in implementation/tests. Paragraphs are not automatically pages.

Proposed depth limit: 64. The 8MiB decoded budget alone bounds input but does not establish a small RSS bound: one start token can create many attribute/namespace objects before caller checks. For tighter admission, put a streaming lexical guard before Decoder with proposed limits of 16KiB start tags, 128 attributes per start tag and 64KiB text/comment/CDATA/PI tokens. It must correctly handle quoted values, comments and CDATA; splitting blindly on angle brackets is unsafe. Guard behavior needs focused tests. A smaller per-XML-part cap is a conservative alternative, but still requires worst-case allocation assessment.

Use one parser worker with bounded admission/queueing. Check context in Read/ReadAt, between ZIP records/XML tokens and during output writes, using a 2-second deadline. Cancellation is cooperative; standard-library work inside a call may run until another check. Input/token/entry limits constrain this interval but do not prove a hard CPU cutoff. No hard 2-second or 512MB RSS claim is supported by this research.

## TXT and Markdown

Stream bounded chunks with strict UTF-8 validation and up to three carry bytes for split code points. Strip exactly one initial UTF-8 BOM; reject UTF-16/32 BOMs and invalid/truncated UTF-8. Enforce output/deadline bounds before writing. Treat Markdown as text without link fetching or HTML execution. If using Scanner, explicitly configure its token limit rather than inheriting an accidental default.

## Proposed synthetic verification

- ZIP64 enormous counts, count mismatch/modulo wrap, central bounds, sparse offsets, oversized metadata, EOCD ambiguity.
- Encryption/unsupported flags, duplicate names, traversal, backslashes, symlinks, nested ZIP left unprocessed.
- Local/central mismatch, overlapping ranges, malformed descriptors, Deflate bomb, declared size under/overflow, truncated streams, compressed trailing bytes and CRC mismatch including declared zero.
- XML nesting, huge start tags/attributes/namespaces, giant CharData/comments/CDATA, DOCTYPE/custom entity, malformed UTF-8, unsupported encodings, output and page limits.
- TXT BOM, split multibyte code points, invalid/truncated UTF-8 and rejected UTF-16/32.
- Expired contexts, cancellation during preflight/decode, worker saturation, exact-bound success and one-unit-over rejection.

Commands used for evidence: Get-Content on required project policy documents; go env GOROOT; rg source-symbol searches; bounded source excerpts; go version. No runtime tests were executed. Parent integration must review the parser implementation and run synthetic, native Linux and resource tests before acceptance.
