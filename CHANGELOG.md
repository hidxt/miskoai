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

- Added durable serial polling/normalization with bounded raw receive admission and cancellation/join behavior; reviewed schema3 stores derived summaries, unconfirmed statement candidates and persistent expression profiles without automatic fact promotion.
- Added a fixed-scope coalesced summary worker with16-message watermark,60-second failed-attempt cooldown, strict bounded JSON and revision CAS. Chat/profile/candidate integration is under a separate task review.
- Extended send acknowledgement under explicit owner approval: preserve explicit ret=0; absent ret requires complete successful JSON, positive uint64 message_id and no error status. Uncertain sends remain claimed and are never automatically replayed. Controlled live delivery was confirmed; fresh strict-ACK validation remains pending.
- Updated Go baseline to1.27.2 for official standard-library security fixes. Reviewed checkpoint358fc7a passed native Linux CI unit/integration, vet, race, govulncheck, full-history policy scan and amd64/arm64 cross-builds. Static gosec findings are retained and triaged rather than presented as zero warnings.

- Added reviewed shared lifecycle ownership and stopped backup/restore, bounded private candidates, strict restore pause marker and checked Linux directory synchronization. Backup identity now travels from exclusive/open handles through validation and owned-only failure cleanup; root normal-user Windows655entries/vet passed. Core/serve/Web remain pending.

No released application version yet. Hosted Linux evidence is not512MB/VPS or arm64-runtime acceptance; lifecycle/Web/media/final release remain open.
