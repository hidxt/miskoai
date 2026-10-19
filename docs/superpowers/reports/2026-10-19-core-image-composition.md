# Core generation image composition — 2026-10-19

Engineering progress remains **78% =39/50**. This report records local synthetic composition evidence only. Package40, native CI/race/static checks, live image/channel capability, arm64 runtime, server/512MB and final acceptance are separate gates.

## Scope and immutable dependencies

Owned changes are `internal/core/lifetime.go`, new `internal/core/image_reply.go`, new `internal/core/image_reply_test.go`, and this report. They live solely in the ignored `core-image-composition-oct19-author-src` copy. The primary retains architecture, integration, independent review, publication and acceptance. No child agents, Git changes, private database/service, external request, provider rerun or paid test is authorized or performed by this task.

The source begins at the primary's accepted475-file `image-selection-snapshot-oct18-author-src`, verified on copy against its SHA256/byte manifest. Exactly the three reviewed preparation paths from `image-upload-preparation-oct18-owned-manifest.json` override that basis. The preparation gate is primary65PASS/test0/vet0 and independent0Critical/Important/Minor, as communicated by the primary. The primary then supplied the exact frozen original478-file Agent source and four-owned-file manifest. All four were SHA256/byte verified before copying. Agent original author314PASS/test0/vet0 was reported by the primary, with independent review pending at handoff. No mutable Agent source was copied. The subsequent Agent FIX1 reservation-failure diagnostic correction (final326PASS reported by the primary) is deliberately excluded from this Core source: the primary will integrate its exact corrected Agent with these four Core paths and run a coherent gate. No foreign file was edited in place.

## Implemented composition and lifetime

The implementation replaces `startGeneration`'s unconditional plain observer with `newObservedChannel`. Full image capability, the existing Manager and owned CDN together enable immutable Agent reply composition. Production uses `replyimage.New` with the existing configured private directory/CDN and captures the observed upload locator. Missing optional dependencies preserve legacy text behavior.

The synchronous private binding captures the original generation context/marker, Store, scope and Manager. It checks captured Manager identity before borrowing and before consuming. The accepted exact-generation helper owns joined cancellation, fresh identity checks, immediate exclusive refusal and Manager → media → Store acquisition. There is no outer Store lease, new pool/transport/configuration, transaction around callbacks or retained artifact. The private synchronous preparation parameter is a test seam; production always supplies the actual Preparer.

## Verification record

All commands use native verified `.tools/toolchains/go1.27.2-verified/go/bin/go.exe`, `GOTOOLCHAIN=local`, `GOPROXY=off`, `GOSUMDB=off`, existing `.tools/go`, `.tools/gomod`, `.tools/gocache`, `.tools/tmp`, and `CGO_ENABLED=0`. Credential environment variables are cleared by name without printing values. The runner is `.tools/reviews/core-image-composition-oct19-run.ps1`.

Semantic RED compiled actual stable dependencies with deliberately no-op Core composition using `go test -json -count=1 -timeout=2m ./internal/core -run '^TestCoreImageReply'`. Compilation failures will never count as semantic RED. GREEN is scoped to new Core tests and existing generation/observed-image/library/lifetime regressions, followed by `go vet ./internal/core`. The primary explicitly narrowed this gate; repeated complete Agent/library/replyimage and whole-project suites remain the primary's integration responsibility.

Tests exercise genuine local PNG validation, actual Manager/Store leases and schema6 receipts; controlled synchronous delegates replace upload/sends. They check exact IDs, independent text/image ACKs and usage once, duplicate suppression, current/captured identities, post-validation Manager replacement, immediate exclusive refusal, shared-media wait without an outer Store lease, generation cancellation and actual Close joining preparation. Optional capability/CDN/Manager composition preserves the original Agent. A separate actual `startGeneration` test uses real encryption/preparation up to a synthetic upload-location auth-expiry return, proving observed-locator routing and actual plaintext/ciphertext cleanup without CDN access. It never invokes `Core.Run`; existing synthetic lifetime regressions may do so.

Actual results are recorded below. Exploratory reads initially named absent guessed files and one PowerShell wildcard path unsupported by `rg`; corrected searches found the actual sources. Those read errors changed no source and are not test evidence. All test failures and actual exits will be preserved under the task prefix.

## Review and remaining gates

Author self-review read the complete production binding/callsite and new tests against the accepted lifetime, Manager, preparation and actual Agent contracts. It found no outstanding production issue; this is author review, not independent approval. The filter mistake and unresolved timeout below remain explicit. Final source/owned/evidence manifests and the unified diff bind this task for the primary and fresh reviewer. Fresh permitted Medium review, primary code/diff inspection, primary scoped checks and coherent full integration must precede adoption. Source manifests bind files by SHA256 and bytes; the owned, full-source and evidence manifests and proper unified diff are emitted by the retained freeze script. This task cannot claim release/live/resource acceptance or increase the project percentage.


## Actual RED/GREEN commands and outcomes

All outcomes count named tests including named parents/subtests, matching the project evidence convention. No undefined-symbol/compilation failure occurred in the semantic RED.

| Phase | Actual command after environment setup | Result |
| --- | --- | --- |
| Semantic RED | `go test -json -count=1 -timeout=2m ./internal/core -run '^TestCoreImageReply'` |6 namedPASS,20FAIL,0SKIP; packageFAIL45.187s; actual test1; empty stderr. No-op binding and unchanged generation callsite are preserved with the exact new test source under `red-*.go.txt`. Failure observations include no two-part receipt, stale Core identity still reaching legacy text, and actual generation not reaching image preparation.|
| Initial overbroad GREEN attempt | `go test -json -count=1 -timeout=3m ./internal/core -run '^Test(CoreImageReply|GenerationLibrary|ObservedImage|CoreSharedModel|CoreCancel|CoreAuthExpiry|LibraryExclusive|CoreLibrary)'` |57 namedPASS (including all26new),0 named assertion FAIL/0SKIP, but incomplete packageFAIL183.300s after global3m timeout in `TestCoreLibraryBundleLeaseOrdering`; actual test1/vet0; not passing evidence. Exact runner and complete logs retained.|
| Final corrected scoped GREEN | `go test -json -count=1 -timeout=3m ./internal/core -run '^Test(CoreImageReply|GenerationLibrary|ObservedImage|CoreSharedModel|CoreCancel|CoreAuthExpiry)'` |86 named/distinctPASS (all26new),0FAIL0SKIP; packagePASS81.842s; actual test0/process0.|
| Core vet | `go vet ./internal/core` |actual0, empty diagnostics. Final stderr is empty.|

No production code or assertion changed after the first GREEN run began. The corrected filter completes the intended bounded gate; it does not diagnose the earlier timeout or substitute for the primary's coherent whole-project/current-Core gate. The original Agent source remains exact throughout.

The original overbroad run's complete JSON is `.tools/reviews/core-image-composition-oct19-green-tests.jsonl`; lossless concatenated output is `-green-output.txt`; all57 completed named test elapsed values are in `-green-completed-timings.json`. At the alarm the framework reported `TestCoreLibraryBundleLeaseOrdering (5s)`. Goroutine35162 was in that test's cleanup waiting on `<-done` at `internal/core/library_bundle_test.go:903`. Restore goroutine35659 was in Windows `GetFileAttributesEx` through `privatefs.ancestors/CheckFile`, `media.(*spool).publish:113`, `StageLocal`, `validateBundleImage`, `checkPrepared/preflightBundle/installBundle`, and `RestoreLibraryBundle`; the dump contains no new Core image-binding frame. Completed top-level bundle examples: download10.19s, restore/pause20.85s, empty/slots11.45s, input-refusals11.16s, output-failures28.96s, install-lock10.97s, media-gates14.65s, legacy-empty4.87s. Cumulative timeout is plausible from this evidence, but it is not a proven diagnosis and the narrow pass does not clear a possible broad regression. The primary explicitly owns the complete current-Core/whole-product gate with sufficient timeout. No broad rerun was performed by this task.

## Exact original Agent dependency

The original Agent478 source and manifests remain under `.tools/reviews/agent-image-orchestration-oct19-*`; this task copied exactly its four manifest rows:

| Path | Bytes | SHA256 |
| --- | ---: | --- |
| `internal/agent/agent.go` |11016|`458e285eff3717f9c8ae3ce43b56a93cd8a903b1400282fb7cacabd01eb64255`|
| `internal/agent/image_reply.go` |8023|`a2da7df6d8719b14f27e319dc2a86f0ba8908cbdd8beed8db554a09c4e0f3d06`|
| `internal/agent/image_reply_test.go` |22596|`1d47d8da5d7b093039c1b5447a8958e93d131b9dcfa77b403b790a63afbcf24f`|
| `docs/superpowers/reports/2026-10-19-agent-image-orchestration.md` |11302|`27038c056fec0c38d5ead215fd9ae5dc0c5ef45a3eaedf1667ecf08191a5e57e`|

Only Core4 are offered for adoption. The original Agent's later reservation-error parent-state review finding is resolved in the primary's separately frozen FIX1 source, not in this snapshot; no claim is made that the older dependency has final review approval. Latest Agent FIX1 and Core must receive the primary's final combined source verification.
