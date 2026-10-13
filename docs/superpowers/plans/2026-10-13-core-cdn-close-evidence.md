# Core CDN close evidence implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: executing-plans. Root implements the small test correction; fresh permitted Medium review follows in the combined regression package.

**Goal:** Resolve CoreM1's weak assertion by observing the owned CDN's specific closed state.

**Architecture:** Before Core.Close, an empty-query Download must return media.ErrInvalid with no network call. After the actual joined Core.Close, the same operation must return media.ErrClosed. Constructor unwind remains source evidence only; no production seam is added.

**Tech Stack:** Existing native Go1.27.2 synthetic Core fixture/CDN guards.

**Spec:** D065 and docs/research/core-image-root-verification.md, retained CoreM1 final43/45.

## Global constraints

- Modify only internal/core/attachment_test.go and create docs/superpowers/reports/2026-10-13-core-cdn-close.md; existing production/source policies/dependencies stay exact.
- No live CDN/provider/WeChat call. Empty query fails URL admission before transport; assert the exact fixed class rather than any error.
- Keep joined worker lifetime and finite focused tests; no weaker assertion, retry or sleep. Actual private data is unused.

## Review focus

1. Removing owned CDN.Close must fail the stronger assertion while the original any-error assertion still passes.
2. A client that is already closed before the fixture's Core.Close must fail the negative control.
3. A successful check demonstrates owned CDN closure, not constructor unwind, actual network cancellation or512MB acceptance.

- [ ] Copy accepted355 source to isolated own-prefix tree; preserve original test and a test-only mutant that omits owned CDN closure.
- [ ] Run original test on mutant: expected PASS, demonstrating the old assertion gap. Strengthen pre/post exact classes; same mutant must FAIL with fixed unclosed-state error, never a network call.
- [ ] Run corrected focused real-source test and scoped Core vet; production bytes remain exact. Freeze corrected owned/evidence identities, retain original mutant results, self-review and prepare fresh review/root integration.

Self-review: a test-only observation gap is corrected with a real specific state distinction; no application behavior changes and no final43/45 completion credit yet.
