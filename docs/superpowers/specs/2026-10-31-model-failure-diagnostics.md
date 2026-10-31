# Model-failure diagnostics: proposed local-only design

Engineering **98%**. Status: owner explicitly approved local diagnostic implementation and synthetic verification on2026-10-31. No public Git action, real provider/WeChat call or server change is included. Historical cause remains unknown.

## Verified problem and limits

On deployed eec21cd the owner confirmed a normal text reply plus a cat image, and subsequently confirmed that a later message received the fixed unavailable-model notice. Those are distinct peer outcomes. A separately authorized six-request metadata snapshot shows receiving, received/processed/sent3, failed/ambiguous0, logical chat3, latest model_error/model. It identifies neither an HTTP status nor the exact failed turn.

Local trace: DeepSeek request preflight, network/HTTP errors, invalid JSON, empty choices/content, unsuccessful finish reason and payload limits may all return errors. Agent's selected model call maps any error to model_error and one safe fixed reply; Service counts that successfully delivered reply as sent. Existing records cannot distinguish those causes retrospectively. This is an observability gap, not proof of any particular provider/network/JSON defect. No retry or guessed repair is justified.

## Proposed approach

Preserve a typed, finite failure category at the provider/transport boundary, then forward only that safe category through Agent Result, Service's process-local status, and the authenticated management DTO. Keep image_decision=model_error and existing counter semantics; add a separate latest model-failure category. Reset it on the next model-success/non-applicable result so old failures are not misrepresented as current. Unknown or injected errors map to unknown without inspecting or printing error strings.

Proposed finite categories: request_invalid, authentication, balance, rate_limit, unavailable, timeout, canceled, transport, payload_limit, response_invalid, response_empty, response_finish and unknown. Map only typed errors or the explicit originating branch; do not match arbitrary error text. Retain useful original error identity with safe wrapping where necessary. No raw HTTP body, URL, model output, prompt, credentials, recipient, message ID or chat text enters the status or log.

Prefer this approach over raw exception logging because the latter violates existing privacy policy. Repeated live requests with the present generic diagnostic cannot reliably identify the cause and may incur cost. The owner approved this finite-category design for local implementation and synthetic verification; that decision does not authorize live requests, public Git actions or deployment.

## Scope and invariants

- Local implementation and synthetic fixtures only, using the existing project Go toolchain and dependencies.
- Potential files: internal/netx/client.go plus a focused error-definition file; internal/provider/deepseek.go plus a focused classifier; internal/agent/agent.go; internal/service/status.go; internal/web/system.go and focused tests. Final file ownership is fixed in the implementation plan after approval.
- Preserve deepseek-flash, endpoint, JSON mode, prompts, context/output limits, timeouts, concurrency, one logical call, existing 429/503-only retry ceiling, fallback behavior, image selection/cooldowns, durable sending/ACK/dedup and schema.
- No new dependency/tool, database migration, external path/key use, public Git action, live model/WeChat test or deployment. Any later update or paid diagnostic needs a separate concrete scope.

## Acceptance for the local diagnostic change

Use synthetic transport errors and responses to test each classified branch, including wrapped cancellation/deadline, explicit 400/401/402/403/422/429/5xx, missing/empty content, response limits and invalid JSON/finish reason. Assert unchanged request payloads and HTTP attempt counts. Unknown/injected errors containing private canaries must reveal neither strings nor payloads. Cover Agent fallback delivery, result reset, latest-status forwarding, counter semantics, authenticated-only access and fixed-enum normalization. Keep existing JSON/cooldown/duplicate-send regressions.

One concentrated review and relevant integrated checks follow implementation. Retain existing mixed Windows/full-native CI evidence separately; no new live/root-cause/final acceptance claim follows from synthetic classification tests. A future live observation can classify a new failure only after separately approved deployment; it cannot reconstruct the already-lost cause.
