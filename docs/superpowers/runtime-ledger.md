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
