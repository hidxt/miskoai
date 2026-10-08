# Phase 0–1: evidence-first foundation

Purpose: establish a safe, buildable native Go foundation for a private WeChat companion whose long-term memory and administration fit a 512MB native Linux server. Binding master requirements are REQUIREMENTS.md and SECURITY.md.

Design: one module github.com/hidxt/miskoai, stdlib HTTP providers, pure-Go SQLite PoC, official-source Go WeChat protocol PoC. No business code precedes root documents. Keep real provider/account tests external and unverified without supplied authorization. Preserve existing repository/license. Select gpt-6.1-sol children only at medium/low effort per user mandate.

Phase0 acceptance: all thirteen required root artifacts exist with real initial status; minimal cmd/miskoai compiles; version exits correctly; Linux amd64/arm64 cross-build evidence recorded; secret ignore and scan gates applied before commit.

Phase1 acceptance: pin official WeChat source/license/protocol and model/search source references; verify SQLite migrations, FTS5, scoped query, consistent backup/integrity; use synthetic HTTP fixtures to validate text/stream/image shape, safe errors, max sizes, cancellation and hosted search; implement protocol only from authoritative source. Real account/media/connection/RSS remains explicitly pending.

Risks: channel source may not guarantee backend compatibility; external calls may incur costs; ambiguous sends cannot promise exactly-once across a process crash. Native VPS is required for RSS claims. Neither missing live credentials nor cross-build success constitutes production acceptance.
