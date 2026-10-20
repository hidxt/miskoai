# Finite real WeChat static-image capability verification

2026-10-20. Engineering **82%** =41/50 after package40 acceptance at its declared implementation/static-image capability gate. Native integrated CI, reconnect/authorization-expiry24, security/UI/resource/final release gates remain separate.

The owner explicitly approved the reviewed finite tool, its five-request maximum/no-retry policy, existing local authorization read, fresh synthetic marker and two-part text-then-image result. They then confirmed that both the new fixed text reply and small PNG were received on the authorized peer. This is real peer receipt evidence, not an inference from server acknowledgement alone.

Reviewed executable SHA256: `ef84ea19ed4b91ff125a39aac1fedad27016dd569de3acb9376ffd4b64d251d5`. Primary had independently verified its43 scoped synthetic tests, vet/build/default exits0 and all504 frozen source identities; fresh permitted Medium review's cleanup correction was confirmed with0unresolved/0newC/I/M. The tool remained isolated from the normal private service and actual legacy database.

Actual authorized read-only preflight exited0 with `preflight_passed`, no poll/preparation/send attempt. The subsequent single live invocation exited0 with empty stderr and these fixed results:

```json
{"code":"image_ack_passed","poll_attempted":true,"preparation_attempted":true,"text_attempted":true,"text_acknowledged":true,"text_persisted":true,"image_attempted":true,"image_acknowledged":true,"image_persisted":true,"cleanup":true}
```

The invocation used the real Weixin client, reviewed upload locator/CDN transport, local image processor/Manager, replyimage Preparer and independent durable schema6 receipts in a fresh private synthetic directory. A generated32x32PNG passed the actual validation/preparation/encryption/upload path before either reply. Strict text acknowledgement was durably persisted before the image attempt; strict image acknowledgement was separately persisted. The attempt latch is retained and never reset, so this invocation cannot restart/replay. No retries or second invocation occurred.

No paid model/search/vision request was made. Existing authorization stayed on the local computer; no token/recipient/context/upload URL/key material entered logs, public records or the supplied server. The actual private legacy schema2 database was not opened or migrated. Only the fixed synthetic marker was admitted; no ordinary private service/Core.Run or ordinary-chat processing was started.

The original master specification §15 explicitly permits supported image fallback when native WeChat stickers cannot be sent. This verifies static PNG image wire capability. Existing reviewed Agent/Core optional PNG/JPEG composition, generation capability checks, conservative selection/frequency and two independent once-only receipts have source/local integrated evidence; this manual capability tool does not claim a real random-selection/frequency run. GIF animation/native sticker wire support remains unsupported/unverified, not silently enabled. JPEG is covered by the declared local validation/composition gate, not a separate live JPEG delivery claim. Those limits remain visible for final functional/delivery review.

Private development evidence: `weixin-image-live-oct20-preflight.json`, `-preflight-exit.txt`, `weixin-image-live-oct20-result.json`, `-exit.txt`, empty stderr and the retained synthetic attempt/receipts. Primary re-read the fixed outputs and actual exit0; owner separately confirmed peer receipt. This does not accept24, native42, final45, resource47/48 or release50.
