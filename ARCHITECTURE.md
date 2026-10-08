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
Pure-Go SQLite candidate modernc.org/sqlite, pending installed-version FTS5/backup verification. Single connection initially; WAL, busy_timeout=5000ms, foreign keys, bounded cache. Explicit facts have scope/source/confidence/importance/status/expiry; summaries never overwrite facts. FTS query uses escaped user terms and mandatory scope. Chinese recall needs real keyword fallback because default FTS word segmentation is insufficient. Short context uses conservative token estimates until official accounting is available.

## Resource budget and failure behavior
Initial engineering caps (not measured guarantees): 2 in-flight model operations, 1 media parse, 1 summary worker, queue of 32; one SQLite connection, 8MiB cache, 32 recent context messages additionally bounded by budget; network response 2MiB; incoming text 16KiB. File limits are proposed engineering defaults, to be tuned only from evidence and within user-approved resource constraints. Avoid multiple whole-image copies; stream encoding where possible. HTTP server deadlines/header caps; bounded management session table and login limiter. OS RSS targets: idle 80MiB, normal 128MiB, sustained 192MiB. Native VPS verification remains mandatory before resource claims.
Timeout/rate-limit/auth/balance/validation errors are distinct safe codes. Retry only idempotent operations and documented safe failures within a total deadline. Shutdown cancels poll/model/summary work, closes HTTP and database, and preserves recovery state.
