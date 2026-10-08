# MiskoAI decisions

| ID/date | Decision and reason | Scope / evidence |
|---|---|---|
| D001 2026-10-09 | Preserve existing Apache-2.0 LICENSE and Public hidxt/miskoai; module github.com/hidxt/miskoai | Existing git f1babb6 and public GitHub page; no recreation |
| D002 2026-10-09 | Direct stdlib HTTP providers, no cloud SDK/runtime | Small auditable transport, official API JSON |
| D003 2026-10-09 | Research pinned WeChat source before Go protocol code | No assumed OpenClaw independence or guessed auth |
| D004 2026-10-09 | Choose modernc.org/sqlite provisionally; verify FTS5/backup before accepting | Pure Go supports requested cross-build without server runtimes |
| D005 2026-10-09 | Record live and mock checks separately; continue independent local work | Keys/account/server not yet provided |
| D006 2026-10-09 | Preserve autonomous implementation mandate over routine skill approval gates | User explicitly requests no repeated ordinary detail approvals; key requirement changes still require consent |
| D007 2026-10-09 | Lock Tencent/openclaw-weixin 2.4.9 source at 24de5c9eb0dd5e595d7e2d090ed8a3f82870d42c; implement narrow native Go text PoC | Official protocol evidence in docs/research/weixin.md; backend/live acceptance remains separate |
| D008 2026-10-09 | Pin modernc.org/sqlite v1.60.1, single connection, WAL/FULL, 8MiB cache | Actual SQLite3.53.4 FTS5/integrity/consistent snapshot and scoped state tests passed on Windows |
| D009 2026-10-09 | Restore validates exact read-only schema before opening/migrating a candidate | Independent review found unrelated SQLite acceptance; regression now rejects without replacing existing data |
| D010 2026-10-09 | Current vision PoC accepts fully decoded PNG/JPEG only | GIF first-frame decoding cannot establish full animation validity; GIF product/media support remains required and pending |
| D011 2026-10-09 | Add x/term v0.46.0 only for hidden, bounded, cancelable human QR verification input | No secrets in arguments or logs; terminal mode restored before cancellation closes input |
| D012 2026-10-09 | Keep ambiguous sends claimed, never automatically replay | Durable message state favors duplicate-side-effect prevention; full cursor/recovery pipeline remains Phase2 |
| D013 2026-10-09 | User explicitly selects deepseek-flash for text/vision; retain configurable model fields | Default already matched; four authorized actual cloud probes passed, no provider change |
| D014 2026-10-09 | Native Unix fixtures create0700 children instead of assuming t.TempDir is private | Actual Go1.27 numbered test temp dirs use0777/umask; product security boundary remains unchanged |
| D015 2026-10-09 | Finite WeChat probe requires exact synthetic marker before claim/persistence/reply | Limit authorized test to synthetic traffic and ignore ordinary older chats |
| D016 2026-10-09 | Defer real WeChat validation and preserve private authorization while owner sleeps; continue local/mock work | Explicit latest user instruction; no new live requests or actual service start |
| D017 2026-10-09 | Add bounded persistent raw receive frames/inbox and global storage quotas before service exposure | Within existing SQLite/dedup/resource requirements; schema2 must retain schema1 snapshot compatibility |
| D018 2026-10-09 | Preserve opaque item msg_id strings and lossless numeric IDs according to pinned source | Synthetic regression reproduces prior rejection; original live error cause remains unestablished |
| D019 2026-10-09 | Reject over-cap/uncheckpointed schema1 migration without deleting data/journals; admit schema2 against effective transactional WAL view | Read-only capacity/type/field guards and initial schema checkpoint tested; completed under-cap schema1 snapshots preserved/migrated |
| D020 2026-10-09 | Enforce byte cap and configurable conservative token estimate while building context; actual provider usage separately | Avoid large tokenizer dependency; estimator UTF-8 bytes+32/message, not exact native token accounting |

| D021 2026-10-09 | Chat memory export encodes individual bounded facts and stops at16KiB before partial-array overflow | Escaping allocation regression7338415B/op RED→66082B/op GREEN; full scoped set remains storage-bounded; no memory deletion advice |
| D022 2026-10-09 | Raw poll decoder enforces message/item counts before typed-array allocation | Prevent tiny-object JSON amplification; runtime task plan updated before implementation; no live protocol claim |

| D023 2026-10-09 | Persist successful zero-byte raw responses as exact BLOB evidence before malformed decode/quarantine | No schema/version change;3-line refinement, scoped lifecycle/capacity/admission tests and independent Medium review |
| D024 2026-10-09 | Planned derived-memory writes use revision CAS and non-reused candidate IDs; privacy clearing retains ID/state dedup metadata | Requested SQLite memory extension, no actual migration; prevents stale generated jobs/review references and repeated remote effects |

No change to product/provider/deployment/resource/security constraints authorized or made.
