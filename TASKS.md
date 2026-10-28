# MiskoAI tasks

2026-10-29 revision checkpoint: engineering **98% (49/50 historical phase gates)**. Owner started F01–F08; all eight groups are implemented in the working candidate. Primary full Windows **3319 PASS / 0 FAIL / 4 platform SKIP**, full vet 0, Node **61 PASS**, three platform builds 0 and current source/symbol govulncheck **0 findings**; concentrated independent review has **0 unresolved C/I/M**. Actual synthetic browser checks passed for login/settings/personas/events/PNG/responsive layout. Fresh exact-source native CI/race, live test-server replacement and owner recall/image acceptance remain pending; existing pilot and deferred public release are unchanged. See [revision evidence](docs/research/trial-revisions-local-verification.md) and [prepared maintenance scope](docs/operations/trial-revision-update-2026-10-29.md).

Current: 2026-10-22. Engineering **98% =49/50** at recorded phase gates. Owner deferred public release50 and requested a test-server full-flow trial before feedback/revisions. [Pilot plan](docs/superpowers/plans/2026-10-21-test-server-pilot.md). The [authoritative checklist](docs/superpowers/project-progress.md) distinguishes engineering progress from final acceptance.

## Current evidence

- **Implementation/local:** accepted Go monolith, scoped SQLite, media/document pipeline, optional text-then-static-image replies, management/CLI and operations. Fresh Go1.27.3 fullWindows3266namedPASS/0FAIL/4platformSKIP,fullvet0/govuln0; older3253 Store.Close full and22 CLI scoped results remain separately recorded. Original failed/partial runs retained. [Local gate](docs/research/final-product-local-verification.md).
- **Native CI:** exact `dce3c1b480fbe8acb2223a498edbe9baeac7533b` / [CI39604537169](https://github.com/hidxt/miskoai/actions/runs/39604537169), job119237235022, all12SUCCESS onGo1.27.3. Older616/d9 results and084 vulnerability failure remain history; earlier5a94038 race-test failure has no proven causal explanation.
- **Real tests:** finite provider batch spent; strict-ACK text23, static PNG/text-image40 and controlled reconnect/official invalid-auth/new-bot recovery24 accepted with persistence/cleanup and explicit owner peer receipt. Natural TTL/full ordinary Core recovery remain separate. [Live recovery](docs/research/weixin-authorization-recovery-live-verification.md).
- **Artifacts46:** fixedGo1.27.3/0.1.0 threeplatform binaries and seven final assets/source/manifests/complete notices primary-reviewed and freshly read back. [Final verification](docs/research/release-0.1.0-verification.md).
- **Resources47/48:** fresh approved native server idle60s plus480-primary/10min Core/SQLite synthetic workload completed; idle peak12.66MiB, full peak19.89MiB, sampled swap0. Complete remote exit0 and exact process absence independently checked after SSH transport255; no workload retry. VisibleRAM406.57MiB/configured1GiBSwap reported separately. [Resource gate](docs/research/server-resource-acceptance.md).

**Feature/UI45 accepted:** one owner-approved nativeLinux35-image browser restore succeeded,32+3 records/exact highID/validated32x32PNG, old editor reset, explicit logout/old-page clearing/freshlogin paused; fixture/wrapper0/externalcalls0 and tunnel released. [Native browser gate](docs/research/native-browser-recovery-verification.md). Windows larger-library deadline remains disclosed.

**Delivery49 accepted:** finalsource/CI/artifacts/limits/operator checklist and concrete release plan reconciled;0.1.0 is not tagged/published. [Owner decision50](docs/operations/release-0.1.0-plan.md).

## Remaining work

**Current owner start2026-10-28:** “先修复这些，开始修复” lifts the prior record-only hold for F01—F08. Implementation has started in three disjoint permitted gpt-6.1-sol/medium tasks, primary integrates/reviews. [Current design](docs/superpowers/specs/2026-10-28-trial-revisions-design.md), [implementation plan](docs/superpowers/plans/2026-10-28-trial-revisions.md). Historical hold statements below are superseded for this scope. New implementation/local/native/live/final gates pending;98% unchanged. Existing server candidate remains running until a verified update/maintenance boundary; public release still deferred.

**Additional trial feedback2026-10-28:** owner invoked frontend-design and specified dashboard hour+minute uptime/more diagnostic codes/compact channel-summary/global copy cleanup/latest3events+independent events page, unified backup/restore, explicit model-edit mode+defaults, simpler custom-only人物设定, and modern frosted-glass UI. Consolidated as F04—F08 alongside original F01—F03 in docs/operations/trial-feedback-2026-10-22.md. Previous record-only hold remains; no code/configuration/deployment changes or new tests. Eight feedback groups are not eight new engineering packages;98% unchanged.

**Owner direction2026-10-22:** selected AI emotion-aware image option1 but explicitly requested recording only while collecting more feedback. [Consolidated trial feedback](docs/operations/trial-feedback-2026-10-22.md) records semantic selection and30s/3min/10min limits. Do not start product fixes/new behavior/deployment until owner says to begin consolidated revisions; current trial service continues on existing rules. No implementation/test/CI/live/acceptance credit for the selected proposal.

**New owner-trial blocker:** direct browser save/keyword fact search and history persistence/search passed, but natural Chinese full-question recall failed with owner peer confirmation and two stored test exchanges. Shared whole-term retrieval returns no match for the full question; short keywords find the saved fact. No fix/deployment change yet. [Failure and source trace](docs/research/chinese-memory-recall-trial-failure.md). Prior local/native passes do not accept this new live gate; engineering98% is unchanged.

| Package | Concrete next result |
|---|---|
|50|Public release deferred by owner; no tag/Release/asset publication. First finish the newly requested isolated server trial and receive feedback.|

**Owner trial:** exact0.1.0/Go1.27.3 Linuxamd64 installed under a fresh locked nonroot account/system unit. Initial configuration/doctor, actual127.0.0.1:18790 listener, nine HTTP checks and manager-confirmed successful stop passed. Owner approved and completed new hidden-input test Keys/fresh QR/normal trial billing. Private provisioning/config validation succeeded; deliberately started service is receiving, both keys loaded, text/vision modelsdeepseek-flash. Ten further login/status/scoped-management/logout checks passed; at that observation zero provider attempts/messages. Owner confirmed the first ordinary AI reply; metadata shows1received/1processed/1sent/1chat attempt/0failed/0ambiguous. Wider user trial/feedback pending. No auto-restart/boot enable or old local state transfer. [Actual pilot evidence](docs/research/test-server-pilot-verification.md). No new percentage credit.

## Working boundaries

Primary owns decisions/integration/security/performance/acceptance. Codex children only GPT-6.1 Sol Medium (`gpt-6.1-sol`/medium). Owner's latest external agy choice is Gemini3.8FlashHigh (`gemini-3.8-flash-high`/high), isolated public simple UI/static/docs only; primary reviews output. Exact model/effort and xx% before dispatch/resume. No concurrent writers or repeated unchanged gates.

Private legacy schema2 DB remains unmigrated and was not opened by new auth/image/recovery tests; an earlier approved text claim did change its lifecycle state, so original bytes are not claimed unchanged. New authorization installed; both old authorization copies and old account scope retained. Do not replay consumed tests, open ordinary private Core.Run, call paid models or reuse old latches. New provider/SQLite/channel/deployment/safety/512MB scope decisions remain owner-controlled. Native browser45 completed under its separate explicit approval; no further live workload is inferred.

Previous complete TASKS bytes are preserved in [2026-10-21 archive](docs/superpowers/history/2026-10-21-TASKS.md), including older links/failures. Earlier stale active statements are history.
