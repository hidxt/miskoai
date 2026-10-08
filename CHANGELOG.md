# Changelog

## Unreleased — 2026-10-09
- Initialized requirements, security policy, architecture, phased task ledger and evidence classification before business development.
- Preserved initial Apache-2.0 license and existing public repository.
- Added development CLI, bounded DeepSeek chat/stream/vision and Ollama hosted search, official-source WeChat text adapter, scoped SQLite FTS5/facts/message states, consistent backup and strict snapshot restore.
- Added synthetic regressions for isolation, duplicates, malformed streams/images, network limits, IPv6 transition SSRF defenses and QR state/verification handling.
- Fixed six independent review findings; added embedded dependency notices and opt-in Windows credential/test scripts.
- Cross-built development Windows amd64 and Linux amd64/arm64 binaries. One authorized real DeepSeek text/stream/PNG and Ollama search batch passed; deepseek-flash explicitly selected. WeChat and real512MB acceptance remain unverified.
- Restricted the finite WeChat text probe to the exact synthetic marker before persistence. Corrected native Unix test fixtures to create0700 children without weakening data directory protection; revised native CI result pending.

No released application version yet.
