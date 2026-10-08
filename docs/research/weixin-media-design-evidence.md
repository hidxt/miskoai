# Pinned Weixin media design evidence

Research date: 2026-10-09. Public official Tencent/openclaw-weixin commit
`24de5c9eb0dd5e595d7e2d090ed8a3f82870d42c`, package 2.4.9. This is source
research and a proposal for primary review, not an implementation or runtime
test. No real WeChat/API calls, credential access, private media, service start,
code adoption, commit or backend acceptance test occurred. Public source copies
are cached only under ignored `.tools/media-research`.

## Inbound wire evidence

[types.ts lines 70–114, 129–134, 175–208](https://github.com/Tencent/openclaw-weixin/blob/24de5c9eb0dd5e595d7e2d090ed8a3f82870d42c/src/api/types.ts#L70)
defines image item type **2** and file item type **4**; these differ from upload
media types image **1**, file **3**. Item metadata includes `type`,
`create_time_ms`, `update_time_ms`, `is_completed`, `msg_id`, `ref_msg`.

| Object | Exact represented fields |
| --- | --- |
| `CDNMedia` | `encrypt_query_param`, `aes_key`, `encrypt_type`, `full_url` |
| `image_item` | `media`, `thumb_media`, `aeskey`, `url`, `mid_size`, `thumb_size`, `thumb_height`, `thumb_width`, `hd_size` |
| `file_item` | `media`, `file_name`, `md5`, `len` (string) |

All are optional in the TypeScript representation; this does not establish
optional server requirements. The image download path requires `media` with a
query parameter or full URL, prefers `image_item.aeskey` hex over
`media.aes_key`, and falls back to plain download when neither key is present.
It does not consume `image_item.url` or thumbnail media. The file path requires
`media.aes_key` and a query parameter or full URL. It uses filename-derived MIME
and does not compare declared `len` or `md5` to downloaded bytes.
[media-download.ts lines 40–69 and 100–128](https://github.com/Tencent/openclaw-weixin/blob/24de5c9eb0dd5e595d7e2d090ed8a3f82870d42c/src/media/media-download.ts#L40).

## Keys, crypto and URLs

`media.aes_key` is base64 decoded; exactly 16 decoded bytes are a raw AES key;
exactly 32 ASCII hexadecimal characters are hex decoded to 16 bytes. Other
forms fail. `image_item.aeskey` represents the raw key as 32 hex characters.
[pic-decrypt.ts lines 30–51](https://github.com/Tencent/openclaw-weixin/blob/24de5c9eb0dd5e595d7e2d090ed8a3f82870d42c/src/cdn/pic-decrypt.ts#L30).
AES is 128-bit ECB with PKCS#7 padding; ciphertext size is
`16 * (floor(plaintext_size / 16) + 1)`, including a whole extra block for
block-aligned input. No IV, MAC or authenticated-encryption envelope is shown.
[aes-ecb.ts lines 6–20](https://github.com/Tencent/openclaw-weixin/blob/24de5c9eb0dd5e595d7e2d090ed8a3f82870d42c/src/cdn/aes-ecb.ts#L6).

The documented default CDN origin is `https://novac2c.cdn.weixin.qq.com`, base
path `/c2c`. Source builders produce `/c2c/download?encrypted_query_param=<URL-encoded parameter>`
and `/c2c/upload?encrypted_query_param=<URL-encoded upload_param>&filekey=<URL-encoded filekey>`.
Full inbound `full_url` and outbound `upload_full_url` take precedence in the
reference. These are source-supported routes, not evidence that arbitrary
returned origins are safe or authorized.
[protocol.md lines 18–24](https://github.com/Tencent/openclaw-weixin/blob/24de5c9eb0dd5e595d7e2d090ed8a3f82870d42c/docs/protocol.md#L18),
[cdn-url.ts lines 8–19](https://github.com/Tencent/openclaw-weixin/blob/24de5c9eb0dd5e595d7e2d090ed8a3f82870d42c/src/cdn/cdn-url.ts#L8),
[pic-decrypt.ts lines 65–77](https://github.com/Tencent/openclaw-weixin/blob/24de5c9eb0dd5e595d7e2d090ed8a3f82870d42c/src/cdn/pic-decrypt.ts#L65).

## Upload and media send

The shared pipeline hashes plaintext with MD5 to lowercase hex, generates a
random 16-byte AES key and a random 16-byte file key rendered as hex, then
requests authenticated `POST /ilink/bot/getuploadurl` with `filekey`,
`media_type`, `to_user_id`, `rawsize`, `rawfilemd5`, padded `filesize`,
`no_need_thumb:true`, hex `aeskey`, and `base_info`. Response fields are
`upload_param`, `thumb_upload_param`, `upload_full_url`; the shared pipeline
uploads only the original. MD5 here is protocol metadata, not a secure
authenticity guarantee.
[upload.ts lines 70–119](https://github.com/Tencent/openclaw-weixin/blob/24de5c9eb0dd5e595d7e2d090ed8a3f82870d42c/src/cdn/upload.ts#L70),
[api.ts lines 490–515](https://github.com/Tencent/openclaw-weixin/blob/24de5c9eb0dd5e595d7e2d090ed8a3f82870d42c/src/api/api.ts#L490).

CDN upload is POST ciphertext with only explicit
`Content-Type: application/octet-stream`; no bot bearer is supplied. Reference
success requires HTTP 200 and nonempty response header `x-encrypted-param`,
which becomes downstream `media.encrypt_query_param`. Its retry loop makes up
to three attempts after non-client failures; that loop is reference behavior,
not a proven idempotency contract.
[cdn-upload.ts lines 24–90](https://github.com/Tencent/openclaw-weixin/blob/24de5c9eb0dd5e595d7e2d090ed8a3f82870d42c/src/cdn/cdn-upload.ts#L24).

The image builder sends type 2, `image_item.media` containing returned
`encrypt_query_param`, **base64 of ASCII hex key text**, `encrypt_type:1`,
and `mid_size` ciphertext bytes. The file builder sends type 4 with the same
media fields, `file_name`, and `len` plaintext bytes as a decimal string; it
does not populate `md5`. In particular, `Buffer.from(uploaded.aeskey)` does
not hex decode: matching this builder means base64 encoding 32 hex characters,
even for outbound images.
[send.ts lines 293–303 and 381–391](https://github.com/Tencent/openclaw-weixin/blob/24de5c9eb0dd5e595d7e2d090ed8a3f82870d42c/src/messaging/send.ts#L293).

The one-item send envelope is `msg:{from_user_id:"",to_user_id,client_id,
message_type:2,message_state:2,item_list:[item],context_token,run_id?}` plus
API base metadata. Caption and media are separate requests in the reference.
[send.ts lines 139–172](https://github.com/Tencent/openclaw-weixin/blob/24de5c9eb0dd5e595d7e2d090ed8a3f82870d42c/src/messaging/send.ts#L139),
[protocol.md lines 218–265](https://github.com/Tencent/openclaw-weixin/blob/24de5c9eb0dd5e595d7e2d090ed8a3f82870d42c/docs/protocol.md#L218).

## Minimal Go proposal requiring primary review

Keep only encrypted image/file transport first. Reject missing/invalid keys
instead of adopting the reference's unencrypted image fallback. Do not fetch
`image_item.url`, thumbnails, model URLs or filenames. Validate strict key
encoding, item type, decimal file length, bounded metadata and URLs before any
network work. Declared size/MD5 may reject a mismatch when supplied, but never
replace actual streamed byte bounds or content validation.

Use one media worker and bounded queue. Plaintext cap: **4 MiB**; encrypted
cap: **4 MiB + 16 bytes**. A 4 MiB plaintext encrypts to 4 MiB + 16, so a
single shared 4 MiB wire cap would incorrectly reject the maximum. Stream
ECB blocks with one final held block; reject empty/non-block-aligned ciphertext
and every invalid PKCS#7 byte. Use `crypto/aes`, `crypto/rand`, `crypto/md5`
only for wire compatibility, `encoding/hex`, `encoding/base64`, `net/http`
and existing reviewed transport primitives. No Node/OpenClaw runtime needed.

Spool to random exclusive files in a verified private directory, owner-only
permissions/Windows ACL, no user filename in a path. Keep incomplete plaintext
private and unavailable to downstream consumers until final padding, byte
limit, optional digest/length and content checks pass; clean on every failure
and cancellation. Streaming and spool quotas bound heap and disk; image
dimensions/decoded pixels still need separate caps. Do not claim 512MB RSS.

Proposed network policy: exact HTTPS host `novac2c.cdn.weixin.qq.com`, default
443, no userinfo/fragments, `/c2c/` paths only, bounded query/URL. Validate
server full URLs under the same policy; reject other origins pending review.
Retain returned bounded query values rather than inventing query semantics.
Use no redirects, no environment proxy, no bearer/API headers/cookies on CDN,
TLS hostname verification and DNS answer-set public-IP checks followed by
direct dial to the validated IP. Timeouts/cancellation include queue wait;
response-body/header limits and zero initial retries are conservative defaults.

Persist authorized scope, intended media item and stable client ID before one
`sendmessage` invocation; require explicit `ret:0` for success. Transport,
5xx, malformed/truncated/missing acknowledgement after possible submission
remain ambiguous and must not trigger another chat send. Keep upload failure
distinct from chat-send ambiguity; a failed upload must never cause a guessed
media send. Do not add a separate caption send to a one-send contract.

## Explicitly unproven

The reviewed enum has no sticker type or native-sticker builder.
GIF MIME is recognized and `image/*` is routed to ordinary image upload/send:
[mime.ts lines 27–36](https://github.com/Tencent/openclaw-weixin/blob/24de5c9eb0dd5e595d7e2d090ed8a3f82870d42c/src/media/mime.ts#L27),
[send-media.ts lines 14–16 and 46–57](https://github.com/Tencent/openclaw-weixin/blob/24de5c9eb0dd5e595d7e2d090ed8a3f82870d42c/src/messaging/send-media.ts#L14).
This does not prove animated delivery, native sticker display, backend format
acceptance, thumbnails being optional on the server, client-ID idempotency,
upload retries being safe, alternate CDN hosts, quotas, retention/deletion,
or independent Go-client acceptance. Do not guess undocumented endpoints.
Local sticker metadata can remain separate from native channel capability.
Real media verification remains deferred with all other real WeChat testing.

Validation performed: read pinned public source and line-numbered cached
files. No media code/tests/builds executed; this report makes no runtime claims.
