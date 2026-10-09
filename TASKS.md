# MiskoAI task ledger

2026-10-09. Primary owner: GPT-6.1 High; Medium/Low selected per task.

| Phase | Deliverable / acceptance | Owner | Dependencies | Status |
|---|---|---|---|---|
| 0 | inspect existing files/name/Public repo; base docs; minimal compile | High | existing LICENSE | complete; initial docs committed before code |
| 1 | pinned official WeChat protocol; providers mock/live matrix; SQLite FTS5 and backup PoC | High + Medium protocol/storage/review + Low evidence/licenses/tests | 0; credentials for live tests | all4live cloud probes passed; QR confirmed, text receive failed before send; real WeChat deferred by owner; native Linux CI e5f0e61 passed including race |
| 2 | bounded chat pipeline, provider/channel, dedup/recovery | High + Medium | 1 protocol evidence | in progress under text-core plan; local/mock only |
| 3 | scoped facts/context/summary/history and FTS retrieval/CRUD | Medium reviewed by High | SQLite PoC | durable inbox/schema2/global quotas implemented and locally reviewed; bounded agent context locally reviewed; summaries pending |
| 4 | personality profiles and natural reply policy | High | 2/3 | pending |
| 5 | search, media, bounded documents/stickers | Medium + Low fixtures | 1/2 | pending |
| 6 | embedded authenticated Web, CLI, doctor/backup/restore | High + Low view | storage/core | pending |
| 7 | Linux amd64/arm64 builds and real 2C/512MB test | High | integrated features; authorized server | waiting server for live test |
| 8 | review/scans/licenses/regression/docs/binary release | High + Medium reviewer | previous phases | pending |

Per deliverable: write meaningful tests, observe missing behavior, implement, run suite/vet, review diff, update docs, scan and commit. Live external tests remain separate gates; keep developing unaffected local work. No phase is complete solely because a mock passes.

Current Phase1 scope: direct official cloud clients, text-only WeChat bootstrap/poll/send, scoped facts/message state and CLI backup/restore. Media transfer, durable batch receive cursor, reconnect/expiry live behavior and resource measurements remain open. This is not a completed chat service.

Text-core Task2 gate: root Windows suite144pass/1Unixskip and vet passed; independent Medium review's two admission findings fixed and re-reviewed clean. Storage persists raw frames/inbox/cursor but service transport/normalization/consumer integration is still planned. Task1 text agent locally accepted after fresh independent Medium spec/quality approval; root184pass/1Unixskip and vet0. Native Linux evidence for the new agent is pending its development commit. No real requests/private service start.

Storage2 native Linux CI gate: [run37830782509](https://github.com/hidxt/miskoai/actions/runs/37830782509) at e988028 completed successfully: unit/integration, vet, native race, govulncheck, full-history policy scan and Linux amd64/arm64 builds. The text agent is not included in that commit. This hosted runner evidence does not establish512MB/VPS performance.

Text-agent native Linux gate: [run37833845622](https://github.com/hidxt/miskoai/actions/runs/37833845622) at ebcfa5c succeeded on all configured unit/integration, vet, race, vulnerability/history checks and two architecture builds. Hosted Linux CI is not realWeChat or512MB/VPS acceptance. Runtime raw transport implementation is subsequent work.

Runtime receive boundary checkpoint: raw transport and pre-allocation decoder guards accepted by independent Medium, root94affected tests/vet0. Exact zero-byte BLOB persistence refinement accepted after one test precision fix/re-review; root scoped zero/storage+CLI backup/restore passed. Runtime serial poller/worker is now being implemented and is not in this checkpoint. Real WeChat remains owner-deferred; no actual private-data migration/service start. Memory/profile schema3 spec/plan prepared, not implemented. Pinned media protocol research is proposal evidence only. Native CI for this receive checkpoint is pending its commit; prior ebcfa5c CI passed.

2026-10-09 runtime localgate: serialservice freshMediumapproved afteraggregateallocationfix/re-review; root258pass/0fail/1Unixskip andfullvet0. Receivecheckpoint51ea0ed nativeCI37838234146 succeeded, service notinthatcommit. Memory/profiles Storage3 nowinimplementation, summaries/integration pending. Core/Web design drafted, notimplemented. Userreturned/resumedrealwx: exactemptylegacyDB backedup/checkpointed underexplicitownerapproval, tokenpreserved; controlledsynthetictext received,replyattemptonce returnedunknown, ownerconfirmedexpectedreplyreceived. Ambiguousstate retained/no resend; ackprotocolcause/media/reconnect remainpending. Cloudbatchneverrerun. RealVPSstillunsupplied. Progressreports mustinclude wholeproject status anddistinguish implementation/local/native/live/acceptance.
