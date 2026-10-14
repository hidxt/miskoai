# Weixin response Close correctness fix

2026-10-14, gpt-6.1-sol / reasoning_effort medium, no children. Engineering remains **74% (37/50)**. This narrow local correctness fix adds no package40, native CI, live delivery or final acceptance credit. Root owns independent review, integrated checks, adoption and Git.

## Source and cause

Read the authoritative 2026-10-14 response-close plan, existing exchange/ACK/image contracts and root's actual synthetic diagnostic. Root's separate425-file diagnostic returned nil in upload/text/image Close-failure and Close-cancellation cases: six subcases plus parent FAIL, zero PASS, actual exit1. No real channel request occurred. Existing `Client.exchange` deferred `res.Body.Close()` without consuming its error and returned successful bytes without checking context after actual Close. That shared boundary allowed a caller to consume metadata or acknowledge a send after uncertain cleanup. The existing media CDN closure provides a related checked synchronous cleanup pattern; this fix preserves exchange's own prior-error precedence instead of changing it to CDN's policy.

The exact424-file inactive image-wire root basis and `library-image-wire-root-oct14-source-manifest.json` passed SHA256 and length verification before copying into new isolated `.tools/reviews/weixin-response-close-oct14-src`. Existing source/author/reviewer evidence and original five image-wire additions remain unchanged. Ownership is THREE paths: narrow `internal/channel/weixin/client.go` exchange result/deferred Close change, NEW `internal/channel/weixin/exchange_close_test.go`, and this NEW report. All other423 basis files are exact; no main/shared source, ACK, modules, notices, LICENSE, policy/state docs, runtime interfaces, Agent/Core/Store, private data or Git state was edited.

## Result and limits

Exchange uses named results only so its synchronous response-Close defer can refuse an otherwise successful result. Close executes exactly once before admission release. If an error has already been classified, it remains unchanged; HTTP401/403/429/400/503, read failure and response-limit behavior retain their existing classifications, including when Close both fails and cancels.

An otherwise successful exchange returns bytes only after actual Close succeeds and context is still live. A possible-send Close failure or post-Close cancellation returns zero bytes and safe `ErrOutcomeUnknown`. A non-send Close failure returns zero bytes and safe `ErrTransport`; non-send cancellation returns its actual context error. Raw remote Close errors never escape. This change does not parse ACKs in exchange, alter endpoints/auth/host/timeout/slots/response limits, retry/fallback/replay, or start any additional send. A previously classified API error cannot become success. Image primitives remain inactive; future two-part orchestration must stop after a part becomes uncertain.

Cleanup remains synchronous; no goroutine is introduced in production and no abandoned operation or hard completion-time claim is made for an arbitrary uncooperative Close. Context is checked after actual Close, which is the cancellation boundary this task guarantees. Existing polling uses its existing dedicated slot; both outbound and polling cleanup ordering are tested.

## Executed TDD and verification

Synthetic RoundTrippers use real Client methods/exchange, real serialized request preparation, actual response Read/Close and existing admission. No public/private/provider/CDN/WeChat requests, real credentials/chats, Core.Run, private database migration, paid/live/server, native CI, whole-product/static/race or publication operation occurred.

Runner `weixin-response-close-oct14-runner.ps1` uses exact bundled verified **go1.27.2 windows/amd64**, GOTOOLCHAIN=local, GOPROXY=off, GOSUMDB=off and repository Go/module/build/tmp caches. It clears MISKOAI_* / DEEPSEEK_API_KEY / OLLAMA_API_KEY environment names without values. Actual version, stdout/stderr, JSON events and exit files are retained under that unique prefix.

| Phase | Actual command | Named PASS events / distinct | FAIL events / distinct | SKIP | Actual exit |
|---|---|---:|---:|---:|---:|
| Compiled semantic RED | go test -json -count=1 -timeout=45s -run '^TestExchangeClose' ./internal/channel/weixin | 34 / 34 | 21 / 21 | 0 | 1 |
| Final scoped GREEN | go test -json -count=1 -timeout=90s ./internal/channel/weixin | 241 / 241 | 0 / 0 | 0 | 0 |
| Scoped vet | go vet ./internal/channel/weixin | n/a | n/a | n/a | 0 |

RED used unchanged production exchange with the complete new test file and compiled successfully. Its21 failures reproduce lost Close failure/cancellation and successful bytes leaking after cleanup, including actual held-Close cancellation. Its34 passes establish live-context success and preexisting classified-failure controls. Complete425-file RED source/preimage manifest is frozen before the two narrow implementation edits. Final241-pass run includes all186 unchanged image-wire/text/ACK/QR/receiver/slot regressions and55 new named tests. Vet stdout/stderr are actual zero-byte files. No source/assertion/test timeout/skip was weakened, and no failing command was credited as passing.

New coverage includes upload/text/image and direct send/non-send exchange Close failure, cancellation and simultaneous failure/cancellation; live success controls; zero output on refusal; once request/Close/slot release; no raw error leakage; HTTP401/403/429/400/503/read/oversize precedence under cleanup errors and cancellation; and actual synchronous Close barriers holding outbound/poll slots until cleanup finishes. Barrier tests register cancel/unblock/WaitGroup cleanup before assertions and launch, so early failure unblocks and joins only work actually started. The root's original diagnostic remains separate immutable evidence and was not overwritten or rerun as an author result.

## Freeze and self-review

Unique artifacts include complete426-file finalsource (424 basis plus new test/report), three-owned-file copies and manifest, complete RED425 source/preimages and manifest, additions/narrow-change diff, actual runner/version/RED/GREEN/vet events/exits and evidence SHA256/byte manifest. Verify hashes and lengths rather than relying on filesystem names. Original image-wire quota/setup/hash-case/diagnostic evidence remains unchanged; this follow-up had no setup/quota/hash-case interruption and no retries of an unchanged successful test run.

Self-review read the complete modified exchange and new tests, checked Go named-result assignment/defer order, verified Close occurs before slot release and timeout cancel, checked failure precedence and safe errors, confirmed all successful bytes are withheld on cleanup uncertainty, and checked other423 source identities. No unresolved technical blocker in the narrow scope. Fresh independent review/root integrated evidence remains required. Author quiescent after handoff, with no main adoption or Git mutation.
