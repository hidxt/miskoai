# Bounded attachments and media integration design

2026-10-09 root design, not implemented. References: pinned Weixin media evidence, DOCX/PDF bounded-source research and existing Core/config/privatefs plans. Existing requested Go/SQLite/native2C512MB route stays fixed. No private media, live API or actual service is used during development. A later task-specific plan must precede implementation.

## Provenance and persistence

Persist authorized scope and original message ID before any download/parser/model/send. A compatible receive-envelope schema extension may carry bounded typed image/file metadata; retain exact prior manifests and admission-before-materialization. Metadata must be included in global queue byte quotas, not hidden outside current bounds. Normalize only after fixed user/account checks and existing message/item-count guards. One attachment per initial message flow; classify additional items as a safe explicit unsupported/limit response rather than silently using arbitrary media. Current raw frame remains durable evidence, queue remains bounded and serial handling preserves the one-send claim/state boundary.

The messages USER-evidence column contains only genuine caption/text supplied by the user. Never place OCR, extracted document body, generated placeholders, media URLs/keys or assistant analysis in that column. Attachment-only handling can retain an empty genuine caption and claim/state; history's nonempty filter then excludes fabricated provenance. Parsed documents are quoted external data in the model context. Candidates must still prove actual genuine USER text; image/document output cannot become confirmed facts automatically.

## Private spool and shared admission

One admitted media/parser operation, including wait/download/decrypt/format/parser lifetime, and no detached timeout worker. Shared privatefs creates random exclusive spool files, never user filenames as paths. At most4MiB plaintext/4MiB+16 ciphertext; separate image dimensions and decoded-pixel/all-animation cumulative limits. Incomplete private files are not passed to downstream consumers; validate final PKCS7/length/optional MD5/format before exposure and clean owned artifacts on every cancellation/error. Cryptographic compatibility is AES128ECB/PKCS7 and MD5 only as protocol metadata; neither is an authenticity guarantee.

Use existing transport public-IP answer-set pinning/no redirect/proxy/TLS checks, exact https novac2c.cdn.weixin.qq.com:443 /c2c/download or /c2c/upload routes and bounded query. Returned full URLs must pass the same policy. No API bearer, cookie or chat token on CDN requests; no raw URL/key/filename/body in errors/events/model context. Authenticated getuploadurl stays on official API transport. Upload has zero initial automatic retries because reference retries do not prove idempotency. Failed upload is distinct from a submitted chat send.

Text/summary/vision delegates share the same DeepSeek instance's transport/admission limit2. A model-selection clone must share that instance's transport rather than creating a new two-slot pool. Vision base64/JSON request construction must be assessed for copies and bounded before allocation; spool transport and one media admission bound concurrent image work. Source-label/caption/excerpt data are lower-role only and never grant tools.

## Document contract

TXT/Markdown strict UTF8/BOM validation; Markdown remains inert text. DOCX is the restricted ZIP/XML subset in the research:4MiB input,256entries/256KiB metadata,8MiB actual total decode,128KiB output,64depth and token/attribute guards before XML materialization; independently validate CRC including zero and compressed exhaustion. No extraction to user paths or external relationship fetch.

PDF candidate remains a guarded attributed internal fork only after all allocation/work/cycle/decoder/text guards are implemented and reviewed. Input4MiB/pages64/decoded8MiB/text128KiB, one worker and cooperative2s context, no abandoned goroutine or hard RSS/time claim. Root verified upstream correctness/resource concerns; unrestricted upstream APIs are not accepted wrappers. Preserve all required BSD notices in source and binary. Valid declared Latin/CJK examples and adversarial/fuzz/native/resource tests precede claims. Scanned/image-only PDF has an honest unsupported OCR result, never invented text.

A128KiB extracted text result does not mean the model saw all of it. Context remains the reviewed byte/token budget; select a bounded UTF8-safe excerpt fitting mandatory current/safety input, label the exact visible range deterministically in the reply/request, and expose full extraction only through a bounded authenticated UI if implemented. Never claim whole-document understanding from a partial prefix.

## Reply and sticker boundary

A media reply selects one allowlisted image item and performs one sendmessage through the same reviewed strict ACK classifier. No second caption send is added under one-send authorization. Persist sending before submission; any uncertain response retains ambiguity and never replays. Caption+media combination must use a source-proven single envelope or declare unsupported, not guess protocol.

Local image/GIF library needs bounded SQLite metadata (hash/tags/emotion/description/enabled/usage) and private immutable content files, fixed quotas, enable/delete and frequency checks without extra model calls. Library effects remain separate from native WeChat sticker capability. Source proves ordinary image routing, not native sticker or animated GIF delivery; live tests must identify accepted forms, and any supported image fallback must be labeled honestly. No clickable upload/send UI before actual backend implementation.

## Separate acceptance gates

Synthetic tests for encrypted block/padding/size/digest, metadata scope/URL refusal before network, source proof isolation, malformed format/pixel/animation/ZIP/XML/PDF work bounds, canceled actual worker joins, queue saturation, source-visible excerpt labeling and one-send ambiguity. Fresh Medium protocol/resource/security reviews plus root full integration/native race/scanners/license checks. Existing finite cloud allowance is spent; additional paid vision calls need a new concrete bounded request. Human-controlled WeChat image/file/reconnect/expiry tests and authorized real512MB workload are separate. No parser/media/resource pass is inferred from this design.
