# MiskoAI tasks

**Current release blocker (2026-10-21):** exact084c8bb CI39600092376 passed full unit/integration, vet and nativeLinux race, then failed govulncheck on GO-2026-6809 in Go1.27.2 crypto/x509 (fixedGo1.27.3); later history/build steps skipped. Engineering stays **96%**,49/50 remain open; existing candidate binaries must not be published. Project-private Go1.27.3 patch verification/rebuild is in progress under D098, with no relaxed security gate or new live workload. See [patch evidence](docs/research/go-1.27.3-release-gate.md). Earlier accepted source-bound results below remain history.

Current: 2026-10-21, owner resumed continuous routine work. Engineering **96% =48/50** at recorded phase gates; **two packages remain:49,50**. The [authoritative checklist](docs/superpowers/project-progress.md) distinguishes engineering progress from final acceptance.

## Current evidence

- **Implementation/local:** accepted Go monolith, scoped SQLite, media/document pipeline, optional text-then-static-image replies, management/CLI and operations. Frozen current production Windows full3253 namedPASS/0FAIL/4platformSKIP, fullvet0; affected CLI22PASS/vet0. Original failed/partial runs retained. [Local gate](docs/research/final-product-local-verification.md).
- **Native CI:** [616ba3b8 / CI39410051711](https://github.com/hidxt/miskoai/actions/runs/39410051711), job118585309788, all12 configured steps SUCCESS. Production compiled source is d9bc732;616ba3b8 changes documentation only. Earlier5a94038 Core reopen/race-test failure remains recorded, with no proven causal explanation from later success. [Native checkpoint](docs/research/native-final-checkpoint.md).
- **Real tests:** finite provider batch spent; strict-ACK text23, static PNG/text-image40 and controlled reconnect/official invalid-auth/new-bot recovery24 accepted with persistence/cleanup and explicit owner peer receipt. Natural TTL/full ordinary Core recovery remain separate. [Live recovery](docs/research/weixin-authorization-recovery-live-verification.md).
- **Artifacts46:** three source-bound0.1.0-dev binaries independently verified and primary hash/header checked, including full notices/module sums. No tag/release. [Artifact gate](docs/research/final-artifact-verification.md).
- **Resources47/48:** fresh approved native server idle60s plus480-primary/10min Core/SQLite synthetic workload completed; idle peak12.66MiB, full peak19.89MiB, sampled swap0. Complete remote exit0 and exact process absence independently checked after SSH transport255; no workload retry. VisibleRAM406.57MiB/configured1GiBSwap reported separately. [Resource gate](docs/research/server-resource-acceptance.md).

**Feature/UI45 accepted:** one owner-approved nativeLinux35-image browser restore succeeded,32+3 records/exact highID/validated32x32PNG, old editor reset, explicit logout/old-page clearing/freshlogin paused; fixture/wrapper0/externalcalls0 and tunnel released. [Native browser gate](docs/research/native-browser-recovery-verification.md). Windows larger-library deadline remains disclosed.

**Release metadata in progress:** only cli.Version changes0.1.0-dev to0.1.0; fresh matching artifacts and exact nativeCI refresh are required before release.

## Remaining work

| Package | Concrete next result |
|---|---|
|49|Consolidate final evidence, known limits and operator/recovery checklist after45 result; preserve previous records and source-bound qualifications.|
|50|Present reviewed binary/source/checksum/license package and concrete release action for owner decision. No deployment, tag or release yet.|

## Working boundaries

Primary owns decisions/integration/security/performance/acceptance. Codex children only GPT-6.1 Sol Medium (`gpt-6.1-sol`/medium). Owner's latest external agy choice is Gemini3.8FlashHigh (`gemini-3.8-flash-high`/high), isolated public simple UI/static/docs only; primary reviews output. Exact model/effort and xx% before dispatch/resume. No concurrent writers or repeated unchanged gates.

Private legacy schema2 DB remains unmigrated and was not opened by new auth/image/recovery tests; an earlier approved text claim did change its lifecycle state, so original bytes are not claimed unchanged. New authorization installed; both old authorization copies and old account scope retained. Do not replay consumed tests, open ordinary private Core.Run, call paid models or reuse old latches. New provider/SQLite/channel/deployment/safety/512MB scope decisions remain owner-controlled. New native browser packet is preparation only until concrete approval.

Previous complete TASKS bytes are preserved in [2026-10-21 archive](docs/superpowers/history/2026-10-21-TASKS.md), including older links/failures. Earlier stale active statements are history.
