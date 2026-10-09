# SDD ledger — plan: docs/superpowers/plans/2026-10-09-runtime.md

Base: e988028; existing codex/foundation checkout. Implementation not started; requires text Task1 gate. Root owns reviews/docs/commit; Medium task implementers do not delegate. No real WeChat/cloud/private-data service start.

| Preflight pair/task | Producer and consumer / self-consistency | Finding |
|---|---|---|
| Task1 → Task2 | RawUpdates returns successful bounded opaque body; DecodeUpdates maps bounded typed batch | Persist raw before decode; explicit business failure remains safe sentinel. Malformed200 must survive. |
| Task1 alone | Existing GetUpdates shares raw exchange; decoder limits precede typed allocation | Added pre-allocation256 msgs/items guard; absent/null msgs allowed as empty, primitive outer response rejects. |
| Task2 → Task3 | Service depends on actual storage2 and agent Result interfaces | Root confirms gates before dispatch; only safe codes/counters are public. |
| Task2 alone | One poller+worker, database work,32 wake hints, bounded backoff | Capacity retries same frame; storage/capacity handler failures retain inbox and pause safely. |
| Task3 alone | Native CI/scan/build evidence and docs before CLI exposure | Hosted runner is not512MB or realWeChat acceptance. |

- Ruling: use reviewed existing development checkout and Windows-compatible ignored .tools review packages, with durable public synthetic-only ledgers — native Bash/worktree helper is not available and existing branch isolates main — cost is extra controller responsibility to enforce file ownership and exact staging; no history reset or private data publication.
- Ruling: service counters cover the current process lifetime, Received counts normalized authorized entries including duplicate deliveries; aggregate tokens with nonnegative checked/saturating arithmetic — store ResolvePoll does not return an inserted-row count and remote usage must not wrap statistics — cost is conservative/session-only dashboard totals until persisted counters are separately designed.

- Text prerequisite accepted locally in ebcfa5c after fresh Medium review and root184pass/1Unixskip/vet0. Development push succeeded; native CI for text snapshot pending.
- Task1 started with fresh GPT-6.1 Medium /root/raw_poll, owns channel/client/types/tests+report only. Base ebcfa5c. No live/private-data action.

- Text prerequisite native CI ebcfa5c/run37833845622 completed success, including race. Raw Task1 remains in implementation/refinement, not part of that CI snapshot. Root classifier refinement: any explicit -14 should outrank other nonzero service codes.

- Task1: complete locally (base ebcfa5c, frozen working patch); fresh independent Medium spec+quality approved with no findings. Root affected Weixin/CLI/agent integration94pass,0fail/skip and scoped vet0. Full child suite supplemental passed; full root suite reserved for service integration.
- Ruling: permit0-byte raw poll bodies as BLOB evidence, while retaining2MiB upper/4frames/8MiB quotas and rejecting SQL NULL/non-BLOB — a successful empty200 is malformed JSON too, and current RecordPoll minimum would prevent durable quarantine before decode — cost is retaining an empty-body frame for owner inspection rather than silently repolling; no schema change or real-data migration.
- Runtime prerequisite refinement: fresh Low implements the exact root-selected empty-body storage guard/query changes; Medium independently reviews before service dispatch. Raw task's persistence/cursor/auth-queue cannot-verify items are explicitly checked by the service gate.

- Empty-body prerequisite complete locally: Low exact3-line product refinement, fresh Medium approved; Minor test assertion fixed round1 and scoped re-review clean. Root zero/storage+CLI backup/restore regressions passed. docs/research/empty-poll-review.md preserves limitations. No native/live/RSS claim yet.
- Ruling: exact pinned SQLite NOT NULL error is sufficient for this bounded regression — distinguishes unrelated failures without widening test dependencies — cost is visible assertion failure if pinned driver wording changes, requiring reassessment.
- Task2 started with fresh GPT-6.1 Medium /root/serial_service, owns only new service files/report. No concurrent product edits/private database/service start. Task1 and empty prerequisite frozen; root publication gate proceeds separately with explicit staging.

- Receive checkpoint committed51ea0ed and pushed codex/foundation. Staged23 files policy0/gitleaks0; full history8 commits676847bytes gitleaks0 and working/history policy282contents0. Service files explicitly excluded and still in progress. Native CI pending.
- Ruling: Duplicate counter counts Handler terminal duplicate only; Received includes normalized committed repeat deliveries — ResolvePoll suppressions have no atomic exposed count — cost is conservative dashboard duplicate statistic, no extra nonatomic inference queries. Durable side-effect prevention remains unchanged.
- Ruling: filter bounded raw authorization envelopes before typed ID/text semantics, without erasing original outer/status/cursor/count256 validity — unknown/group/bot/generating malformed IDs must not block the permitted user — cost is bounded additional raw JSON inspection; original private frame remains exact and reviewed decoder is used on admitted envelopes.

- Task2: complete locally (frozen newservicefiles), fresh Medium spec+qualityapproved; Minor aggregateJoinbudget fixedround1 andscopedre-reviewclean. Rootfullrepo258pass/0fail/1Unixskip,fullvet0. No native/live/RSS acceptance claim for service snapshot; coreCLI exposure pending.
- Receivecheckpoint51ea0ed nativeCI run37838234146 succeeded:unitintegration/vet/race/govuln/historypolicy/Linuxamd64+arm64builds; exactjobsverified. Service notincluded inthatcommit.
- Ruling: enforce aggregate16KiB text beforeJoin withnonnegative subtraction/separatorbudget — bounded2MiB wirealone stillallowed needlessrejectedallocation — cost is smallhelper/test; focusedallocationregression provesearlyadmission.
- Owner returned and explicitly resumed finite realWeChat verification. Root privately backedup/checkpointed exactempty schema1 with0-byteWAL afterexplicitapproval; no manualjournal/main deletion and originalbytesunchanged. Subsequentcontrolledsynthetic receive andonesend ran; sendreturnedunknown, userconfirmedexpectedreplyreceipt. Ambiguousclaimkept, neverresent. Pendingboundedackshapediagnostic requiresnewsyntheticmessage; no rawpayload/token persists inpublicdocs.

- Service checkpoint f7ab3c6 committed and pushed to codex/foundation. Explicit staged13 files passed policy/gitleaks; history9 commits750629bytes gitleaks0 and working/history policy311contents0. Storage3 and ack work excluded. Native CI pending for service.
- Owner-approved ack investigation used one new fixed synthetic message, never retried the prior ambiguous claim. Structural observation only: HTTP200,34bytes,complete valid JSON object,message_id present,ret/errcode absent. No identifier/raw value logged. Source types.ts239-243 and api.ts579-595 confirm optional ret in reference client; complete server contract still not established solely by source.
- Ruling approved by owner: retain explicit ret=0 and allow absent-ret ONLY with valid positive lossless uint64 message_id, successful HTTP,complete valid JSON and no error status; malformed/null/duplicate known fields remain conservative, no automatic resend — actual service response conflicts with the former stricter rule — cost is exact acknowledgement parser/tests and independent review before another new-message test.
- Fresh Medium /root/weixin_ack_fix owns only channel client/newack/tests/report. Existing tests/prior ambiguous states unchanged; no live call by child. Root live binary must use immutable reviewed export, excluding concurrent unreviewed Storage3.

2026-10-09: f7ab3c6 native CI run37890923895 passed unit/integration, vet and Linux race; govulncheck failed, so later secret/cross-build steps were skipped. Public check annotations name HTTP2 client calls. Root official-source verification found Go1.27.2 released2026-10-08; local JSON scanner under1.27.0 completed after one network EOF and reported reachable GO-2026-6603/6605/6607/6608/6610/6611/6613/6617, all fixed1.27.2. JSON exit0 is not a clean verdict. D026 patch-baseline update and private verified toolchain/revalidation underway. Prior CI success does not establish current vulnerability clearance.

ACK fresh Medium review approved with no findings; root immutable reviewed-base+frozenACK export excludes Storage3. Related package tests/vet/build passed on oldGo; live binary must be rebuilt on security patch before use. No earlier ambiguous messages retried. Storage3 fresh Medium review found schema2 sequence-admission I1; original implementer fixround1 underway, Task2 remains gated.
