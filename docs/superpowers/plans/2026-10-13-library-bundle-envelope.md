# Library bundle envelope implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development or executing-plans. Root integrates and reviews; continuous routine implementation is owner-authorized.

**Goal:** Implement the bounded version1 stream envelope as an independent primitive for later private-library recovery.

**Architecture:** A pure Go codec has no filesystem, database, model or network capability. It validates the entire small header before callbacks, streams one declared payload at a time, checks every digest and exact final EOF, and returns success only after actual callback/source cleanup completion. Import callbacks write only private uncommitted stages; this primitive alone cannot install a restore.

**Tech Stack:** Native Go1.27.2 standard library; no new dependency/runtime.

**Spec:** docs/superpowers/specs/2026-10-12-private-library-design.md, D069. Task3 content files and interfaces remain unchanged.

## Global constraints

- Children only gpt-6.1-sol/medium. No actual private data, provider/CDN/WeChat/service/server effects.
- Magic is exactly `MISKOAI-LIBRARY-1\n`; little-endian uint32 header length1..65536; exact EOF.
- Strict header shape: `{"database":{"bytes":N,"sha256":"64lowerhex"},"assets":[{"key":"64lowerhex","bytes":N,"sha256":"64lowerhex"}]}`. All named fields required exactly once, no extra fields, duplicate JSON keys, nulls or trailing JSON. Byte counts are integer JSON numbers.
- Database1..256MiB; assets0..128, each1..4MiB, summed<=64MiB, strictly ascending distinct keys. Whole declared envelope<=321MiB using overflow-safe arithmetic. No generic ZIP/tar or compression.
- Fixed4096-byte streaming scratch. Small bounded header/record arrays only; no whole payload allocation. Cancellation is cooperative; no detached workers or hard timeout/RSS claim.
- Caller owns stages and their cleanup. Decode callbacks must synchronously consume the exact limited payload and finish their own file Sync/Close before returning. A callback sees unverified bytes; no destination mutation is authorized before complete codec success plus later SQLite/image/row validation.

## Review focus

1. Duplicate/unknown JSON fields, noninteger/overflow sizes and unsorted keys refuse before any callback.
2. Short/corrupt/extra payloads cannot produce a successful complete bundle, even if a callback returns nil early.
3. A reader/callback/source error cannot expose raw canaries or permit processing the next record; actual source release must complete exactly once.
4. Cancellation and cleanup errors may coexist; borrowed input/output streams are never closed, and admitted work is synchronous.
5. A successful empty-assets database bundle, fragmented reads/writes and exact limits remain usable; accepted bytes are not inferred from supplied hashes alone.

### Task4a: Pure version1 envelope

**Owned files (six NEW):** internal/librarybundle/header.go, codec.go, header_test.go, codec_test.go, lifecycle_test.go, docs/superpowers/reports/2026-10-13-library-bundle-envelope.md. All existing files and Task3 files read-only. Own ignored .tools/reviews/library-bundle-envelope-*.

**Interfaces:**
- `type File struct { Bytes int64; SHA256 string }`; `type Asset struct { Key string; Bytes int64; SHA256 string }`; `type Header struct { Database File; Assets []Asset }`, exact lowercase JSON tags above.
- `type Kind uint8`, `const Database Kind = 1`, `const Image Kind = 2`; `type Record struct { Kind Kind; Key string; Bytes int64; SHA256 string }` (database key empty).
- `Decode(ctx context.Context,input io.Reader,consume func(Record,io.Reader) error) (Header,error)`: strict complete header first. Callback receives context-aware hash-counting limited reader for one record; returns nil only after exactly Bytes read and its own stage Sync/Close. Compare actual SHA before next record; probe exact final EOF. Any failure returns zero Header and never invokes another record callback. Input remains borrowed.
- `Encode(ctx context.Context,output io.Writer,header Header,open func(Record)(io.Reader,func() error,error)) error`: validate/copy header before output/source calls. Produce canonical bounded JSON, then each source exactly declared bytes plus one EOF probe, compare actual SHA, stream to output with fixed scratch. Every returned nonnil release executes exactly once including error-with-release, and before advancing/returning. Nil reader/release with nil error refuses. Source release failure is a safe cleanup error; output remains borrowed. Partial output on failure is an uncommitted artifact, never success.
- Fixed `ErrInvalid`, `ErrIO`, `ErrCleanup`; preserve actual context cancellation/deadline and these fixed classes without exposing arbitrary callback/source/read/write/release strings.

- [ ] Write minimal compiling scaffolding and meaningful synthetic semantic RED for strict header, complete round-trip, short/digest/trailing-byte refusal and source release; freeze complete RED source and actual commands/failures.
- [ ] Implement strict bounded parsing, canonical encoding, overflow-safe quotas and sequential hash-counted streaming, with only owned files.
- [ ] Test Review Focus cases; exact128/129 records, exact/over64MiB declared sum and256MiB database via header-only admission tests (no giant payload allocations); real small database/asset byte fixtures for round-trip and digest behavior. Test one-byte fragmentation, short writers, cancellation during actual read, held release, error-with-release, nil callback dependencies and safe raw canaries.
- [ ] Run inventory-based finite30s disjoint new-package test shards and scoped vet; no unchanged whole-root rerun. Preserve original failed attempts; no assertions/caps weakened.
- [ ] Self-review; freeze six owned files and all actual/copied evidence identities, report actual commands/results and limitations. No main adoption until root clearance; no bundle restore, SQLite/image validation,39/native/live/resource/final credit from this primitive.

Self-review: each interface is defined here; all five review-focus cases have explicit tests; resource bounds and source release are part of actual stream lifetimes. Full manager export/import, retained-object recovery, snapshot row/image validation and Core/Web integration remain separate dependent tasks, not implicit codec capabilities.
