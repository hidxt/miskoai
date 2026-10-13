# DOCX declaration regression implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development or executing-plans. Root reviews/integrates; continuous routine fixes are owner-authorized.

**Goal:** Resolve the explicitly retained D062 declaration Minor before final43/45 review.

**Architecture:** Extend the existing bounded lexical XML guard used by all three fixed DOCX parts. Validate an optional XML declaration before releasing it to encoding/xml, with no new parser pool or resource bounds. Ordinary processing instructions remain inert.

**Tech Stack:** Native Go1.27.2 standard library, existing guarded DOCX fixtures.

**Spec:** D058/D062/D063 in DECISIONS.md; docs/research/docx-extraction-root-verification.md and existing DOCX extraction plan/report.

## Global constraints

- Children only gpt-6.1-sol/medium; all token/tag/depth/ZIP/output/work/time caps and source attribution stay intact. No provider/network/private data/deployment changes.
- Optional initial UTF8 BOM remains accepted. XML declaration must be the first lexical token after BOM, at most once, with target exactly lowercase `xml`. Reserved case variants of that exact target refuse; ordinary `xml-stylesheet` remains an ordinary bounded PI.
- Require `version` first with quoted `1.0`; optionally `encoding` next with quoted case-insensitive `UTF-8`; optionally `standalone` last with quoted `yes` or `no`. Exact XML whitespace/equals/paired quote grammar; no duplicate/unknown attributes, reordered fields, missing version, extra data, entity references or misplaced declarations. Unsupported version/encoding returns the existing safe unsupported class; malformed syntax/placement returns existing safe invalid class and zero extraction result.
- No regex backreference, unbounded scan/allocation or new dependency. Use the existing admitted bounded PI token and a small deterministic ASCII scanner. Input reader/Parser lifetime stays synchronous and cancellation-aware.

## Review focus

1. Empty, duplicate, nested, trailing or whitespace/comment-prefixed declaration refuses before text extraction succeeds.
2. Reserved XML target casing cannot bypass declaration policy; legitimate inert nonreserved PIs remain usable.
3. Quote/equals/attribute-order/version/encoding/standalone mutations produce fixed safe errors and zero partial result.
4. One-byte fragmented reads and BOM boundaries behave like contiguous input under current limits.
5. Content-types/relationships/main-story all use the same guard; preserving only main-story tests is insufficient.

### Task: Guard declaration grammar and placement

**Owned five files (FIX1 extension):** modify internal/document/xml_guard.go and existing internal/document/docx.go safeXMLToken; create internal/document/xml_declaration.go, xml_declaration_test.go and docs/superpowers/reports/2026-10-13-docx-declaration.md. Other files/rootdocs/Git read-only; existing tests remain byte-exact. Own ignored .tools/reviews/docx-declaration-*.

- [ ] Reproduce meaningful compiled RED for the retained malformed/duplicate/misplaced/casing cases using real existing DOCX parser fixtures and the lexical guard; preserve original source/commands/results. Tests assert safe class and zero Result on refusal, plus supported declaration/no-declaration control.
- [ ] Implement the bounded declaration scanner and first-token state in owned files. Do not change any accepted output/capability or swallow other guard errors.
- [ ] Test all five review-focus cases, all three fixed parts, exact allowed whitespace/quote/order controls, cancellation, hostile raw canaries and one-byte fragmentation; use real synthetic ZIP/DOCX where parser-result claims are made.
- [ ] Run finite30s inventory/disjoint affected document tests and scoped vet; no unchanged whole-root repetition or weakened existing assertion. Freeze four owned files and actual/copied evidence identities, self-review and return ready-for-adoption. Main adoption only after root clearance.

Self-review: only an explicitly tracked grammar defect changes; declaration support and existing guard limits remain exact, every review-focus case has a test. Root fresh independent review and whole-product/native gates are still required before marking D062 resolved; this task alone earns no final43/45 completion credit.

FIX1 ruling2026-10-13: root finite proof and fresh review M1 show whitespace-only lexical target scanning misses actual reserved XML Names ending at `/` or `=`. Check the actual encoding/xml ProcInst.Target at the existing safe token boundary as a bounded fallback, reusing the same strict declaration-body scanner and safe class mapping. Existing lexical validation still enforces normal declaration first position; malformed delimiter bodies refuse before any usable extraction token. Do not copy/redefine Unicode Name grammar or add another XML parser. Add compiled RED for <?xml/?>, <?xml=?>, <?XML/?> in all3fixedparts, with ordinary xml:/xml-stylesheet/Unicode PI success controls. Original4-file freeze/review and initial root diagnostic are retained; root diagnostic's xml: refusal assumption is invalid and excluded from the finding. Five-file replacement requires root clearance and fresh scoped re-review.
