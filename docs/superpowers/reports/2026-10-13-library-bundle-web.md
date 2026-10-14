# Complete library bundle HTTP contract groundwork

2026-10-13 Task11a, ONLY gpt-6.1-sol / reasoning_effort medium. Engineering 74% (37/50) remains unchanged. Implementation and local synthetic HTTP tests only; no package39, actual full Core bridge, native CI, live/provider/WeChat/server/resource or final acceptance credit.

## Source and ownership

Exact immutable installer basis `.tools/reviews/library-bundle-install-root-reviewed-src`: all 419 source lengths and SHA256 identities verified against `library-bundle-install-root-source-manifest.json` before copy. Work directory `.tools/reviews/library-bundle-web-oct13-http-src`. Root extended MAIN Web plan ownership to a fifth file, server.go, solely for exact two-route inclusion in the existing 30-second maintenance branch. Original four-file/RED evidence preserved independently. No mutable Task10 source read or copied.

Five owned paths: internal/web/application.go, internal/web/library_bundle.go, internal/web/library_bundle_test.go, internal/web/server.go, this report. The other 417 original files remain exact basis bytes, including existing system/library/auth/tests/Core/CLI/assets/modules/LICENSE/notices/ledgers. No Git staging/commit/push or main source modification.

## Behavior and interfaces

Two fixed authenticated POST routes with no accepted query keys. Optional `libraryBundleBackend` provides `prepareLibraryBundle(context.Context)(managementDownload,error)` and `RestoreLibraryBundle(context.Context,io.Reader)(maintenance.RestoreResult,error)`. Existing management backends remain compatible and return503 without this capability. No actual coreBackend bridge was added; root integrates it only after Task10 acceptance. Groundwork must not be published/exposed as operational bundle support before that integration.

Backup accepts bounded exact empty JSON object, refusing any Content-Encoding and malformed/unknown/trailing input before backend effects. Prepared metadata must name `miskoai-library.bundle` and declare positive size <=321MiB before binary headers. Only bounded4096-byte scratch is used, actual context is checked around every read and before write. The download is closed exactly once synchronously, including metadata refusal and errors. After headers, any read/write/short-write/cancel/Close failure causes `http.ErrAbortHandler`; JSON is never appended. Before-header cancellation plus a failed download Close retains the established download-policy500 safe error rather than claiming successful cleanup.

Restore requires exactly one literal `application/octet-stream` header with no parameters, no Content-Encoding, nonnil body, positive known declared length <=321MiB. MaxBytesReader and existing post-read context wrapper are combined with declared-length tracking, a one-byte EOF probe, sticky refusal errors, and an EOF-consumption requirement before success. No request filenames, paths, base64 or whole-bundle buffering. Trusted backend must consume strict EOF during pre-install validation; HTTP does not drain after a potentially mutating backend return. Successful flags use the actual typed PreviousRetained and ChannelPaused booleans. No retry or channel resume.

Safe error responses reuse library classes, with context/cleanup precedence before maintenance classification. No raw private error strings are returned. Root's server ownership extension adds only the two exact routes to the existing30s operation deadline branch; all other10s route classes and socket/header/auth/admission/recovery remain unchanged.

## TDD and preserved failures

Initial RED and green1 runs were compiled but failed auth403: a test prerequisite GET /api/session rotated CSRF while the helper retained the old token. They earn no semantic route RED credit. Both logs and original test/application source are retained. Helper now parses the actual refreshed CSRF response before POST. Implementation was removed, original application restored, and corrected RED2 then failed real fixedbackup/fixedrestore semantics with404 after authenticated prerequisites succeeded (four named failed events, exit1).

A subsequent green2/deadline-red run revealed omitted dispatch caused by a newline-sensitive edit; compiled failures and source remain preserved. Corrected dispatch green3 showed the intended deadline RED (actual backend10s) and canceled-read RED (io.CopyN wrote final n>0 bytes despite the cancellation error). Fixed only the two-route deadline branch and bundle copy loop. Green4:75 named/distinctPASS,0FAIL,0SKIP, exit0.

A separate safe-error precedence RED confirmed a joined maintenance+cleanup error was incorrectly400; preserved test/production source and log, then routed cleanup/context through existing library classification before maintenance. This RED recorded1PASS/2FAIL/0SKIP, exit1; standalone canceled+maintenance already used the existing timeout classification.

## Local verification

Native Windows verified Go1.27.2; offline `GOTOOLCHAIN=local`, `GOPROXY=off`, `GOSUMDB=off`, root `.tools/go`, `.tools/gomod`, `.tools/gocache`, `.tools/tmp` caches. MISKOAI_* environment names cleared without values. Test fixtures contain synthetic-only bytes and refused dependencies. No credentials/private database/network/provider/CDN/paid/real WeChat/server effects, and no real Core.Run. Existing synthetic Core fixtures in unchanged Web regressions are distinct from actual bundle backend integration.

Commands from isolated source:

- `go test -count=1 -json -timeout=45s -run '^TestLibraryBundleHTTPFixed' ./internal/web` -> corrected semantic RED2 exit1.
- `go test -count=1 -json -timeout=45s -run '^TestLibraryBundleHTTP' ./internal/web` -> green4 exit0,75 named/distinctPASS,0FAIL,0SKIP.
- `go test -count=1 -json -timeout=45s -run '^TestLibraryBundleHTTPErrorPrecedence' ./internal/web` -> precedence RED exit1,1PASS/2FAIL/0SKIP.
- `go test -count=1 -json -timeout=90s ./internal/web` -> original final suite exit0,227 named/distinctPASS,0FAIL,0SKIP. Adds complete unchanged Web/auth/library/SQLite-only regressions.
- Final2 same complete Web command adds backup preparation lifetime cases -> exit0,233 named/distinctPASS (92 bundle events),0FAIL,0SKIP, one packagePASS,29.631s package elapsed, empty stderr.
- `go vet ./internal/web` -> both final and final2 exits0, empty stdout/stderr; final2 checked the exact final Go source and tests.

Tests exercise real Server.Handler login/session/auth/CSRF/origin/Host gates; fixed method/query/body admission; exact321MiB upper-bound metadata/declared-length admission with bounded short readers (no full321MiB artifact claim); false/true restore flags; unsupported backends; binary headers; exact close counts; synchronous held actual reads/Close; post-read cancellation; actual loopback HTTP truncated transfer; truncation/overflow/unconsumed EOF/sticky ignored read refusal; safe errors, deadline observation, preparation/nil artifact lifetime and existing regressions. Actual transfer failures are deterministic mock boundaries, not candidate installation proofs.

## Freeze, self-review and limits

Final complete source snapshot and five owned source/report identities, unified diff and all actual output/exit/RED source evidence are frozen under `.tools/reviews/library-bundle-web-oct13-*`. Manifest lengths/SHA256 allow root independent verification. Self-review checked optional-backend isolation, only fixed dispatch, exact raw-body admission, context around real reads, pre-install EOF obligation, Close joins, abort recovery, safe error precedence, scope/resource caps and unchanged legacy routes. No unresolved author finding; root fresh review and combined actual Core/browser/static/native checks remain required.

Readers and Close must cooperate: no detached goroutines or hard real-time/RSS guarantee. Actual codec/database/content installation is outside these optional-backend tests. Full bundle success, all-scope roundtrip, durable recovery, real provider/channel, actual Linux/arm64/systemd/server/512MB and final acceptance remain unverified here. Work quiesces after freeze; no further source mutation without root handoff.

## FIX1 — final actual Read errors, 2026-10-14

Root's bounded independent diagnostic supersedes the earlier self-review's no-finding conclusion for this ordinary transfer correctness gap. Original422-file source, five owned identities,36870-byte patch/all52 evidence artifacts,233PASS and fresh143PASS/zero-findings records remain historical evidence and are unchanged. Root `rootLastReadError` returns all four declared bytes together with io.ErrUnexpectedEOF; io.ReadFull intentionally drops that non-EOF error when its buffer fills. The old copy therefore wrote4bytes and returnednil. Normal final n+io.EOF is the valid control. This is an incomplete transfer contract, not a security escape finding.

Only original gpt-6.1-sol / medium implements FIX1; engineering74% (37/50) unchanged. New `.tools/reviews/library-bundle-web-fix1-oct14-src` copied all422 original lengths/SHA256 after complete verification. Only library_bundle.go, library_bundle_test.go and this report change. Original application.go/server.go plus other417 files stay byte-identical. No mutable Core source, main/Git/private/network/provider/CDN/paid/WeChat/server action. Actual Core bridge remains separately root-owned/dependent.

The copy loop now inspects each actual input.Read n/error and actual context before exposing bytes, with the same4096 scratch and declared finite length. Every non-EOF read error aborts before writing its returned bytes. Final n+EOF succeeds only when those bytes finish the exact declared size; early EOF safely aborts. Invalid negative/oversized n and zero-progress0,nil readers refuse synchronously; valid short positive reads remain supported. Afterheaders failures still use ErrAbortHandler; prior fully written chunks may exist, but a failed final read can never become complete success or append JSON. Once-only synchronous Close, existing auth/deadline/socket/cap/error/retry and SQLite-only routes are unchanged.

Actual authenticated HTTP regressions cover4,4096 and4097bytes (final partial/full scratch), complete n+nonEOFerror, canceled n+error, good n+EOF and good n+nil. Final failed chunk is not exposed; exact previously completed chunk bytes remain distinguishable. Added invalid-reader/no-progress/short-positive-read/short-write controls. Five inapplicable Admission table combinations are excluded before t.Run, replacing earlier no-op PASS events; all applicable assertions remain unchanged. Corrected bundle count is109named events: original92 minus5 no-op cases plus16 final-read and6 direct-read events. These counts must not be compared to original92 as pure added coverage.

Initial author test compilation failed due missing strconv import (preserved exit1/build failure, no semantic RED credit). Corrected authenticated RED2 ran against unchanged production:9named/distinctPASS,7FAIL,0SKIP/exit1. Three failing final-read leaf cases4/4096/4097 prove dropped error; cancellation and ordinary EOF controls already pass. Complete pre-fix test/production snapshot retained under fix1 red2 source.

Actual offline verified native WindowsGo1.27.2, same GOTOOLCHAIN=local/GOPROXY=off/GOSUMDB=off and existing root caches; MISKOAI_* names cleared without values. Final commands from new source:

- `go test -count=1 -json -timeout=30s -run '^TestLibraryBundleHTTP' ./internal/web` initial GREEN103named/distinctPASS/0FAIL/0SKIP/exit0 (before6 bounded-reader controls).
- `go test -count=1 -json -timeout=45s -run '^Test(LibraryBundleHTTP|DownloadCancelCleanup|DownloadHTTPTruncationAndPreCanceledContext|Download|RestoreStreamBoundsAndTypes|Authentication|CSRF|Origin|Host)' ./internal/web` ->128named/distinctPASS/0FAIL/0SKIP/exit0, empty stderr.
- `go test -count=1 -json -timeout=30s -run '^Test(Login|Session|RandomFailure|MalformedLogin|AuthHeaders|ApplicationDeadlines|HTTPAbort|URLQuery)' ./internal/web` ->12named/distinctPASS/0FAIL/0SKIP/exit0, empty stderr. Combined two final scoped selections140pass events/140distinct tests, zero failures/skips.
- `go vet ./internal/web` ->exit0, empty stdout/stderr, exact corrected source and tests.
- Root diagnostic copied unchanged into separate NEW `.tools/reviews/library-bundle-web-fix1-oct14-proof-src` after all422 runtime source identities copied/verified. `go test -count=1 -json -timeout=30s -run '^TestRootBundleFinalReadFailure$' ./internal/web` ->3named/distinctPASS (parent plus exact2 error/normalEOF controls),0FAIL/0SKIP/exit0, empty stderr. Original root failed diagnostic/source remains untouched. Proof copy contains one extra root-authored test, absent from corrected422 production snapshot.

No full old233Web rerun was performed: the root explicitly scoped this fix to new bundle and affected transfer/auth/deadline regressions. No updated whole-product/static/native/live/39/final acceptance claim follows. Root fresh corrected review and actual combined bridge/integration gate remain required. Original full233PASS refers only to original source; the newly confirmed bug was not covered there.

Corrected complete422 source, modified-three/all-five owned manifests, differential FIX1 patch and complete-against-installer patch, actual outputs/exits/RED snapshots/root proof copies are frozen under the new fix1 prefix. Self-review checks raw Read errors before writes, exact finalEOF, cancel precedence, finite invalid-reader refusal, shortwrite abort and no changed surrounding contracts. Only author self-review credit; root fresh review follows. All work quiesces after freeze; no source changes without root handoff.

Artifact generation retained its first exit1: Python default GBK could not decode the UTF8 report's em dash. Corrected explicit UTF8 reading generates both patches; original diagnostic stdout/stderr/exit remain preserved, with no runtime-source or test change.
