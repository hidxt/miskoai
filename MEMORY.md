# MiskoAI development memory

**Current release blocker (2026-10-21):** exact084c8bb CI39600092376 passed full unit/integration, vet and nativeLinux race, then failed govulncheck on GO-2026-6809 in Go1.27.2 crypto/x509 (fixedGo1.27.3); later history/build steps skipped. Engineering stays **96%**,49/50 remain open; existing candidate binaries must not be published. Project-private Go1.27.3 patch verification/rebuild is in progress under D098, with no relaxed security gate or new live workload. See [patch evidence](docs/research/go-1.27.3-release-gate.md). Earlier accepted source-bound results below remain history.

Current 2026-10-21: engineering **96% =48/50**, remaining **49,50**. Owner wants continuous routine development, concise progress and necessary/risky decisions only. No user chats or credentials belong here.

## Identity and constraints

- Module github.com/hidxt/miskoai, brand MiskoAI, branch codex/foundation. Preserve LICENSE/content. Read mandatory constraint docs before work.
- Primary owns architecture/scope/integration/security/performance/acceptance. Codex children ONLY `gpt-6.1-sol`, reasoning_effort `medium`. Latest owner agy override: ONLY `gemini-3.8-flash-high`/`high` for isolated public simple UI/static/docs. Announce exact model/effort and authoritative xx% before every invocation/resume. No concurrent file writers.
- Native Go only; no containers/WSL/permanent extra frontend/parser runtime. Product target Linuxamd64/arm64. DeepSeek `deepseek-flash`; finite paid allowance spent. Scope/provider/SQLite/channel/deployment/safety/512MB changes need explicit owner decision; routine internal work proceeds.
- D082/D086/D090 preserve max2 independent text/image sends, claim-before-effects, unknown outcome stops/no retries/replay/startup continuation. Static PNG fallback verified; native GIF/stickers unsupported/unverified.

## Accepted source and evidence

Production d9bc7320ba6f78350fe9d622e35e5e49d28b8f2c includes checked synchronous Store.Close drain and probe maintenance ownership/pause correction. Accepted254 inputs correspond to d9/616 (documentation-only delta); working0.1.0 changes only cli.Version, pending matching builds/native refresh. Native [616ba3b8 / CI39410051711](https://github.com/hidxt/miskoai/actions/runs/39410051711), job118585309788, all12 configured steps SUCCESS. Packages42/43/44 accepted; full source manual128/128+managementJS, final review0unresolvedC/I/M; retained static83Windows/86Linux-static warnings, no clean-scan claim. Windows full3253PASS/0FAIL/4platformSKIP/vet0 and affected CLI22PASS/vet0; author fixtureAccessDenied failures and historical native failures remain retained. D091 explains a separate Close cancellation cleanup gap; causal relation to old native reopen failure is unproven.

Package46 accepted for0.1.0-dev candidate: Windowsamd64 SHAa1e0b14b484b89f8a44ca236195a1792e372b3b4485b41e908e5053a894dc82c; Linuxamd64 SHAb68024731542fea5ceec2b527ca36932c2ade1bd54ed8f80b12953488a8a54d8; Linuxarm64 SHA32fe6208b888f5eaf471529bdda8f1515bad7453a7ab7f244e628fa6da056a16. Build/metadata/license/source provenance in [artifact report](docs/research/final-artifact-verification.md). Frozen compiled bytes retained;8 line-ending-only current/frozen differences including CSS are explicit, not raw byte equality. buildvcs=false means manifests bind source; no reproducible-rebuild claim.

## Live/private boundaries

Real text23, PNG40, controlled reconnect and owner-approved invalid-auth/new-bot recovery24 passed strictACK/persistence/cleanup/localdedup/ownerreceipt. Old token actually rejected; QR same user/origin issued different bot; strict original tool refused before owner approved this specific adoption. New authorization installed with independent+retained old copies; old scope never aliased/mixed. All once latches/candidates/old data retained. Original schema2 DB unopened/unmigrated in these new tests; earlier text test changed state. No naturalTTL/ordinaryfullCore/powercrash/nativephonecode proof. No new real calls required by remaining work.

## Current resource and UI state

Fresh owner-approved A2 resource run completed60s standalone idle and600442ms paced480-primary workload; idle12.66MiB/full19.89MiB peaks, process+hostswap sampled0,483chat/482sends/30summaries/oneexpectedcancel. Remote exit0/empty stderr/all phase success; exact workload process absent, binaries/scripts hashes stable and8080 released. Long SSH transport255/server-not-responding remains unknown cause, not retried; independently collected bounded completed remote results establish completion. Old paused A2 exit143 remains separate. OS-visibleRAM406.57MiB/configured1GiBSwap retained; no pure512MB/no-swap/actualarm64/systemd guarantee. Packages47/48 accepted at this finite synthetic-adapter gate. [Report](docs/research/server-resource-acceptance.md).

UI45 accepted for declared nativeLinux scope after one separately owner-approved35-image browser restore over a bounded temporary SSH tunnel: prior_retained/channel_paused true,32+3 records/highID9007199254740993, validated complete32x32PNG, old fact/editor empty, explicit sessionrefresh/logout/oldtab32→0records/0preview/empty password, freshlogin still restore_paused. No restore-triggered automatic logout/rotation claim. Fixture/wrapper/quit0, externalcalls0, ownedprocessabsent/local+remote18789released/tunneljoined;427monitor samples peakHWM20840KiB. See [native UI report](docs/research/native-browser-recovery-verification.md). Original input169290bytes/hash24b6af7db2fbe09cb9c78c58f7db01fdb19533a62ac530de893fc384e8f58d65 untouched; packet/data retained. No realauth/privateDB/paid/WeChat/Core.Run.

Windows35-image browser408 and isolated16-image32.203s deadline in final sixth media validation remain [known limitation](docs/research/windows-library-restore-deadline.md);3-image7.820s passed. No deadlines/security/schema changes; native35 passed with existing30s budget. Existing401cancel wording Minor retained despite correct refusal/clearing. Historical single-image resource restore is not the35-image proof. All synthetic processes and owned tunnel now joined.

## Next handoff

Read [TASKS](TASKS.md) and [delivery checklist](docs/operations/delivery-checklist.md). Refresh source-bound0.1.0 artifacts/nativeCI after the metadata-only version change, reconcile49, then present concrete50 release. No actual application service remains running from completed root work. Do not reset old once markers or automatically retry ambiguous writes. Before commit/push inspect ignored/private exclusions, explicit staged diff and working/history secret scans; no force push.

Full previous MEMORY bytes: [2026-10-21 archive](docs/superpowers/history/2026-10-21-MEMORY.md). Consult archives for historical failures, not current state.
