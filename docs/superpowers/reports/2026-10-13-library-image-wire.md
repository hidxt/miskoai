# Inactive image wire groundwork

Completed local source handoff 2026-10-14 by gpt-6.1-sol / reasoning_effort medium, no children. Engineering remains **74% (37/50)**. These inactive primitives earn no package40, native CI, live delivery or final acceptance credit. Root owns adoption, independent review, integration, decisions and Git.

## Scope and source

Five new owned files only: `internal/channel/weixin/image_upload.go`, `image_upload_test.go`, `send_image.go`, `send_image_test.go`, and this report. Work occurred in ignored `.tools/reviews/library-image-wire-src`; no main source, existing client/types/ACK/media, Agent/Core/Store, interface, schema, dependency, LICENSE or notice was edited. Before copying, all **419** entries of `library-bundle-install-root-source-manifest.json` passed exact SHA256 and length verification against `library-bundle-install-root-reviewed-src`.

Read the MAIN 2026-10-13 library-image-wire plan, repository policy/state documents, private-library capability report and existing Weixin/media/ACK evidence. Inspected cached pinned Tencent source (`api/types.ts`, `cdn/upload.ts`, `messaging/send.ts`), existing exchange/ACK helpers and guarded `media.CDN.Upload`. Root's independent 11-file public byte comparison is separate evidence in `library-reply-official-compare-oct13.json`; this author performed no new public fetch. Source is Tencent/openclaw-weixin commit `24de5c9eb0dd5e595d7e2d090ed8a3f82870d42c`, MIT. Original attribution, LICENSE and notices are retained.

## Contracts and limitations

`GetImageUploadURL(ctx, ImageUploadRequest)` makes exactly one authenticated getuploadurl request with image media type1, actual caller-supplied plaintext byte count/MD5, random filekey/AES hex metadata, padded ciphertext count, no_need_thumb:true and existing base_info. Lowercase32hex values and plaintext1..4MiB are admitted before dispatch. Caller must supply actual validated immutable content, actual hash and random keys; this method performs no file read or crypto and MD5 provides no authentication.

The location parser requires complete valid UTF8 JSON object/EOF, recognized-field uniqueness under encoding/json-equivalent case folding, integer status fields and bounded control-safe string metadata. It honors -14 auth expiry before malformed/duplicate metadata, rejects malformed/null statuses and honors nonzero statuses. Genuinely absent ret succeeds only with valid nonempty original upload metadata. Optional thumbnail metadata is bounded and checked but discarded. Errors return a zero location and constant classified errors. Unknown JSON values remain slices of the bounded response through existing validated scanners.

Returned UploadParam/UploadFullURL is **untrusted metadata**. No location is fetched here. Any later caller must use existing `media.CDN.Upload` URL/IP guards; there is no new host policy or full-URL authorization. Source-shaped metadata may therefore pass this parser and still be refused by CDN validation.

`SendImage(ctx,to,contextToken,clientID,ImageSend)` makes exactly one authenticated send request, one type2 image item, explicit empty from_user_id, message_type2/state2, encrypt_type1, canonical base64 of ASCII lowercasehex32 AES key and ciphertext mid_size16..4MiB+16 divisible16. Identity/context/clientID/query values require nonblank bounded UTF8/control-safe input. The exact existing `sendAcknowledgement` classifies API acknowledgement. Missing/malformed acknowledgement and possible-send transport/read/oversize/5xx retain ErrOutcomeUnknown; no retry, text fallback, caption, mixed items, replay, run ID or invented thumbnail/dimension/ref fields exists.

Both methods reuse existing one outbound slot, cancellation/deadline/exchange limits and15s deadline. Existing response Close runs inside admission. No secret/body/location/raw remote error logging was added. This is source-only image representation evidence; no observed peer delivery, GIF animation, native sticker support or client-ID idempotency is established. Newly approved future text-then-optional-image strategy remains a separate dependent implementation, requiring per-part durable states and truthful history/accounting/generation lifetime. These methods do not compose or activate themselves.

## Actual verification and retained interruptions

Offline normal Windows host Go version was **go1.27.2 windows/amd64**, exact verified bundled binary; GOTOOLCHAIN=local, GOPROXY=off, GOSUMDB=off and repository Go/module/build/tmp caches. Runner removes any MISKOAI_* environment names without values. Synthetic in-package injected RoundTrippers capture actual serialized HTTP requests and exercise actual exchange, without a real endpoint or private fixture. No Core.Run, private DB/chat/key, provider/CDN/WeChat, paid/live, server, race, whole-product/static, publication or native CI operation occurred.

Commands are preserved in `library-image-wire-runner.ps1` and logs under the same unique prefix:

| Phase | Actual command | Named PASS events / distinct | FAIL | SKIP | Actual exit |
|---|---|---:|---:|---:|---:|
| RED initial | go test -count=1 -json -timeout=90s -run '^TestImage(Upload\|Send)' ./internal/channel/weixin | 2 / 2 | 33 | 0 | 1 |
| RED corrected fixture metadata | same selector | 2 / 2 | 33 | 0 | 1 |
| GREEN1 | go test -count=1 -json -timeout=90s ./internal/channel/weixin | 184 / 184 | 0 | 0 | 0 |
| GREEN2 final | same full scoped package command | 186 / 186 | 0 | 0 | 0 |
| scoped vet | go vet ./internal/channel/weixin | n/a | n/a | n/a | 0 |

RED is compilable refusal-only method stubs with semantic capture/location/ACK failures, not a compile failure. Initial test fixture expected bot_agent MiskoAI instead of actual MiskoAI/0.1.0; corrected before RED2 freeze, retaining initial source/logs. GREEN1 completed before quota interruption and was not rerun unchanged. GREEN2 follows only added padding-boundary/actual-Close slot tests and source attribution comments. Vet produced no diagnostics; PowerShell's empty pipeline produced no diagnostics file, recorded explicitly rather than inferring a file from an attempted Get-Item.

The first post-RED2 freeze command used a relative path after the runner changed the caller location, so Copy-Item refused with a duplicated source path before copying. Corrected absolute freeze preserves four RED2 files and manifest; the accidental nested `.tools` empty artifacts remain excluded from the canonical finalsource export. Initial four generic RED copies were renamed to unique `library-image-wire-red1-*` without deleting their bytes. Broad initial rg inventory produced noisy/truncated output; a Windows wildcard rg invocation also refused two path arguments. These setup/read errors and quota interruption produced no acceptance evidence; no assertions, refusals or checks were weakened. Original logs/source remain retained.

Tests cover exact request route/auth/base_info/single-item/key representation. Padding covers the 4 MiB plus 16-byte boundary; plaintext cases cover 1, 15, 16 and 17 bytes, input refusal before dispatch, absent/null/malformed/casefoldduplicate/status precedence/location control/UTF8/trailing/type/size refusal, untrusted full-URL metadata, HTTP401/429/400/503, safe transport/read errors, response cap/truncation, response Close and slot release, held-slot cancellation and cancellation after dispatch. Existing text, strict ACK, QR, receiver and slot regressions are unchanged and included in the complete scoped186-pass run.

## Self-review and handoff

Read complete owned production/test files and existing exchange/scanners; checked exact pinned outbound field/type/key/size conventions against capture assertions. Confirmed recognized duplicates cannot overwrite earlier location/status, unknown fields cannot become success, auth expiry precedence and zero-output errors, complete JSON validation before scanners, no added runtime callsites and fixed one-image array. No code outside five owned additions changed in the canonical finalsource. Source/evidence manifests record actual SHA256 and byte counts; an additions-only diff and five-file copies are frozen under unique library-image-wire artifacts. No unresolved technical blocker within this inactive primitive scope. Fresh root review/integrated checks and operational activation are pending. Author quiescent after handoff; no Git mutation.
