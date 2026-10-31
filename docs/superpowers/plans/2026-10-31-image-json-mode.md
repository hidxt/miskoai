# Image JSON-mode implementation plan

> **For agentic workers:** Use superpowers:executing-plans inline; one concentrated fresh GPT-6.1 Sol Medium read-only review of the frozen candidate in parallel with full affected checks, after targeted RED/GREEN. No mutation during review/tests; resolve concrete findings before final broad verification. This scheduling adjustment reduces elapsed time without weakening gates.

**Goal:** Enforce JSON syntax on the single existing eligible image-advice reply request without changing request count or send authority.

**Architecture:** Optional `ChatJSON(context.Context, []provider.Message, int)` capability, shared DeepSeek request/response path and existing transport; a counted Core adapter forwards the capability once; Agent selects it only after the captured-catalogue branch. Legacy Model.Chat and all noneligible paths remain compatible.

**Tech stack:** Existing Go1.27.3/stdlib; no dependency or tool installation.

**Spec:** ../specs/2026-10-31-image-json-mode-design.md. Owner explicitly approved the provider parameter change for local implementation/synthetic verification only. No new real calls, publication or deployment.

## Constraints and owned files

Primary owns internal/provider/deepseek.go +json_mode_test.go, internal/core/metrics.go +json_mode_test.go, internal/agent/agent.go,image_advice.go +json_mode_test.go, this plan/spec and phase documents. Work first in an ignored public-source copy .tools/reviews/oct31-json-mode/src; preserve existing docs and server bf521038. No concurrent file writers.

Keep model/endpoint, one logical reply attempt, shared admission,60s provider deadline, existing explicit429/503-only max3HTTP attempts, context/output limits, parser/cooldowns/once-only text+image ACK/replay rules. Whitespace or unsupported format never triggers a second plain-text/paid fallback. No UI, SQLite or live credentials change. A caller's prompt text cannot enable provider JSON mode.

## Steps

- [x] Freeze baseline public source and hash Go tool; record local-only owner approval.
- [x] RED: mocked actual HTTP verifies missing explicit JSON capability/field; ordinary Chat/Stream/vision retain default format; errors/blanks/cancellation preserve no-extra-call behavior. Agent tests require JSON capability only for eligible captured advice and a real parseable string-field example; Core tests prove forwarding/accounting and actual borrowed-image two-ACK path.
- [x] GREEN: implement DeepSeek.ChatJSON via a shared private chat method; optional Agent JSONModel, trusted selection at existing structured branch; Core counted forwarding without double-counting; compact example included in existing reservation budget.
- [x] Run affected full provider/agent/core suites and vet; concentrate one fresh read-only review in parallel, then resolve concrete findings before final broad verification. Review0 actionable C/I/M; no corrective mutation. Reuse unchanged UI evidence; no new Node/runtime access.
- [x] Full local suite completed with retained failures/timeouts; one actual-failure-driven serial supplement completed, retaining the remaining hardlink setup failure. No clean full-suite claim, reused native-proof authorization or waived assertion. Three native candidates built/header/hash checked,7 owned files integrated exactly,266 input identities checked; phase documents updated. Final public delta is sealed after its documentation/secret checks. New exact-source CI/live verification remain pending a later concrete authorization.

## Review focus

Trusted mode selection versus untrusted prompt; summary/stream/vision/document/command/search behavior; zero second call after JSON error or blank output; precise one-attempt/usage counting; shared transport/admission/cancellation; invalid/truncated/unknown image advice still never grants image authority. JSON syntax does not ensure semantic image suitability.
