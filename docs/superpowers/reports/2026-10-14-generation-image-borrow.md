# Inactive exact-generation image borrowing

Task authored 2026-10-14; resumed and verified 2026-10-18 after the usage-limit interruption. Author: gpt-6.1-sol, reasoning_effort medium. No child agents, model substitution, purchase or limit changes.

Engineering remains **78% (39/50)**. This is inactive implementation plus scoped synthetic Windows-host evidence. Root review/integration, fresh independent review, publication/native CI, live providers/real WeChat, server/resource testing and final acceptance are separate. Accepted commit d811e643e801f7fcd6d42bdf3cf0ad7d75d23bb5 / native run38540398046 / job115773423422 does not cover this slice. No package40 or final credit is claimed.

## Ownership and dependency

Exactly three NEW source deliverables:

- internal/core/library_reply.go
- internal/core/library_reply_test.go
- docs/superpowers/reports/2026-10-14-generation-image-borrow.md

All work is in `.tools/reviews/generation-image-borrow-oct14-src`; main is not modified. All463 immutable dependency entries from `image-reply-integration-root-oct14-green-src` were checked against `image-reply-integration-root-oct14-green-source-manifest.json` before copying. Post-edit baseline check again found463 exact byte-count/SHA256 matches and zero mismatches. The dependency includes reviewed Manager.WithAssetStore/schema6 groundwork and the root's authentic legacy fixture corrections; its later whole-project gates and remaining legacy-fixture diagnosis belong to root.

## Implemented contract

`withGenerationLibraryAsset` accepts the planned exact generation marker, its captured context, Store, scope, asset ID/digest and synchronous callback. Nil/invalid contexts, generation/Store/callback, nonpositive IDs, noncanonical lower-case64-hex digests and invalid/overbound scope are refused before Manager/media/file admission.

The cheap check holds Core's existing mutex. It rejects closed/closing/unavailable/paused/expired state, missing Store/Manager, unconfigured scope, canceled current contexts, exclusive maintenance and changed generation/Store/scope. The captured persistent Manager is passed into the lease factory; every real lease checks the same identities again under the mutex before atomically incrementing the existing lease count. Exclusive admission refuses immediately. Release uses sync.Once, decrements and notifies the existing changed channel exactly once. The new path never calls general c.lease or waits for c.changed, and takes no outside Store lease before entering Manager.

A caller-derived context is canceled by captured generation cancellation through context.AfterFunc. Return stops or joins that callback, checks both captured cancellation states after actual Manager/media cleanup, preserves safe classified errors and releases its own context. There is no extra30-second timeout, worker pool or lifetime authority.

Manager.WithAssetStore keeps its existing Manager -> media -> Store order, enabled-only fresh reborrow and actual content validation. Before lending anything to consume, the wrapper matches the fresh row SHA256 against the selected digest. Consume receives the actual held Store and original validated artifact with the bound context. It must complete synchronously, cooperate with cancellation, avoid Manager/Core reentry and retain neither artifact nor Store. No transaction spans consume; this helper neither records use nor sends anything.

No lifetime.go, constructor, generation start, Agent/channel callsite, schema, storage policy, provider, private data or runtime activation changed.

## Commands and actual results

All Go commands used only `C:/Users/auzasr/Documents/Projects/miskoai/.tools/toolchains/go1.27.2-verified/go/bin/go.exe` and its gofmt sibling. The retained runner clears credential environment variables by NAME, sets GOTOOLCHAIN=local, GOPROXY=off, GOSUMDB=off, CGO_ENABLED=0 and uses existing `.tools/go`, `.tools/gomod`, `.tools/gocache`, `.tools/tmp`. No downloads, ambient Go, private data, network, Core.Run, Git, full/heavy suite, race/static/CI/live/resource work was run.

1. `test -count=1 -timeout=60s -run '^TestGenerationLibrary' ./internal/core`: completed semantic RED, exit1, against the refusal-only stub. CurrentBorrow reported calls=0/ErrUnavailable; unsafe-error's expected callback also reported0. Real synthetic Core/Manager/Processor/Store/PNG fixtures compiled and ran; this was not an undefined-method compile failure. Retained red.log plus red-stub.go/red-test.go preserve the test/stub content. A complete original RED source snapshot was not separately frozen before implementation; the baseline manifest plus preserved fragments identify its inputs.
2. `test -count=1 -timeout=120s -run '^TestGenerationLibrary' ./internal/core`: first GREEN attempt failed at the aggregate120-second watchdog. Stack retained in green1.log shows HeldCallbackCancellationAndCloseJoin/close still staging its actual PNG at FlushFileBuffers, before consumer entry. No timing cause or deadlock conclusion is asserted. The original wrapper also emitted a post-run relative Get-Content path error after Set-Location leaked into the caller; runner now uses Push/Pop-Location. These original failures remain visible.
3. `test -json -count=1 -timeout=5m -run '^TestGenerationLibrary' ./internal/core`: exit0,33 test/subtest PASS events,0FAIL; package82.095s. This covers all8 new top-level tests.
4. `test -json -count=1 -timeout=5m -run '^(TestGenerationLibrary(CancelMediaWait|CheapRefusalAndLeaseIdentity)|TestCoreLibrary(CloseJoinsConsumer|CloseJoinsUploadReader|CapturedScopeRecheck)|TestBorrowedStore(ActualUse|ActualCloseOnceCancelAndFailure|HeldCallbackCloseAndMedia|LeaseReleasedBeforeActualArtifactCleanup))$' ./internal/core ./internal/library`: exit0,22 test/subtest PASS events,0FAIL; core18.718s/library7.883s. The two modified new tests add an explicit zero-outer-lease assertion at occupied actual-media entry and safe failure cleanup.
5. `vet ./internal/core ./internal/library`: exit0, empty diagnostic log.
6. Final cheap-refusal singleton after adding a failure-path changed-channel notification, plus final scoped vet: outcomes are retained in final-cheap.jsonl/final-vet.log and summarized by results.json.

The full original/native suites and unrelated legacy bundle tests were intentionally excluded by the dispatch's scoped verification constraint. Root owns whole-project integration and those existing fixture failures.

## Test evidence and self-review

Actual synthetic PNG upload/enable and held Store use prove successful borrowing, exact original artifact delivery, independent short Store mutation, one held lease through callback and no surviving artifact after return. Digest mismatch and disabled content refuse before consume; arbitrary private callback text is reduced to fixed safe errors.

Cheap refusal tests independently check generation/scope/Store/Manager/exclusive/state and the private lease factory, including idempotent release and notification. Real shared processor delegation changes each identity after initial lookup and before fresh Store reborrow, proving the second check rather than only preflight. Parent/generation cancellation are tested before entry, at an occupied real media slot and through a held consumer. Controlled generation markers avoid ordinary channel/service workers. Exclusive synthetic maintenance joins and actual Core.Close cannot complete until the held consumer returns, while Store remains usable. A real StageLocal/ValidateImage/hash processor exposes actual artifact Close to prove the Store lease is released first; cancellation and an injected cleanup classification survive actual Close. This instrumentation never fabricates successful content validation; ordinary paths use the persistent real cap-one processor.

Self-review checked: no general lease wait or outer lease, exact Manager capture and fresh identity checks, bounded/canonical input, synchronous callback, one release/notification, stopped/joined AfterFunc, cleanup/cancel classification, no transaction across callback, no activation or unowned edit. Trusted composition must supply the correct generation context because generation currently stores no context field. Cooperative callbacks remain required; this task does not make noncooperative reads/sends forcibly interruptible.

## Frozen handoff

Ignored prefix artifacts include the proper new-file unified diff, complete final source manifest (baseline463 plus3 owned files), three-file owned manifest, evidence manifest, commands, runner, original failures, JSON test logs, vet logs and results. Manifests record relative path, byte count and SHA256; evidence manifest excludes itself to avoid a circular digest. Final source is quiescent after handoff. Root must inspect the production/test/report diff, independently verify these identities and run its own integrated checks before accepting or activating anything.
