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
