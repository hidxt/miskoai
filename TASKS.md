# MiskoAI task ledger

2026-10-09. Primary owner: GPT-6.1 High; Medium/Low selected per task.

| Phase | Deliverable / acceptance | Owner | Dependencies | Status |
|---|---|---|---|---|
| 0 | inspect existing files/name/Public repo; base docs; minimal compile | High | existing LICENSE | complete; initial docs committed before code |
| 1 | pinned official WeChat protocol; providers mock/live matrix; SQLite FTS5 and backup PoC | High + Medium protocol/storage/review + Low evidence/licenses/tests | 0; credentials for live tests | all4live cloud probes passed; QR confirmed, text receive failed before send; real WeChat deferred by owner; native Linux CI e5f0e61 passed including race |
| 2 | bounded chat pipeline, provider/channel, dedup/recovery | High + Medium | 1 protocol evidence | in progress under text-core plan; local/mock only |
| 3 | scoped facts/context/summary/history and FTS retrieval/CRUD | Medium reviewed by High | SQLite PoC | durable inbox/schema2/global quotas implemented and locally reviewed; bounded agent context in progress; summaries pending |
| 4 | personality profiles and natural reply policy | High | 2/3 | pending |
| 5 | search, media, bounded documents/stickers | Medium + Low fixtures | 1/2 | pending |
| 6 | embedded authenticated Web, CLI, doctor/backup/restore | High + Low view | storage/core | pending |
| 7 | Linux amd64/arm64 builds and real 2C/512MB test | High | integrated features; authorized server | waiting server for live test |
| 8 | review/scans/licenses/regression/docs/binary release | High + Medium reviewer | previous phases | pending |

Per deliverable: write meaningful tests, observe missing behavior, implement, run suite/vet, review diff, update docs, scan and commit. Live external tests remain separate gates; keep developing unaffected local work. No phase is complete solely because a mock passes.

Current Phase1 scope: direct official cloud clients, text-only WeChat bootstrap/poll/send, scoped facts/message state and CLI backup/restore. Media transfer, durable batch receive cursor, reconnect/expiry live behavior and resource measurements remain open. This is not a completed chat service.

Text-core Task2 gate: root Windows suite144pass/1Unixskip and vet passed; independent Medium review's two admission findings fixed and re-reviewed clean. Storage persists raw frames/inbox/cursor but service transport/normalization/consumer integration is still planned. Fresh Medium Task1 agent implementation started; no real requests/private service start.
