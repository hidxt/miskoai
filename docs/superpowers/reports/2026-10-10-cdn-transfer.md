# CDN Task2 author report

2026-10-10. Author: GPT-6.1 Sol Medium (`gpt-6.1-sol`, reasoning effort `medium`). Engineering progress remains **56%** under the root ledger. This is local synthetic implementation evidence only. The latest root-reported public UI checkpoint `dab6c53ecccbfaeddfc1d7c4fa0f2388dec24ae8` / nativeCI38042004429 passed its configured steps and excludes this working CDN patch. Root integration, independent review, native CI for this patch, live CDN/media and final acceptance remain separate gates.

## Scope and ownership

Created only `internal/media/cdn.go`, `cdn_url.go`, `cdn_test.go`, `cdn_url_test.go` and this report. No dependencies, shared codec/netx/privatefs files, channel/API/getuploadurl/send, database, provider, parser, image/GIF, UI or shared ledgers changed. No delegation, Git mutations, live network, private credentials or actual media were used. The status-only Git command encountered an existing global-ignore permission warning; no Git configuration was changed.

Read AGENTS, REQUIREMENTS, SECURITY, ARCHITECTURE, TASKS, MEMORY, DELIVERY, the private CDN brief, approved transfer plan, attachment design and pinned Weixin media research. Applied runtime TDD and verification-before-completion skills. Dependency contracts were sufficient without amendments. Tencent source details are attributed in the new source to commit `24de5c9eb0dd5e595d7e2d090ed8a3f82870d42c`; existing complete Tencent MIT notice in `internal/notices/notices.txt` and `docs/research/tencent-LICENSE.txt` remains intact.

## Implemented contract

- `NewCDN` reuses `netx.NewPublicTransport(30*time.Second)`, public-answer-set DNS pinning/TLS controls and bounded headers. The CDN client has no cookie jar or redirect following. CDN-only HTTP1 and disabled keep-alives prevent implicit stdlib request replay; shared netx settings are untouched.
- One actual transfer admission covers wait, metadata validation, request, response reading, codec/private-file work, checked response close and borrowed upload-reader close. One fixed30s context includes all those stages. No detached timeout/Reader goroutine, configurable timeout override, retry or upload replay source exists in production.
- `Close` publishes refusal, cancels admitted work and waits for actual completion. Concurrent Close callers join the same completion. Cooperative cancellation does not free admission before a blocked read/body-close actually returns. A noncooperative Reader can hold Close; callback-initiated Close is unsupported and documented.
- Preferred full URLs receive strict validation without fallback on error. Exact HTTPS host, absent/443 port, literal route, UTF8/control/whitespace guards, query size/key/value/count/duplicate guards and unchanged accepted query bytes enforce the plan. Upload always requires32 lowercase hexadecimal filekey characters, including with a full URL.
- Download requires HTTP200 and identity/absent content encoding. Declared excess is refused before codec work; actual known/unknown/dishonest streams remain bounded by Decrypt. A response-close failure or late cancellation removes a completed private artifact before return. Padding/truncation/read failures never publish partial plaintext.
- Upload borrows a completed16-aligned bounded Artifact through a SectionReader with exact ContentLength, fixed Content-Type and no GetBody. It never closes/removes the caller Artifact. A synchronized non-owning request-body wrapper joins active reads even when a RoundTripper returns before asynchronously closing its request body. Cancellation during that final join still clears success. HTTP200, exactly one nonempty bounded UTF8/control-free encrypted-parameter header, a complete bounded64KiB response drain and checked response close are required.
- Returned failures are fixed safe media codes or standard cooperative context errors. No URL/query/key/body/private path or raw remote/reader error is returned or logged. Only successful Upload returns the bounded opaque protocol parameter to its caller.

## Installed standard-library replay review

Read the verified installed Go1.27.2 `src/net/http/transport.go`, not an internet approximation. `protocols()` lines476–503 shows that explicit `Transport.Protocols` wins over default negotiation; the existing custom DialContext/TLSClientConfig constructor already defaults conservatively to HTTP1, but this CDN explicitly sets only HTTP1 to preserve the contract against default changes. `roundTrip` lines721–745 chooses the HTTP2 alternate path and has a NoCachedConn retry branch. `shouldRetryRequest` lines844–889 checks HTTP2 NoCachedConn before its fresh-connection `!pc.isReused()` refusal. `tryPutIdleConn` lines1129–1140 returns before marking reuse when DisableKeepAlives is true. Therefore the CDN's explicit HTTP1-only protocol set excludes the earlier HTTP2 branch and disabled keep-alives prevents the reused HTTP1 retry precondition. Root approved this routine CDN-owned configuration; shared transport is unchanged. Constructor tests assert this policy and mock one-attempt tests assert application behavior; no mock is claimed to prove native TLS/CDN behavior.

## Runtime RED evidence

All test commands below use the verified native Go executable and offline environment listed below. No compile/setup failure is counted as runtime RED.

1. `go test -json ./internal/media -run 'TestCDN' -count=1`, exit1, `.tools/reviews/cdn-transfer-red.json`: interface-only stubs compiled;25 named failures,0 named passes,0 skips. Six top-level failures: TestCDNNoCredentialForwarding, TestCDNActualDownloadBoundsAndPadding (eight child cases), TestCDNUploadExactlyOnce (eleven child cases), TestCDNAdmissionCancellationJoinsBody, TestCDNErrorCanariesAndClose, TestCDNURLPrecedenceAndRefusal. Failures show missing requests/completed artifacts/actual-reader lifetimes/preferred URL behavior. Fixture wait timeouts here identify missing actual Reader entry; upload fixture creation itself succeeded.
2. `go test -json ./internal/media -run '^TestCDNTransportNoReplayPolicy$' -count=1`, exit1, `.tools/reviews/cdn-transfer-protocol-red.json`: compiled runtime refusal because explicit HTTP1-only policy was missing. Then explicit Protocols was added.
3. `go test -json ./internal/media -run '^TestCDNUploadBorrowCloseJoinsReader$' -count=1`, exit1, `.tools/reviews/cdn-transfer-borrow-red.json`: interface-only non-owning wrapper returned Close before an actual held Read ended. Then read/close synchronization was implemented and wired into Upload.
4. `go test -json ./internal/media -run '^TestCDNUploadCancellationDuringBorrowClose$' -count=1`, exit1, `.tools/reviews/cdn-transfer-borrow-cancel-red.json`: response close had completed, a borrowed request read remained held, and cancellation during the final borrow-close join incorrectly returned nil. Then context was checked after the actual borrowed-read join. An earlier version of this test passed because it canceled before response close; that test-ordering observation is not reported as RED. The retained RED uses the intended later boundary.

First transfer GREEN: `.tools/reviews/cdn-transfer-green-first.json`, exit0. First assigned full scoped GREEN: `.tools/reviews/cdn-transfer-green.json`, exit0. Final result below supersedes intermediate passing snapshots.

## Final commands and results

Executed with PowerShell from `C:/Users/auzasr/Documents/Projects/miskoai`:

```powershell
$env:GOTOOLCHAIN='local'
$env:GOPATH='C:/Users/auzasr/Documents/Projects/miskoai/.tools/go'
$env:GOMODCACHE='C:/Users/auzasr/Documents/Projects/miskoai/.tools/gomod'
$env:GOCACHE='C:/Users/auzasr/Documents/Projects/miskoai/.tools/gocache'
$env:GOTMPDIR='C:/Users/auzasr/Documents/Projects/miskoai/.tools/tmp'
$env:TEMP=$env:GOTMPDIR
$env:TMP=$env:GOTMPDIR
$env:GOPROXY='off'
$env:GOSUMDB='off'
& .tools/toolchains/go1.27.2-verified/go/bin/gofmt.exe -w internal/media/cdn.go internal/media/cdn_url.go internal/media/cdn_test.go internal/media/cdn_url_test.go
& .tools/toolchains/go1.27.2-verified/go/bin/go.exe test -json ./internal/media ./internal/netx ./internal/privatefs -count=1 | Out-File -Encoding utf8 .tools/reviews/cdn-transfer-green-final.json
exit $LASTEXITCODE
```

Final scoped test exit **0**: **126 named passes / 0 failures / 2 existing platform skips**. Counts include named parent tests and child cases, exclude package completion events: media83, netx26, privatefs17. All3 packages passed. Windows skip `TestPrivateDirNeverRepairsExisting` explicitly defers to the broad-DACL Windows test; skip `TestPrivateSymlinkTraversal` reports missing host symlink privilege. No test/ACL was weakened or repaired; no escalation was needed for these synthetic fixture runs. Temporary private fixtures were freshly created under dedicated `.tools/tmp` test directories, never actual databases/private media.

Using the same environment:

```powershell
& .tools/toolchains/go1.27.2-verified/go/bin/go.exe vet ./internal/media ./internal/netx ./internal/privatefs *> .tools/reviews/cdn-transfer-vet.txt
exit $LASTEXITCODE
```

Scoped vet exit **0**, empty diagnostics. Root owns full integrated tests/scanners/review/native race/builds after freeze; these were not duplicated by the child.

All39 named CDN test/child passes are retained in final JSON. Top-level coverage:

- `TestCDNURLPrecedenceAndRefusal`, `TestCDNURLMetadataBounds`: invalid preferred origin/port/userinfo/path/encoding/fragment/duplicate/malformed/oversized query, invalid filekeys and fallback metadata; accepted signed query bytes/escaping/explicit443 preserved.
- `TestCDNNoCredentialForwarding`, `TestCDNTransportNoReplayPolicy`, `TestCDNRedirectAndPreCancelledNeverReplayed`: GET headers/deadline, no credentials/proxy/cookies/redirect/replay policy, redirect one attempt and pre-canceled/nil context no dispatch.
- `TestCDNActualDownloadBoundsAndPadding`: unknown valid, maximum4MiB valid, declared/unknown/dishonest excess, truncation, padding, compressed response and checked-close failure; actual body closure and owned-file cleanup asserted.
- `TestCDNDownloadReadAndTransportErrorCanaries`: read and ambiguous transport errors hide synthetic URL/query/key/body/path canaries and attempt once.
- `TestCDNUploadExactlyOnce`: success, exact64KiB drain,429/503/uncertainty, missing/empty/duplicate/oversized/control/invalidUTF8 header, read/close failure and response excess; POST bytes/length/fixed header/no GetBody/one attempt checked, borrowed body becomes unreadable while caller Artifact remains readable/present.
- `TestCDNInvalidUploadNeverReachesTransport`: nil/zero/nonaligned/oversized Artifact and forbidden preferred upload endpoint never reach transport.
- `TestCDNAdmissionCancellationJoinsBody`, `TestCDNAdmissionAndCloseJoinCheckedBodyClose`: cancellation-aware held read and held checked response Close retain the slot; canceled waiters make no request; cancellation and Close join actual work before cleanup.
- `TestCDNUploadBorrowCloseJoinsReader`, `TestCDNUploadCancellationDuringBorrowClose`: borrowed read must actually finish before request close/admission completion; late cancellation still refuses success.
- `TestCDNErrorCanariesAndClose`: Close cancels/joins admitted work, refuses new requests, repeated Close succeeds and cleanup leaves no spools.

## Frozen source

The four Go files are frozen after the final scoped test/vet runs. SHA256:

| File | SHA256 |
|---|---|
| internal/media/cdn.go | ea803f6742fdf9eb66f89ad757de1ed949e452ee8c415e51d06bb98a71e7616b |
| internal/media/cdn_url.go | df867e9d59faab7f6c7e44d6468ffd65d9f518f598c2cd9db110e752f7b0d5b2 |
| internal/media/cdn_test.go | bb6b56bb89bb8ad68f647bb2f8874b8d1cd8e65b4dc393d3cde2830e777e337a |
| internal/media/cdn_url_test.go | ec29e95d1e5bb5757479e18328e06d9b081763103fa62cf9f95b39750f505429 |

No unresolved dependency gap or author-known defect is reported. Independent review and root acceptance are pending. Only completed encrypted transport is implemented: receive authorization/claims, metadata digest/length comparisons, image/GIF/sticker/media-format validation, provider vision, documents/library and channel send integration remain downstream. AES ECB/PKCS7 is unauthenticated wire compatibility. Conservative metadata bounds may refuse larger real server values and must not be silently relaxed. No live CDN, provider, real WeChat/media/sticker, native Linux/arm64 execution,512MB/RSS or final product acceptance claim is made by this report.

## FIX1 — review findings and replacement freeze

2026-10-10, same GPT-6.1 Sol Medium author. Engineering progress remains **56%**. The prior author-known-defect statement and source freeze above describe the initial submission only. Fresh review returned **Spec/Quality ChangesRequired**, with important I1 (early successful response could hide incomplete/failed upload request reads) and minor M1 (held-fixture fatal cleanup could block before release). Root independently reproduced I1 at runtime despite the prior passing full baseline. This FIX1 replaces the source freeze; same-reviewer/root acceptance is still pending.

Authorized changes are limited to `cdn.go`, `cdn_test.go` and this report. `cdn_url.go` and `cdn_url_test.go` remain byte-exact at their prior hashes. No image WIP, dependency, shared ledger, immutable review export, root diagnostic probe, Git, credential, live network or private data was changed or accessed.

I1: the non-owning upload request wrapper now retains actual consumed bytes and the first nonEOF read failure under its existing read/close mutex. The final completion guard joins any active read, forbids subsequent reads and refuses success unless exactly Artifact.Size bytes were consumed without read failure. A simultaneous full-byte read plus nonEOF error still fails. Exact-byte consumption without an extra EOF read succeeds. Cancellation remains highest precedence; existing HTTP/transport/response-body errors retain precedence over the completion refusal. Every refusal clears the opaque parameter. No retry/replay or Artifact ownership change was introduced. Download cleanup now checks its Artifact.Close error safely, and Upload checks completion rather than discarding borrowed Close status; this narrowly addresses the two reported new LOW unchecked-error sites without suppression or global scans.

M1: every held-read/held-response-close fixture now registers an idempotent `sync.Once` release before awaits/assertions, after client cleanup registration so LIFO cleanup releases before blocking client Close. Normal paths use that same release. Fixtures requiring context cancellation also have an independent parent-cancel cleanup. Private temporary directory cleanup is registered before client/release cleanup. The standalone held-reader test joins its read in cleanup, and final error-channel waits are bounded. Actual read/body-close/admission/Close join assertions remain intact. A new subtest returns with an active held body and proves registered cleanup releases/cancels/joins the operation instead of depending on manual release.

All FIX1 commands ran from root's writable dedicated test copy:
`C:/Users/auzasr/Documents/Projects/miskoai/.tools/reviews/cdn-transfer-fix-work-src`.
It excludes held image WIP and retains the root's complete initial frozen public source. Before each RED/GREEN/final/vet invocation, the three authorized main files were copied there using literal Copy-Item paths and SHA256 equality checked, throwing on mismatch. The two immutable URL files were also checked equal before final tests. No immutable export or root probe was mutated.

Commands used the same absolute verified Go/offline cache/tmp environment as the initial report, with log paths pointing to the main root `.tools/reviews`:

```powershell
& "$root/.tools/toolchains/go1.27.2-verified/go/bin/go.exe" test -json ./internal/media -run '^TestCDNUploadEarly' -count=1 |
  Out-File -Encoding utf8 "$root/.tools/reviews/cdn-transfer-fix1-red.json"
exit $LASTEXITCODE

& "$root/.tools/toolchains/go1.27.2-verified/go/bin/go.exe" test -json ./internal/media -run '^TestCDN(UploadEarly|HeldFixture)' -count=1 |
  Out-File -Encoding utf8 "$root/.tools/reviews/cdn-transfer-fix1-green.json"
exit $LASTEXITCODE

& "$root/.tools/toolchains/go1.27.2-verified/go/bin/go.exe" test -json ./internal/media ./internal/netx ./internal/privatefs -count=1 |
  Out-File -Encoding utf8 "$root/.tools/reviews/cdn-transfer-fix1-final.json"
exit $LASTEXITCODE

& "$root/.tools/toolchains/go1.27.2-verified/go/bin/go.exe" vet ./internal/media ./internal/netx ./internal/privatefs *> "$root/.tools/reviews/cdn-transfer-fix1-vet.txt"
exit $LASTEXITCODE
```

Here `$root` is explicitly `C:/Users/auzasr/Documents/Projects/miskoai`; cwd is the dedicated fix-work-src, not the main source tree. The shell tool executed each command separately and reported its actual exit.

- Owned runtime RED: **exit1, 7 named failures / 1 named pass / 0 skips**. No compile/setup errors. `TestCDNUploadEarlySuccessRefusesIncompleteRequest` failed in unconsumed, partial, readerror, closed-artifact and truncated-artifact cases; exact-complete passed. Parent failure plus `TestCDNUploadEarlyResponseJoinsHeldReadFailure` account for the seven failures. A valid200/header/complete response was insufficient to prove upload request completion.
- Focused GREEN: **exit0, 10 named passes / 0 failures / 0 skips**, including the new cleanup parent/subtest. This ran after the completion guard and cleanup release implementation; subsequent final tests include final cleanup refinements.
- Final assigned scope: **exit0, 136 named passes / 0 failures / 2 existing Windows platform skips**. Media93, netx26, privatefs17;49 named CDN passes including parent/child cases. Skips remain exactly TestPrivateDirNeverRepairsExisting and TestPrivateSymlinkTraversal, for the same Windows reasons above. No ACL weakening/repair/escalation was used.
- Final scoped vet: **exit0**, empty diagnostics (`cdn-transfer-fix1-vet.txt` length0).

Covering tests: `TestCDNUploadEarlySuccessRefusesIncompleteRequest/{unconsumed,partial,readerror,closed-artifact,truncated-artifact,exact-complete}`, `TestCDNUploadEarlyResponseJoinsHeldReadFailure`, `TestCDNHeldFixtureCleanupReleasesBeforeClientClose/cleanup`. Existing exact-once/header/body/error/caller-ownership tests, late cancellation during borrowed-read join, admission wait/body-close joins, no-replay protocol policy and unchanged URL guards also passed. The held late-read-failure case returns all16 bytes plus a synthetic nonEOF error after response close; admission remains occupied until the actual read ends, and no parameter escapes.

Replacement frozen Go-file SHA256, checked equal between main and dedicated test copy after final tests/vet:

| File | SHA256 |
|---|---|
| internal/media/cdn.go | c04e0a37c29a106014cc25c20d5beb566231d9410cdb4fa73fad262ecacbe352 |
| internal/media/cdn_test.go | 9f10272d0fc8510cb759fbc2be660b0b195fb23d6711faabaa2a8bec0d5c8887 |
| internal/media/cdn_url.go (unchanged) | df867e9d59faab7f6c7e44d6468ffd65d9f518f598c2cd9db110e752f7b0d5b2 |
| internal/media/cdn_url_test.go (unchanged) | ec29e95d1e5bb5757479e18328e06d9b081763103fa62cf9f95b39750f505429 |

The updated report is mirrored and hashed separately in the returned five-file manifest to avoid a self-referential report hash. Runtime RED source-copy equality was checked by the executed command; a separate RED source hash manifest was not retained. Final hashes above identify the actually tested replacement source. No new dependency gap is known. Same-reviewer approval, root integration/full scanners/native CI remain pending; there is no live CDN/media/WeChat, native Linux/arm64 execution,512MB/RSS or product acceptance credit from FIX1.
