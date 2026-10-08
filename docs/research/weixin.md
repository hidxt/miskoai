# Weixin protocol feasibility and text PoC

Research date: 2026-10-09. Official repository main resolved through GitHub's
commit API to **24de5c9eb0dd5e595d7e2d090ed8a3f82870d42c**, package **2.4.9**.
This note distinguishes source evidence, synthetic tests and live acceptance.
Initial research used synthetic fixtures. Later authorized QR login confirmed after one expired attempt; text receive failed before claim/send. The owner deferred further real WeChat testing. No text send success is claimed.

## Source evidence

- [Protocol reference](https://github.com/Tencent/openclaw-weixin/blob/24de5c9eb0dd5e595d7e2d090ed8a3f82870d42c/docs/protocol.md): HTTPS JSON, bot endpoints, headers and media wire formats. The document explicitly says client types/behavior do not establish the complete server contract.
- [API implementation](https://github.com/Tencent/openclaw-weixin/blob/24de5c9eb0dd5e595d7e2d090ed8a3f82870d42c/src/api/api.ts): POST headers, encoded client version, base_info and lossless uint64 identifiers.
- [QR implementation](https://github.com/Tencent/openclaw-weixin/blob/24de5c9eb0dd5e595d7e2d090ed8a3f82870d42c/src/auth/login-qr.ts): fixed bootstrap, verification code, statuses, redirect host and confirmed credentials.
- [Monitor](https://github.com/Tencent/openclaw-weixin/blob/24de5c9eb0dd5e595d7e2d090ed8a3f82870d42c/src/monitor/monitor.ts): 35-second polling default, cursor, server timeout hint, retry/backoff and auth expiry.
- [Session guard](https://github.com/Tencent/openclaw-weixin/blob/24de5c9eb0dd5e595d7e2d090ed8a3f82870d42c/src/api/session-guard.ts): -14 pauses an account for one hour in the reference.
- [Send builders](https://github.com/Tencent/openclaw-weixin/blob/24de5c9eb0dd5e595d7e2d090ed8a3f82870d42c/src/messaging/send.ts) and [inbound mapping](https://github.com/Tencent/openclaw-weixin/blob/24de5c9eb0dd5e595d7e2d090ed8a3f82870d42c/src/messaging/inbound.ts): client IDs, reply context, peer identity and persisted context tokens.
- [Package metadata](https://github.com/Tencent/openclaw-weixin/blob/24de5c9eb0dd5e595d7e2d090ed8a3f82870d42c/package.json) and [MIT license](https://github.com/Tencent/openclaw-weixin/blob/24de5c9eb0dd5e595d7e2d090ed8a3f82870d42c/LICENSE): original plugin requires Node/OpenClaw. Retained full Tencent license is tencent-LICENSE.txt beside this note.

HTTP and cryptographic transport can be implemented with Go. OpenClaw imports
implement plugin registration, routing, pairing, configuration/state storage,
agent dispatch and framework media operations. MiskoAI owns replacements for
these functions; the adapter never loads OpenClaw or Node. Source-derived code
carries Tencent attribution and the retained MIT notice. Service authorization
and independent-client acceptance still require live verification.

## Implemented boundary

`internal/channel/weixin` uses only Go standard libraries:

- `New(baseURL, token)` validates an HTTPS origin under the boundary-correct `.weixin.qq.com` suffix; default is `https://ilinkai.weixin.qq.com`.
- `StartQR(ctx)` POSTs to fixed `/ilink/bot/get_bot_qrcode?bot_type=3` with an empty local-token list and no bearer credential.
- `QRStatus(ctx, code, verificationCode)` GETs the fixed status endpoint. `QRStatusAt` allows caller-selected IDC continuation only through the same validated-origin/public-IP policy, without bearer forwarding. No automatic redirect is followed.
- QR statuses expose all pinned fields and support `wait`, `scaned`, `confirmed`, `expired`, `need_verifycode`, `verify_code_blocked`, `scaned_but_redirect`, `binded_redirect`. Confirmation requires issued token and bot ID; any supplied API origin is validated. Bot IDs and peer IDs remain opaque.
- `GetUpdates(ctx, cursor)` makes one authenticated bounded long poll and returns messages, opaque cursor and timeout hint. Server timeout hints do not expand the current 40-second engineering limit.
- `SendText(ctx, to, contextToken, clientID, text)` requires the authorized inbound context and a stable persisted outbox client ID; sends type 2/state 2 and a type-1 text item, with explicit empty from_user_id.
- Top-level `message_id` uses `json.Number`, preserving decimal uint64 values. Item `msg_id` follows the pinned string contract, retaining opaque strings and converting integer JSON literals losslessly to strings. Synthetic tests reproduce the old opaque-string rejection and verify preservation above JavaScript's safe integer range. This mismatch is not proven to be the original live failure's cause. Text and raw image/voice/file/video/reference fields are retained; this is not full group or media support.

Every call has caller cancellation, a deadline, bounded per-client request slots,
2MiB response cap and bounded input. Text is capped at 16KiB and opaque fields
at 16KiB. Send and bootstrap deadlines are 15 seconds; poll/status deadlines
are 40 seconds including queue wait. HTTP redirects and environment proxies
are disabled. The production dialer resolves allowed hosts, rejects a DNS
answer set containing restricted IPs, and connects directly to a validated IP
so DNS is not resolved a second time for that connection. TLS retains hostname
certificate verification and requires TLS 1.2 or newer. There is no logging.
IPv6 transition prefixes Teredo (`2001::/32`) and 6to4 (`2002::/16`) are
explicitly rejected because they can tunnel to embedded IPv4 destinations
despite belonging to the global IPv6 allocation. Independent synthetic
regressions first reproduced acceptance of both prefixes, then verified their
rejection.
Focused verification: `go test ./internal/channel/weixin -run
TestRejectIPv6TransitionDestinations -count=1` initially failed for both
literal transition addresses; after exclusion, `go test
./internal/channel/weixin -count=1` passed in 1.554s and `go vet
./internal/channel/weixin` completed without diagnostics (combined exit 0).

Error sentinels omit raw remote response text, URLs, tokens and messages.
Business -14 maps to ErrAuthExpired; HTTP 401/403 and 429 are separate. The
caller must stop account activity on expiry and require authorized re-login
or implement the reference cooldown. This adapter does not perform retries.
Transport failures, server 5xx, truncated/oversized/malformed successful send
responses and absent send acknowledgement produce ErrOutcomeUnknown. Do not
automatically resend these operations. A successful send requires explicit
ret=0, conservatively stricter than the reference's absent-ret handling.

## Integration and limitations

Persist authorized QR credentials in owner-only storage and initialize the
allowlist from the scanning user. Never log/export QRStatus wholesale: it
contains secret material. Do not render qrcode_img_content as an arbitrary
network URL; validate any later display/download mechanism separately.

The caller must atomically ingest the inbound batch and next cursor before
processing. Persist a claim keyed by account/channel/provider ID before
model/tools and store outbound send state/client ID before sending. The
reference persists its cursor before dispatch and contains no dedicated
provider-message dedup gate in the reviewed monitor/process/inbound path.
Server-side client_id idempotency and exactly-once delivery are undocumented.
The PoC does not store credentials/cursors, filter senders, implement durable
inbox/outbox, auto refresh QR, run a reconnect loop, invoke models, or process
group messages. Those are explicit later pipeline responsibilities.

One per-client poll slot and one per-client outbound/login slot permit a reply
while a long poll is active, while bounding the client to two requests. A
synthetic test first reproduced the single-slot problem with a send deadline
failure, then verified the separate bounded slots. No current end-to-end chat
latency claim is made.

ProtocolVersion 2.4.9 and numeric app client version match the pinned reference;
bot_agent truthfully declares MiskoAI/0.1.0. A service policy for independent
client versions is not established by the source and has not been tested.
The full user verification-code flow must remain intact; no authentication
bypass, fabricated identity or guessed endpoint is implemented.

Media wire evidence includes CDN uploads, plaintext MD5, AES-128-ECB/PKCS#7,
returned x-encrypted-param, and raw/hex AES key encodings. Media URLs require a
separate audited host/IP policy. No media download/upload, SILK decoder,
thumbnail, sticker, outbound voice or native group implementation is included.

## Verification evidence

Tests inject a synthetic RoundTripper at the private HTTP boundary; no test
contacts Tencent, resolves a production hostname or uses a real credential.
The initial `go test ./internal/channel/weixin` failed with undefined Client,
New, Message and error symbols before implementation. The first implemented
targeted suite passed on Windows: `ok .../internal/channel/weixin 4.409s`.

Commands use local caches because the global cache is outside sandbox writes:

```powershell
$env:GOPATH=Join-Path (Get-Location) '.tools\go'
$env:GOMODCACHE=Join-Path (Get-Location) '.tools\gomod'
$env:GOCACHE=Join-Path (Get-Location) '.tools\gocache'
go test ./internal/channel/weixin
```

Final scoped verification: `go test ./internal/channel/weixin -count=1` passed
in 0.158s, followed by `go vet ./internal/channel/weixin` with no diagnostics;
the combined command exited 0. Test-first follow-ups reproduced rejection
failures for NAT64 private-address synthesis and a send blocked by an active
poll; both pass after the bounded transport corrections. `gofmt -w
internal/channel/weixin` ran successfully.

An initial `go test ./...` passed CLI, WeChat, netx, provider and storage.
A final integrated run during concurrent primary implementation passed WeChat
(2.290s), netx, provider and storage (9.126s), but reported `internal/config` undefined `Load`
from new tests and `internal/cli` import failures for os/path/filepath with
an empty cache path. These failures are outside this agent's file ownership;
the primary must rerun integrated checks once concurrent work stabilizes.

Real QR login confirmed. Authorized text receive failed before send; a subsequent read-only diagnostic initiated before deferral decoded an empty batch. Further real WeChat testing is paused by the owner. Code-verification terminal restoration, receive/reply, context expiry, reconnect/replay, media and512MB RSS acceptance remain unverified. Native Linux CI run37822881696 passed configured synthetic tests/race/builds; fixture success is not live channel acceptance.
