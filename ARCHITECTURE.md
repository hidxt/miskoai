# MiskoAI architecture

Initial design 2026-10-09; implementation progress is in DELIVERY.md.

## Approach
Use a Go modular monolith and narrow internal packages. A separate OpenClaw/Node runtime conflicts with deployment constraints; retaining it is rejected by the supplied requirements. A full rewrite before protocol validation risks untestable behavior; use official-source protocol PoCs and mock boundaries first, then integrate. No provider switch is currently needed: current official DeepSeek docs include vision.

## Modules and data
- cmd/miskoai: one executable and CLI lifecycle.
- internal/config: environment validation; no secret printing.
- internal/netx: bounded TLS transport, deadline, cancel, no redirects and safe classified errors.
- internal/provider: configured DeepSeek chat, stream, usage and vision; Ollama search data.
- internal/channel/weixin: protocol based on pinned official sources; QR authorization, polling cursor, text/media transport, auth-expiry state.
- internal/storage: SQLite schema/transactions/FTS5, scoped facts/context/messages/outbox, consistent backup.
- internal/agent: allowlisted deterministic tool policy, bounded context, personality, processing states and reply orchestration.
- internal/document and internal/media: bounded extraction/validation, private temporary files and cleanup.
- internal/web: embedded resources, sessions/CSRF, authenticated administrative endpoints.

Credentials flow config → transport only, never into model context or browser. Message identity includes channel/account/conversation/sender/message ID/type/time/reply context. Persist claim before network work. After possible send but before durable acknowledgement, mark ambiguous: manual reconciliation is safer than duplicate reply. Receiving cursor is committed only after durable ingestion.

## Storage and memory
Pure-Go SQLite modernc.org/sqlite v1.60.1 passed Windows installed-version FTS5/backup verification. Single connection; WAL/FULL, busy_timeout=5000ms, foreign keys, 8MiB cache. Explicit facts have scope/source/confidence/importance/expiry; summaries never overwrite facts. FTS query uses escaped user terms and mandatory scope, with Chinese keyword fallback. Short context uses conservative token estimates until official accounting is available. Storage2 adds bounded durable raw frames/inbox/cursor, global fact/message quotas and reply reservations; serial receive runtime is locally reviewed and its earlier checkpoint passed native CI. Compatible schema3 adds bounded derived summaries, reviewable candidates and persistent scoped profiles, with revision CAS and duplicate-ID-preserving clear.

At the Phase1 checkpoint the implemented packages are cmd/miskoai, internal/cli/config/netx/provider/channel/weixin/storage/notices. A synthetic bounded text agent now supplies explicit tools, context and builtin expression profiles with reviewed durable send behavior. Persistent custom profiles, a coalesced summary worker and agent/service integration now have local reviewed evidence; Core/CLI serve exposure and Web remain planned. Serial receive processing durably commits scoped inbox/cursor after raw evidence admission. Document/media processing remains planned. Provider inline vision currently accepts bounded PNG/JPEG fixtures; full animation/media handling is Phase5.

## Resource budget and failure behavior
Initial engineering caps (not measured guarantees): 2 in-flight model operations, 1 media parse, 1 summary worker, queue of 32; one SQLite connection, 8MiB cache, 32 recent context messages additionally bounded by budget; network response 2MiB; incoming text 16KiB. File limits are proposed engineering defaults, to be tuned only from evidence and within user-approved resource constraints. Avoid multiple whole-image copies; stream encoding where possible. HTTP server deadlines/header caps; bounded management session table and login limiter. OS RSS targets: idle 80MiB, normal 128MiB, sustained 192MiB. Native VPS verification remains mandatory before resource claims.
Timeout/rate-limit/auth/balance/validation errors are distinct safe codes. Retry only idempotent operations and documented safe failures within a total deadline. Shutdown cancels poll/model/summary work, closes HTTP and database, and preserves recovery state.
