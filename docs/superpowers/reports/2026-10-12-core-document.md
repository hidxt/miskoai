# Claimed document Core and receiver integration

2026-10-12 Task3 author: GPT-6.1 Sol Medium (`gpt-6.1-sol`, `reasoning_effort medium`), no delegation. Engineering progress remains **72%** at the existing project ledger. This report records implementation and scoped offline Windows host evidence; primary integration, fresh independent review, native CI for the application integration, live file delivery, actual arm64 execution, 512MB measurements and final acceptance are separate gates.

## Source context and ownership

Work ran in the primary checkout, `C:/Users/auzasr/Documents/Projects/miskoai`, after the primary adopted the accepted document processor and Agent dependencies and completed the 235-pass joint dependency gate. The primary reported PDF publication commit `6fff184f6f907301eaf40830bfd217dafbf9a3e4`; that commit excludes the adopted application groundwork. The author neither inspected nor changed Git or its index and does not claim that this commit contains the application dependencies used by these tests. The primary-owned frozen joint source is `.tools/reviews/claimed-document-joint-reviewed-src`.

Owned changes are only `internal/core/core.go`, `internal/core/lifetime.go`, new `internal/core/document_test.go`, new `internal/service/document_test.go`, and this report. Existing content and LICENSE remain preserved. Parser, processor, Agent, provider, storage, configuration, service production, Web, CLI, root ledgers and dependencies were not edited. New evidence uses only the `core-document-*` prefix; historical evidence was not truncated or replaced. The primary owns publication, complete frozen snapshots, scans and root ledger updates.

## Implementation and interface

`Dependencies.Documents` is an optional borrowed `agent.DocumentProcessor`; `Controller.documents` retains it. Existing Vision/Images pairing is still rejected before filesystem effects. Injected text-only constructors continue to create no CDN, image processor or document processor. Explicit document injection remains borrowed and is never silently replaced or closed.

The default owned CDN construction path calls `document.New()` once and passes that parser to `attachment.NewProcessor` once. The resulting outer processor is assigned to both image and document interfaces. The existing original DeepSeek text model and vision clone still share their transport/admission. No per-message or per-generation parser, slot, CDN or model pool is added. When Documents is explicitly injected, the existing production image path still owns its processor/CDN while preserving that separate borrowed document dependency; the shared default applies when no document override is supplied.

Generation construction selects `agent.NewWithAttachments` when Documents exists. It passes the existing counted text delegate with operation `chat`, and a counted vision delegate only when the validated image pair exists. Otherwise it retains `NewWithImages` or `New`. Thus document calls use ChatAttempts and the existing SuccessfulUsage accounting, without a document metric or retry.

The shutdown implementation is unchanged apart from its ownership comment: cancellation and actual Service/summary generation joins precede lease drainage, owned CDN close, store close and lock release. Replacement and restore also use the existing synchronous generation join. No arbitrary borrowed Close interface is called and no timeout worker is detached.

Ruling: retain explicit Documents overrides even when the default text/image constructor runs — the plan requires borrowed injection compatibility and prohibits replacement of an explicitly supplied dependency — an incorrect interpretation would change an injection caller's selected processor. This is covered by `TestCoreDocumentInjectionCompatibility` and does not change default shared admission.

Ruling: persist synthetic next-message inbox rows before starting/resuming the next Service generation — direct storage insertion does not notify a Service already waiting on its wake channel — inserting after the initial scan would test a nonexistent storage-to-service notification contract and falsely time out. Production was not changed for this fixture correction.

## Semantic RED and retained diagnostics

Before production edits, compiling tests used an optional reflection assignment to a not-yet-existing Dependencies.Documents field. Its absence left the actual baseline text-only controller intact, rather than causing a compilation failure. The final helper uses the typed field directly; the original RED source remains in `core-document-red-source.go.txt`.

Actual command:

```text
.tools/toolchains/go1.27.2-verified/go/bin/go.exe test -json -count=1 -timeout=15s ./internal/core -run ^TestCoreDocument(DurableEmptyCaptionAndMetrics|ProductionSharedProcessor)$
```

RED exit1, 0 named passes, 2 expected named failures, 0 skips. The empty genuine-caption file was durably received but produced processor0/model0/send1 and no excerpt label. Production processor parser identity was nil. Both failures were semantic, finite and compiled; Core package elapsed 1.977s. Actual events, stderr and exit are `core-document-red-tests.jsonl`, `core-document-red-stderr.txt`, `core-document-red-exit.txt` under `.tools/reviews`.

First expanded GREEN attempt (`./internal/core ./internal/service`, `-run ^(TestCoreDocument|TestServiceDocument)`, same count/watchdog) exited1 with 18 named passes and 2 named failures, including the restore child and its parent. A retained isolated diagnostic (`./internal/core`, `-run ^TestCoreDocumentMaintenanceJoinsActualCleanup/restore$`) exited1 with 0 named passes and 2 named failures. The diagnostic showed the resumed worker receiving with processor callbacks1/processed0 after direct post-start inbox insertion. All actual cleanup-join assertions had already passed. The fixture correction moved persistence before generation start/resume, with no production modification. These original failed logs remain preserved.

Focused GREEN2, same two-package document regex/count/watchdog, exited0 with 20 named passes, 0 failures and 0 skips. The final image-pair compatibility test was subsequently added and is included in the final complete scoped union below.

## Final local verification

The exact reproducible native offline runner is `.tools/reviews/core-document-run-final.ps1`. It removes `MISKOAI_*` by name, disables provider credentials, sets GOTOOLCHAIN local/GOPROXY off/GOSUMDB off, and uses the root `.tools/go`, `.tools/gomod`, `.tools/gocache`, `.tools/tmp` directories. No credential value is printed. It does not evaluate or slice historical runner scripts.

For each of `./internal/core` and `./internal/service`, `go test -list ^Test` enumerated the compiled top-level tests. The runner divided that exact inventory into nine disjoint shards of at most five top-level names each. Every actual test command is:

```text
.tools/toolchains/go1.27.2-verified/go/bin/go.exe test -json -count=1 -timeout=15s ./internal/{core|service} -run ^(exact-disjoint-top-level-names)$
```

`.tools/reviews/core-document-final-results.json` records every complete argument array, selected names, actual exit and event counts. The recorded union was independently compared to each compiled inventory: Core43/43 distinct top-level tests; Service44/44; no missing, extra or repeated top-level name.

| Scoped package | Shards | Top-level tests | Named passes including subtests | Failures | Skips | Actual shard exits |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| Core | 9 | 43 | 79 | 0 | 0 | all 0 |
| Service | 9 | 44 | 127 | 0 | 0 | all 0 |
| Total | 18 | 87 | 206 | 0 | 0 | all 0 |

`go vet ./internal/core ./internal/service` exited0; stdout and stderr are both empty. All individual shard JSON streams, stderr and actual exit files are preserved. These are complete scoped package checks, not whole-module, static, native Linux/race or live acceptance.

New test coverage includes:

- Durable empty genuine-caption file claim before document processing, one model and one send, maximum unsigned protocol ID, visible excerpt label, ChatAttempts once and empty USER-history exclusion.
- Unsupported extension adapter result producing one honest response without a model call; the borrowed fixture performs no download. Real processor no-fetch allowlist and actual supported parser behavior remain established by the separate accepted processor/joint tests, not fabricated by this Core mock.
- Successful text usage retained when borrowed processor cleanup subsequently fails.
- Genuine caption history on an acknowledged reply; acknowledged and ambiguous database reopen preserving receive/claim dedup with zero processor/model/send replay.
- Held synchronous document cleanup under Close, generation replacement and restore. Every release/cancel/join cleanup is registered before assertions. Old database remains usable and ownership lock remains held while cleanup is blocked. Close releases the lock only after actual join; replacement/restore retain the borrowed processor and admit a later document. A borrowed Close method is never invoked.
- Default image/document outer processor identity and nonnil private parser pointer identity, unchanged across generation replacement, saved settings and database restore. Tests only observe pointer identity, do not export or read parser contents, add getters, or weaken private interfaces.
- Explicit borrowed document preservation with default or borrowed text; legacy borrowed text construction retains nil media resources; Documents does not relax the Vision/Images pair-before-filesystem check.
- Service synthetic protocol frame through real normalization and durable SQLite inbox/cursor before handler delivery: exact file descriptor including length/MD5/key, empty genuine caption, exact high ID, one completion. Unauthorized account/user frames ignore an invalid key before consumption; an authorized invalid file key quarantines without handler effects or cursor advancement.

## Limits, freeze and handoff

Core file delivery uses a borrowed synthetic processor/result and mock text/channel delegates. Service tests use actual synthetic frame normalization and durable storage with a test handler. Production construction tests instantiate resources without calling a provider/CDN; they prove identity and reuse, not a real network/file delivery. The accepted real Parser/processor/Agent combination was verified separately by the primary. No real credential, private schema2 database, user chat, media, ordinary receiver startup, cloud/CDN/WeChat request, server or container/WSL workflow was used.

Owned CDN closure and constructor unwind remain source-proven. The prior image test's invalid-CDN-request assertion is not a runtime closure observation and earns no strengthened closure claim here. Existing final43/45 Minor follow-ups are unchanged.

All four source files and this report, plus all actual author RED/GREEN/diagnostic/final/vet/runner evidence, are frozen with lengths, SHA256 and independently checked copies under `.tools/reviews/core-document-owned-hashes.json` and `.tools/reviews/core-document-evidence-hashes.json`. No frozen source/report includes a manifest's hash recursively. Actual/copy paths are recorded in those manifests.

The author is quiescent after the final runner has returned actual exit0 and all held goroutines have joined. No author Go/PowerShell job remains active and no further file edits are planned. Primary must verify identities and source/diff, use a fresh permitted reviewer, run complete accepted frozen-source integration/scans/native gates and update its ledgers before acceptance/publication. No additional progress credit or final/live/resource result is claimed by this task.

## FIX1: synthetic fixture secret-scan false positives

2026-10-12 bounded author resume, ONLY `gpt-6.1-sol`, `reasoning_effort medium`, no delegation; engineering **72%** unchanged. The primary reported a complete frozen332-file application snapshot with native Windows1725 named passes, four existing platform skips and full vet0, followed by a Gitleaks exit1 with three generic-api-key matches. Those original root source/tests/static/scanner records remain preserved and are not replaced by this correction.

The three scanner matches were fixed synthetic AES fixture values in the new Core/service tests, encoding the publicly recognizable 16 bytes 00,11,...,ff. They were not real credentials or private input. FIX1 replaces them with the canonical Base64 encoding of sixteen zero bytes. This retains canonical key encoding and the exact AES128 key length accepted by the existing descriptor validation. No production file, secret-scanner rule, allowlist, suppression, protocol, assertion or provider setting changes.

Inspection also found two literal search operands in the Service invalid-key mutations. Replacing only the three reported values would make those mutations no-ops. The primary explicitly agreed to update all five related literals: Core1 and Service4, including both invalid-key search operands. Both unauthorized-key and authorized-invalid-key quarantine assertions are preserved and pass.

Before editing, all original five owned files and 78 evidence files, both actual and copied paths, passed 166 SHA256/length identity checks with zero mismatches. The original manifests and frozen copies remain untouched. Reversing the five substitutions in memory exactly reproduces both original test-file SHA256 values, proving the test changes contain only those substitutions; Core production hashes remain unchanged. Proof is recorded in `.tools/reviews/core-document-fixture-fix1-change-proof.json`. No new RED test was added for this reversible fixture-value correction, as explicitly instructed by the primary; original semantic RED evidence remains available.

The new explicit offline runner is `.tools/reviews/core-document-fixture-fix1-run.ps1`; no old runner was sliced, evaluated or modified. The runner compiled fresh inventories and ran the complete Core43/Service44 top-level union in 18 disjoint shards of at most five top-level tests each, using native Go1.27.2, `-json -count=1 -timeout=15s` and the same offline root caches/synthetic environment. Actual results: Core79 named passes, Service127 named passes, **206 total named passes, zero failures/skips**, all18 shard exits0. Both compiled inventories match the selected unique union exactly. `go vet ./internal/core ./internal/service` exited0 with empty stdout/stderr; the runner returned actual exit0.

Exact test argument arrays, names and actual counts are in `.tools/reviews/core-document-fixture-fix1-results.json`, and union checks are in `.tools/reviews/core-document-fixture-fix1-union.json`. All shard events/stderr/exits and vet evidence use the new `core-document-fixture-fix1-*` prefix. No original RED/GREEN/final/diagnostic log was overwritten. The primary owns the corrected public/source Gitleaks rerun; the author makes no new scanner-clean claim and scanned no private filesystem.

Corrected owned identities and actual/copy hashes are frozen in `.tools/reviews/core-document-fixture-fix1-owned-hashes.json`; new evidence is frozen in `.tools/reviews/core-document-fixture-fix1-evidence-hashes.json`. The three changed files are the two new test files and this appended report; both unchanged production files are also identified in the corrected owned manifest. The original report prefix is preserved byte-for-byte. After the new runner returned exit0, all test lifetimes had joined and the author is quiescent. Fresh primary review, corrected frozen full/scanner/publication/application-native gates remain separate; no new engineering/native/live/resource/final acceptance credit is claimed.