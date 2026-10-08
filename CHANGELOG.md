# Changelog

## Unreleased — 2026-10-09
- Initialized requirements, security policy, architecture, phased task ledger and evidence classification before business development.
- Preserved initial Apache-2.0 license and existing public repository.
- Added development CLI, bounded DeepSeek chat/stream/vision and Ollama hosted search, official-source WeChat text adapter, scoped SQLite FTS5/facts/message states, consistent backup and strict snapshot restore.
- Added synthetic regressions for isolation, duplicates, malformed streams/images, network limits, IPv6 transition SSRF defenses and QR state/verification handling.
- Fixed six independent review findings; added embedded dependency notices and opt-in Windows credential/test scripts.
- Cross-built development Windows amd64 and Linux amd64/arm64 binaries. One authorized real DeepSeek text/stream/PNG and Ollama search batch passed; deepseek-flash explicitly selected. WeChat and real512MB acceptance remain unverified.
- Restricted the finite WeChat text probe to the exact synthetic marker before persistence. Corrected native Unix test fixtures to create0700 children without weakening data directory protection; revised native Linux CI e5f0e61 passed, including race.
- QR authorization confirmed; synthetic text receive failed before send and remains deferred by the owner. Preserved official opaque item identifiers with synthetic regression coverage; the live failure cause is not established.
- Added compatible schema2 durable receive frames/inbox/cursor, quarantine, global fact/message quotas and pre-send reply reservations. Read-only snapshot/WAL admission validates bounded row types/fields and scoped quotas; independent review findings fixed with adversarial regressions.

- Added reviewed bounded text-agent orchestration with explicit memory/search commands, three honest builtin profiles, cancelable admission, byte/token-budgeted context and one-send ambiguity handling. Incremental chat export avoids full escaped-array encoding; synthetic integrated suite184pass/1Unixskip and vet0. Storage2 native Linux e988028 passed all configured gates; new agent native evidence pending.

No released application version yet.
