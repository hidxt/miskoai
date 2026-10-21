# MiskoAI tasks


Current: 2026-10-21, owner resumed continuous routine work. Engineering **98% =49/50** at recorded phase gates; **only package50 remains**. The [authoritative checklist](docs/superpowers/project-progress.md) distinguishes engineering progress from final acceptance.

## Current evidence

- **Implementation/local:** accepted Go monolith, scoped SQLite, media/document pipeline, optional text-then-static-image replies, management/CLI and operations. Fresh Go1.27.3 fullWindows3266namedPASS/0FAIL/4platformSKIP,fullvet0/govuln0; older3253 Store.Close full and22 CLI scoped results remain separately recorded. Original failed/partial runs retained. [Local gate](docs/research/final-product-local-verification.md).
- **Native CI:** exact `dce3c1b480fbe8acb2223a498edbe9baeac7533b` / [CI39604537169](https://github.com/hidxt/miskoai/actions/runs/39604537169), job119237235022, all12SUCCESS onGo1.27.3. Older616/d9 results and084 vulnerability failure remain history; earlier5a94038 race-test failure has no proven causal explanation.
- **Real tests:** finite provider batch spent; strict-ACK text23, static PNG/text-image40 and controlled reconnect/official invalid-auth/new-bot recovery24 accepted with persistence/cleanup and explicit owner peer receipt. Natural TTL/full ordinary Core recovery remain separate. [Live recovery](docs/research/weixin-authorization-recovery-live-verification.md).
- **Artifacts46:** fixedGo1.27.3/0.1.0 threeplatform binaries and seven final assets/source/manifests/complete notices primary-reviewed and freshly read back. [Final verification](docs/research/release-0.1.0-verification.md).
- **Resources47/48:** fresh approved native server idle60s plus480-primary/10min Core/SQLite synthetic workload completed; idle peak12.66MiB, full peak19.89MiB, sampled swap0. Complete remote exit0 and exact process absence independently checked after SSH transport255; no workload retry. VisibleRAM406.57MiB/configured1GiBSwap reported separately. [Resource gate](docs/research/server-resource-acceptance.md).

**Feature/UI45 accepted:** one owner-approved nativeLinux35-image browser restore succeeded,32+3 records/exact highID/validated32x32PNG, old editor reset, explicit logout/old-page clearing/freshlogin paused; fixture/wrapper0/externalcalls0 and tunnel released. [Native browser gate](docs/research/native-browser-recovery-verification.md). Windows larger-library deadline remains disclosed.

**Delivery49 accepted:** finalsource/CI/artifacts/limits/operator checklist and concrete release plan reconciled;0.1.0 is not tagged/published. [Owner decision50](docs/operations/release-0.1.0-plan.md).

## Remaining work

| Package | Concrete next result |
|---|---|
|50|Present reviewed binary/source/checksum/license package and concrete release action for owner decision. No deployment, tag or release yet.|

## Working boundaries

Primary owns decisions/integration/security/performance/acceptance. Codex children only GPT-6.1 Sol Medium (`gpt-6.1-sol`/medium). Owner's latest external agy choice is Gemini3.8FlashHigh (`gemini-3.8-flash-high`/high), isolated public simple UI/static/docs only; primary reviews output. Exact model/effort and xx% before dispatch/resume. No concurrent writers or repeated unchanged gates.

Private legacy schema2 DB remains unmigrated and was not opened by new auth/image/recovery tests; an earlier approved text claim did change its lifecycle state, so original bytes are not claimed unchanged. New authorization installed; both old authorization copies and old account scope retained. Do not replay consumed tests, open ordinary private Core.Run, call paid models or reuse old latches. New provider/SQLite/channel/deployment/safety/512MB scope decisions remain owner-controlled. Native browser45 completed under its separate explicit approval; no further live workload is inferred.

Previous complete TASKS bytes are preserved in [2026-10-21 archive](docs/superpowers/history/2026-10-21-TASKS.md), including older links/failures. Earlier stale active statements are history.
