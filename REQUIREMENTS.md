# MiskoAI requirements

Source: user master task received 2026-10-09. No product requirement changes approved.

## P0: mandatory architecture and safety
Go modular monolith, SQLite, one resident process, no local LLM/GPU, no Redis/other database service. Linux amd64 and arm64 native binaries; embedded HTML/CSS/vanilla JS; systemd. Target 2 vCPU / 512MB. DeepSeek official text/vision and Ollama hosted search. Study official Tencent/openclaw-weixin and implement a lightweight adapter only if evidence supports it. No auth bypass or guessed protocol.
Every remote call has cancellation, timeout, concurrency cap, request/response size bounds, classified errors and bounded retry policy. Never retry ambiguous chat sends. Web listens on 127.0.0.1 by default; authentication, CSRF, XSS defenses, credential redaction and secure sessions required. Scope-isolated storage, safe uploads, no arbitrary model tools. Public repository secrets protection is a hard gate.

## P1: functional acceptance
Message pipeline: authorize → persistent dedup → normalize → bounded context → scoped facts → personality → allowlisted tools → model → output validation → channel reply → persistence → bounded summary work. Unique IDs account/channel scoped. Restart and duplicates must not repeat side effects.
Memory distinguishes short context, explicit facts, summaries, searchable history. FTS5 plus keywords/time/importance/source; no semantic database. CRUD, correction, clear/export with user isolation. Model guesses never become confirmed facts. Multiple personality profiles control expression, never permissions.
On-demand search preserves source title/URL/content. Validated image input to configured DeepSeek vision. TXT/Markdown/PDF/DOCX bounded document extraction; scanned-PDF OCR may be an explicit unsupported limitation. Local image/GIF sticker metadata, hash, tags, enable/disable/delete, frequency controls; native sticker capability must be proven.
Embedded management: Dashboard, AI/personality/context, Memory, stickers, System/diagnostics/backups. CLI: init, serve, status, doctor, config validate, backup, restore, version. No secrets in command arguments.
SQLite: versioned migrations, transactions/indexes, busy timeout, assessed WAL, consistent backup/restore. Installation/upgrades/uninstall documented without extra server runtimes.

## Acceptance evidence
Release requires compiled native binaries, tested main flows, actual provider calls and image validation, authenticated management, doctor, reliable storage and no outstanding high-risk vulnerability. Real WeChat is either passed with evidence or explicitly blocked. Real VPS memory results only after an authorized supplied machine: idle <80MiB, chat <128MiB, sustained <192MiB are targets, not guarantees. No server supplied means waiting, never passed.

## Excluded
Containers, WSL, microservices, local Ollama/local models, permanent Node/Python/Java frontend/parser processes, vector DB, arbitrary shell, fabricated real API/WeChat/VPS test results. No automatic supplier/route/safety/resource changes.
