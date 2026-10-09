# Bounded CDN transport Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans task by task. Root dispatch is required after the private media codec gate. All steps use synthetic inputs; no live transfer is authorized by this plan.

**Goal:** Download encrypted attachments into completed private artifacts and upload a completed encrypted artifact through the source-proven CDN routes without credential forwarding or automatic retries.

**Architecture:** Reuse one public-IP-pinned HTTP transport constructor from netx. A separate media CDN client owns one actual transfer admission and its full response/decryption lifetime. API getuploadurl, durable receive claims, image validation and chat sending remain downstream gates.

**Tech Stack:** Go1.27.2 stdlib net/http/net/url/io and reviewed netx/privatefs/media codec; no new dependency/runtime.

**Spec:** docs/superpowers/specs/2026-10-09-attachments-design.md; pinned primary-source evidence docs/research/weixin-media-design-evidence.md. Root inspected cached cdn-url.ts, cdn-upload.ts and pic-decrypt.ts at Tencent commit24de5c9eb0dd5e595d7e2d090ed8a3f82870d42c. Preserve Tencent MIT attribution for adapted protocol details.

## Global Constraints

- Child model only GPT-6.1 Sol Medium (gpt-6.1-sol, reasoning_effort medium); no delegation/private data/live network/Git/global configuration changes.
- One native Go/SQLite process, Linux amd64/arm64,512MB target remains unverified. No whole media buffers or detached timeout workers.
- Plaintext4MiB, ciphertext4MiB+16. Source-proven AES128ECB/PKCS7 is compatibility, not authentication. Only completed codec artifacts are exposed.
- Exact HTTPS novac2c.cdn.weixin.qq.com port443, /c2c/download or /c2c/upload. Reject userinfo, fragment, opaque URL, unsafe path/encoding, private DNS answers, redirects, environment proxies and unexpected Content-Encoding.
- No bot bearer, cookie, chat context, API metadata or arbitrary caller headers on CDN. No URLs/query/key/body/error payload in returned errors or logs.
- One30s context covers admission, request, body consumption/decryption and close. Cooperative custom readers must actually end before admission release. No retries, including HTTP429/503 and possible upload submission.

## Review Focus

- Returned full URL with a forbidden origin/path is refused before any request; no fallback hides an invalid preferred URL.
- A mixed public/private DNS answer set is refused, and the validated public address is used directly without a second lookup.
- Unknown-length, excessive, truncated, compressed or invalidly padded download never publishes partial plaintext; owned files and actual response body are closed.
- Upload is attempted once, keeps artifact ownership with its caller, and cannot report success from missing/duplicate/oversized encrypted-parameter headers or an unfinished failing response body.
- Cancellation while an actual body/decryption is held does not free the one-transfer slot early; secret canaries remain absent from errors and request headers.

## Task1: Shared pinned public transport constructor — Medium

**Files:** modify internal/netx/client.go; create internal/netx/transport.go and internal/netx/transport_test.go; report docs/superpowers/reports/2026-10-10-cdn-transport.md. No media/channel/provider edits.

**Interfaces:**
- `NewPublicTransport(timeout time.Duration)(*http.Transport,error)` returns a transport with nil Proxy, reviewed safeDial, TLS minimum1.2, TLS handshake10s, response headers timeout, idle60s, max idle4/per-host2, max connections per-host2, compression disabled, max response headers32KiB. Accept timeout>0 and<=2min. It neither authorizes URLs nor supplies headers, retries, response bounds or operation admission; each caller owns those controls.
- Existing `New(base,allowed,timeout,max)` uses this constructor while retaining all current base/max validation, the exact two-slot client admission and its no-redirect http.Client. Existing SetTransport semantics remain application-owned/test-only and configure-before-use.
- Production safeDial retains full answer-set validation, pinned direct dialing and fixed safe errors. Narrow private per-call resolver/dial seams may exercise mixed answers and exact dial identity; no global mutable hook or exported arbitrary dial injection.

- [ ] Runtime RED TestSharedTransportPreservesCloudContract (constructor settings/old client constraints), TestPinnedDialRejectsMixedAnswers (no dial), TestPinnedDialUsesValidatedAddress (captured IP/port, no second resolution). Use synthetic resolver/dialer, not a remote endpoint.
- [ ] Implement factoring without changing existing retry/JSON/SSE behavior; ensure every connection still enters the reviewed public-IP guard. No duplicated independent DNS implementation in CDN.
- [ ] Run `go test -json ./internal/netx ./internal/provider -count=1` and scoped vet with verified Go/offline caches; require exit0, record named pass/fail/skips. Freeze for fresh Medium review/root tests; root alone commits.

## Task2: Single-admission completed CDN transfers — Medium

**Files:** create internal/media/{cdn.go,cdn_url.go,cdn_test.go,cdn_url_test.go}; report docs/superpowers/reports/2026-10-10-cdn-transfer.md. Consume codec without editing it and Task1 netx constructor without edits. If either contract needs amendment, escalate to root before modifying owned dependencies.

**Interfaces:**
- `NewCDN()(*CDN,error)` constructs the shared reviewed transport plus no-redirect http.Client, no cookie jar, one transfer semaphore, no requests. Private per-instance roundtripper seam supports offline tests; no model/HTTP-controlled transport.
- `Download(ctx context.Context,dataDir,encryptedQuery,fullURL string,key [16]byte)(*Artifact,error)`: if fullURL is nonempty, strictly validate that exact preferred URL; otherwise construct documented /c2c/download with url.Values encrypted_query_param. GET has no credentials. Require HTTP200, absent/identity Content-Encoding and declared content length<=4MiB+16 if known; actual codec stream still bounds unknown/dishonest length. Feed the live response reader into Decrypt, retain admission until full codec return and checked response close. On post-decrypt response close failure, close/clean returned artifact and return safe refusal.
- `Upload(ctx context.Context,ciphertext *Artifact,uploadParam,fullURL,filekey string)(string,error)`: use validated preferred full URL or documented /c2c/upload with encoded encrypted_query_param and filekey. Filekey exactly32 lowercase hex, even when full URL is provided. Artifact size positive,16-aligned and<=4MiB+16; read via io.NewSectionReader with declared exact ContentLength, no whole copy and no artifact ownership transfer. POST Content-Type application/octet-stream; no GetBody/replay source. Require HTTP200 and exactly one x-encrypted-param field, nonempty<=4KiB validUTF8/control-free. Drain response through a bounded64KiB reader and check read/close before success. Return opaque parameter only to later protocol code, never status/UI. Call caller-owned Artifact.Close separately after actual upload ends.
- `Close()error` closes idle connections; define and test refusal of new operations plus cancellation/join of any admitted operation before completion. Calling Close inside a transfer callback is unsupported. No fake deadline join or successful early admission release.
- Fixed safe invalid/endpoint/limit/HTTP/transport codes; preserve cooperative context errors. Return no raw URL/http/reader errors. No automatic retry branch anywhere.

**URL admission values:** raw full URL<=16KiB, validUTF8/no controls or whitespace, exact HTTPS hostname and absent/443 port; no userinfo/fragment/opaque/ForceQuery. Exact literal allowed path, no RawPath or percent-encoded path aliases. Require nonempty query<=12KiB; use url.ParseQuery to reject malformed escape/semicolon, at most16 unique keys, exactly one value per key, each decoded key1..64bytes/value<=4KiB and no controls. Preserve accepted full URL query bytes rather than reinterpreting unknown signed query semantics. Fallback encrypted/upload opaque parameter1..4KiB, UTF8/no controls, constructed URL passes the same policy. FullURL precedence never turns unsafe preferred input into fallback. These conservative metadata defaults may refuse larger server values; do not silently relax them from a mock.

- [ ] Runtime RED TestCDNURLPrecedenceAndRefusal (userinfo/evil host/port/path/escaped alias/dupe/malformed query, no request), TestCDNNoCredentialForwarding (fixed headers/proxy/redirect/encoding), TestCDNActualDownloadBoundsAndPadding (known/unknown length, max+1, truncation, response close failure, no partial artifact).
- [ ] Runtime RED TestCDNUploadExactlyOnce (HTTP429/503/transport ambiguity/missing duplicate header/body read/close failure yield one attempt, caller artifact survives), TestCDNAdmissionCancellationJoinsBody (held cancellation-aware reader, second admission blocked until first really returns), TestCDNErrorCanariesAndClose (URL/query/body/key absent, Close joins and refuses new work).
- [ ] Implement focused stdlib client, no API getuploadurl/model/send/DB/UI behavior. Check cancellation before acquisition and use context across actual body work; no goroutine around Reader to simulate timeout.
- [ ] Run `go test -json ./internal/media ./internal/netx ./internal/privatefs -count=1` and scoped vet; require exit0 and exact results. Freeze for fresh independent Medium protocol/resource/security review and root integrated/native race/scans.

## Root self-review and boundaries

Task1 produces exactly the constructor Task2 consumes; media codec Artifact/Decrypt contract is from the separate codec plan. Each of five Review Focus conditions maps to named fixtures above. These two tasks cover only CDN transport from the attachments design; authorized receive claims, getuploadurl, metadata digest/length comparison, image/GIF validation, shared vision, documents, library and one strict-ACK media send are separate gates. No real CDN upload/download, ordinary image/GIF/native sticker,512MB RSS or full product acceptance is inferred. Source response-header success is not chat-delivery success. Root maintains sequential file ownership, explicit frozen review packages and final native/live gates.
