# Private bounded media codec Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans. This isolated codec gate precedes CDN/channel integration; root dispatch is required.

**Goal:** Implement the source-proven AES128ECB/PKCS7 compatibility codec with bounded private output and exact ownership cleanup.

**Architecture:** A small internal/media package streams bytes into exclusive private spools and publishes only completed validated handles. It does not fetch URLs, start workers, parse images/documents, select replies or send messages. Later integration owns the single actual media/parser lifetime and durable side-effect claim.

**Tech Stack:** verified Go1.27.2 stdlib crypto/aes, encoding/base64/hex, io, os and reviewed privatefs. No dependency/runtime.

**Spec:** docs/superpowers/specs/2026-10-09-attachments-design.md; pinned protocol evidence docs/research/weixin-media-design-evidence.md. Tencent source attribution/license remains preserved.

## Global Constraints

- Children only GPT-6.1 Medium (gpt-6.1-sol, medium), no delegation/network/private data/staging/commit. Linux amd64/arm64 native512MB target remains unverified.
- Plaintext4MiB; ciphertext4MiB+16. AES128ECB/PKCS7 is wire compatibility, not authenticated encryption. No IV or invented MAC. MD5 remains separate later protocol metadata, not security proof.
- Private random exclusive spool, no personal filename in path/error/output. Only completed valid output is exposed. Exact original file identity is retained before any payload and through reopening/cleanup; replacement cleanup refuses.
- Arbitrary Reader cancellation is cooperative between actual reads. Do not abandon a goroutine or claim forced deadline/RSS guarantees. Later outer admission stays occupied until real codec/parser work and spool close finish.

## Review Focus

- Block-aligned maximum plaintext needs a complete extra PKCS7 block without incorrectly exceeding the4MiB limit.
- Invalid padding/truncated or non-aligned ciphertext cannot expose even a partial plaintext handle.
- A writer/read/sync/close/cancellation error cleans only the owned artifact and returns safe constants.
- Source key spellings cannot permit whitespace, partial hex/base64 or unsupported length through permissive decoding.
- Same-path replacement on reopen/cleanup is refused and replacement bytes are retained.

## Task1: Streaming codec and private completed artifacts — Medium

**Files:** create internal/media/{codec.go,spool.go,codec_test.go,spool_test.go}; report docs/superpowers/reports/2026-10-09-private-media-codec.md. No other ownership. Later CDN/library/image-validator modules have separate plans.

**Interfaces:**
- `ParseKey(imageHex,mediaBase64 string)([16]byte,error)` follows source precedence: if imageHex is present it must be exactly32 ASCII hex chars and valid; otherwise standard strict base64 media key decodes to16 raw bytes or32 ASCII hex chars then16bytes. Invalid preferred hex never silently falls back. Reject absent key, whitespace/control bytes/noncanonical or invalid padding and excess input; key material never appears in errors.
- `Decrypt(ctx context.Context,dataDir string,input io.Reader,key [16]byte)(*Artifact,error)` streams ECB into a private spool, holding the final16-byte block until strict PKCS7 verification. Ciphertext actual bounds4MiB+16; actual plaintext<=4MiB. Refuse empty/non-aligned/invalid padding; output is not image/document validation. Return a handle only after successful sync/close/privacy/identity/reopen validation.
- `Encrypt(ctx context.Context,dataDir string,input io.Reader,key [16]byte)(*Artifact,error)` streams actual plaintext<=4MiB with at most one unfinished block, always appends PKCS7 including a whole extra block for aligned input; actual ciphertext<=4MiB+16. Empty plaintext may be encoded by this pure codec; later format validation decides meaningful media. Check bounds before writes, no whole input/output buffer.
- `Artifact` keeps private original identity/path and completed file internally; exposes only `Read`, `ReadAt`, `Size()int64`, `Close()error`. No public path or personal name. Actual Close checks ownership, closes handle and removes only original main; idempotent and concurrency-safe; replaced file remains. The immutable input contract is for later caller, never inferred from a passed arbitrary Reader.
- Sentinel error codes are constant invalid/limit/private/IO, preserving context.Canceled/DeadlineExceeded when cooperative context ends. Narrow private per-call seams may inject failures for tests; no exported runtime mutation hook/global mutable seam.

- [ ] Write runtime RED fixtures before production implementation: TestMediaKeyFormsAndPrecedence, TestMediaECBKnownVectorAndPadding, TestMediaMaximumAlignedRoundTrip, TestMediaRefusesInvalidCipherBeforeExposure, TestMediaIOAndCancellationCleanup, TestArtifactReplacementOwnership. Use generated bounded synthetic bytes and independent block/padding vector, not only round-trip tests. Both size boundary+1 and exact maximum must be checked; replacements before reopen and cleanup must survive unchanged.
- [ ] Observe runtime RED with explicit not-implemented stubs when missing exported API would otherwise only fail compilation. Failure fixtures must terminate and use dedicated private children of t.TempDir.
- [ ] Implement minimal codec/spool with32KiB-or-smaller fixed scratch, file Sync/Close checked, identity/privacy before payload and before publish, owned-only failure cleanup. No image decoding/network/library/database behavior in this task.
- [ ] Run media/privatefs tests and vet with verifiedGo1.27.2/offline caches, report exact pass/fail/skips/runtimeRED→GREEN/maximum streaming allocations/limitations, freeze for fresh independent Medium security/resource review. Root performs full integration/nativeCI/scans before adopting into CDN/channel.

## Root self-review and downstream obligations

Every Review Focus maps to a named fixture. The codec publishes bounded completed bytes only; callers must independently validate images/documents before model use. CDN policy, download lifetime admission, scope/dedup receive metadata, shared DeepSeek vision, user-evidence separation, visible document excerpts, upload/send ambiguity and sticker metadata remain separate tasks. This plan implements none of those and cannot establish GIF/backend/real media or512MB acceptance. Existing spent cloud allowance is not extended. Routine design/plan choices follow owner autonomy; no new actual-operation approval is inferred.
