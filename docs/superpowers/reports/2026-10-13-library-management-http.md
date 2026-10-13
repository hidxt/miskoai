# Private library HTTP author report — 2026-10-13

Task 6 from `docs/superpowers/plans/2026-10-13-library-management-http.md`, executed only with gpt-6.1-sol / medium. Engineering progress remains **74%** (37/50). This isolated implementation and normal-owner Windows local evidence are provisional until primary acceptance. No package 39, native CI, live, resource or final acceptance credit.

## Isolation and ownership

Dependency is exactly `.tools/reviews/library-core-snapshot-root-reviewed-src`, using the sibling `library-core-snapshot-root-source-manifest.json`. Before editing, all **392** manifest entries were checked against actual SHA256 and byte length, copied to `.tools/reviews/library-management-http-work-src`, and verified again. `library-management-http-base-manifest.json` records the complete copied base. The original request's manifest location inside the source directory did not exist; the exact named sibling manifest is the actual supplied root manifest, with 392 entries. No suffix or alternative dependency source was guessed.

Owned source only:

- `internal/web/application.go` — exact method map, query allowances and dispatch.
- `internal/web/library.go` — optional capability, DTO validation, strict metadata parsing, fixed errors and synchronous streaming.
- `internal/web/library_test.go` — real authenticated Server/Core/SQLite PNG/GIF lifecycle and bounded refusal/lifetime cases.
- This report.

All other isolated base files retain their exact dependency identities. Main workspace source, Git, server/auth/CSP/assets, Core/library/storage code, root ledgers, actual private data, providers, network, CDN calls, WeChat and services were not modified or started. Default Core constructs borrowed transport objects but is never Run; no provider/channel requests occur.

## Implemented behavior

GET `/api/library` performs one bounded fixed-scope metadata list with canonical `after` and `limit`, validates count and strictly increasing positive IDs, and returns metadata only with `availability: "unchecked"`. ID, usage and last-used milliseconds remain canonical decimal strings. Scope and private paths are omitted; hostile metadata remains escaped JSON and decodes losslessly.

POST `/api/library` accepts one exact `application/octet-stream` type, no encoding and a known length 1..4194304. Validation occurs before reads and backend invocation. `http.MaxBytesReader` and a cancellation-checking reader feed the actual Core processor with empty metadata. Full image validation and exact EOF remain Core/Manager responsibilities. Upload is disabled by default; duplicate content preserves original metadata/state and never invokes an implicit edit.

PATCH accepts only required ID/tags/emotion/description/enabled fields through the existing strict raw-object parser, with a 16 KiB total body cap, decoded UTF8/control/length/tag uniqueness and encoded-tags limits before effects. DELETE uses the unchanged common no-body admission and canonical query ID. Missing optional capability leaves legacy management fixtures usable and returns fixed 503 `web_unavailable`.

Preview uses `WithLibraryAsset(ctx,id,false,consume)` and checks actual row/image/artifact identity, digest, dimensions, frames, format and size before headers. It streams exact bytes through `io.SectionReader` with 4096 bytes of scratch, correct PNG/JPEG/GIF MIME, fixed inline filename, exact Content-Length, no-store and nosniff. The artifact is borrowed: no manual Close, retained pointer, reentry, full image allocation or new pool. Read/write/short-write/cancellation or Manager cleanup failure after headers aborts with `http.ErrAbortHandler`; it never appends error JSON. Success without a consumer callback becomes fixed 500.

Primary clarified the plan's content-class ambiguity during implementation: `library.ErrContent` maps to 400 `web_content`; `ErrRecovery` to 409 `web_recovery`; capacity to 409 `web_busy`; closed/Core unavailable to 503; cleanup/unknown to 500; context cancellation/deadline to 408. The Core classes and existing general management error handling are unchanged. Cleanup takes precedence over an accompanying invalid-input class, while context retains its timeout class.

## Commands and actual results

Runner `library-management-http-run.ps1` uses exactly `.tools/toolchains/go1.27.2-verified/go/bin/go.exe`, GOTOOLCHAIN=local, GOPROXY=off, GOSUMDB=off and repository-local GOPATH/GOMODCACHE/GOCACHE/GOTMPDIR. MISKOAI environment variables are removed by NAME only and fixed synthetic data/password/provider values are supplied. Test helpers remove names and restore environment values without printing them. No ACL relaxation/escalation was required.

Actual command shape, from the isolated directory:

```text
<verified go.exe> test -json -count=1 -timeout=30s ./internal/web -run <expression>
<verified go.exe> vet ./internal/web
```

Final successful new-test evidence: `-run '^TestLibraryHTTP'`, log `library-management-http-final-new-fixed-tests.jsonl`: **58 named PASS**, zero failures/skips, package exit 0, elapsed 6.856 seconds, stderr empty. This is 13 top-level tests plus their subtests, not a whole-product run.

After adding the stronger held-cancellation admission assertion, the disjoint rerun `-run '^TestLibraryHTTPRealPreviewCloseAndCancelJoin$'`, log `library-management-http-held-final-tests.jsonl`: **3 named PASS**, zero failures, package exit 0, elapsed 2.854 seconds. It proves canceled preview retains Manager/media admission while its actual response writer is blocked: a second real upload times out before admission, and Core.Close cannot return before the held writer joins. Unblock and both goroutine joins are registered before held assertions.

Existing focused regression expression:

```text
^Test(ManagementAuthAndScope|ManagementStrictInputs|ManagementDeleteRejectsUnexpectedBodies|ManagementMutationObjectCaps|HostAndOriginRefusal|URLQueryAndOverrideRefusal|HTTPAbortPreservesInterruptedResponse|HTTPAbortSentinelPropagatesAndReleasesAdmission|CSRFAndLogout|AuthHeadersAndCookieAmbiguity|LoginNonceAndSessionFixation|ApplicationDeadlinesAndSafeFallback|DownloadCancelCleanup|RestoreStreamBoundsAndTypes)$
```

Log `library-management-http-focused-final-tests.jsonl`: **33 named PASS**, zero failures/skips, package exit 0, elapsed 0.723 seconds. Scoped Web vet exit **0**, empty `library-management-http-vet.txt`. The final evidence is a disjoint union with the changed held test rerun, not a final whole-module run or one combined invocation.

## Original failures retained

- `red`: compiled semantic RED, two named tests failed with 404 `web_not_found` for actual authenticated raw upload and legacy capability discovery. Source `library-management-http-red-library_test.go` plus the complete verified base manifest preserve the initial source; no compile/setup failure credit.
- `green1`: legacy unavailable passed; real fixture got 503 because the existing injected-model helper intentionally has no local-image processor. Corrected the new test fixture to actual default Core; no production capability workaround.
- `green2`: default fixture setup failed after setting absent configuration variables to empty values. Corrected name-based environment removal; no setup credit.
- `green3`: initial two tests passed.
- `contract1`: one semicolon-query fixture expected 400 but existing Server correctly refused malformed request target with 403 before the application. Corrected that exact status expectation; backend calls remain zero.
- `expanded1`: actual malformed/truncated/trailing uploads returned Core's content class through the original 503 mapping. Primary specified fixed 400 `web_content`, retained all no-row assertions and unchanged Core classes.
- `expanded2`: restored missing content refused with actual 409 `web_recovery`, because retained physical content requires reconciliation. Corrected the test's exact expected recovery class; listing remains unchecked and foreign ID read/delete remain 404.
- `cleanup-red`: compiled semantic RED: joined invalid+cleanup returned 400 instead of required 500. Preserved pre-fix `library-management-http-cleanup-red-library.go`; fixed precedence and passed the covering test.
- `final-new`: a PowerShell text edit introduced `tcase`; gofmt/build refused it. This is an editing/compile failure with zero verification credit, retained in original JSONL. Corrected the typo before final successful verification.

All original JSONL and stderr evidence remains under the owned ignored prefix. These failures are author evidence, not product failures hidden by weakened assertions or retries.

## Coverage and limits

Tests cover actual raw PNG and animated GIF upload; metadata/enable/disable; exact duplicate preservation; authenticated complete preview; delete; malformed bytes leaving no rows; high signed64-bit IDs; unavailable legacy backend; bad auth/CSRF/Origin/Host before effects; unknown/duplicate/query and escaped JSON aliases; missing fields/null tags/nonboolean enable; invalid UTF8 surrogate/control/NUL; decoded byte limits; fully escaped maximum metadata; upload length/type/encoding rejection without reads; over-limit/nonincreasing/invalid backend rows; safe error redaction; before-header metadata rejection; post-header read/write/short-write/cancellation/cleanup aborts; real held preview/cancel/Close joins; database-only restore with reused ID/different digest; missing content and foreign scope refusal; original content retained without availability claims.

Fault spies are limited to unreachable OS/write/cleanup/error-class branches and pre-effect call counting; primary lifecycle, image validation, storage, restore and held admission tests use real synthetic Core/SQLite/private files. No actual browser/UI, remote provider, CDN, native Linux/race/arm64 runtime, systemd, WeChat, 512 MiB machine, live service, bundle/install/reply or release result is inferred. The frozen complete source includes unchanged prior code solely as dependencies; author did not rerun the whole product.

Self-review checked the application diff and every new source path against the five review focus cases. No additional policy changes or deferred author findings. Fresh independent review, primary full source/read/integration tests/static/security, native CI and publication remain root responsibilities. Author is quiescent after freezing identities; main adoption requires root clearance.
