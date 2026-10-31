# Model failure diagnostics implementation plan

> **For agentic workers:** Use superpowers:executing-plans inline, with one concentrated fresh-context review at the end. Steps use checkbox syntax for tracking.

**Goal:** Preserve safe finite model error categories from originating branches to authenticated dashboard diagnostics.

**Architecture:** Add typed safe failures in netx, retain error identity through Unwrap, classify explicit HTTP/context/limit types without string matching. Ordinary, JSON, vision and document model-call failures feed a per-Handle diagnostic value; Service/Web normalize it independently. No request behavior or durable schema changes.

**Tech stack:** Existing native Go1.27.3, embedded vanilla JS, current module dependencies only.

**Spec:** [approved local design](../specs/2026-10-31-model-failure-diagnostics.md).

## Constraints and rulings

Owner approved local-only implementation/synthetic tests. No push/commit, live calls, SSH/key use, deployment, new tool/dependency or external path. Preserve all request/timeout/retry/send/image policies. Preserve existing mixed Windows results and earlier successful native CI separately.

Ruling: use a fresh ignored public-source copy under .tools/reviews/oct31-model-failure/src, consistent with preceding isolated work; no Git metadata mutation or additional worktree approval is needed. Preserve baseline manifests; integrate only inspected owned deltas after checks. Existing project toolchain/caches stay inside the project. Keep prior evidence, not cleanup deletion.

## Task 1: typed classification and provider origins

Owned: internal/netx/failure.go and failure_test.go; internal/netx/client.go; internal/provider/deepseek.go and failure_test.go.

Interfaces: netx.WithFailure(code string, cause error) error; netx.FailureCode(error) string; netx.NormalizeFailureCode(string) string. Error() returns fixed safe text, Unwrap preserves cause; nil remains nil. Unknown classifications never expose arbitrary error text. HTTP statuses use explicit finite mapping, context and ErrLimit retain identity.

- [x] Baseline existing JSON-mode tests; freeze public-source hashes.
- [x] RED typed-context/status/canary and origin/unchanged-request-attempt fixtures.
- [x] Implement minimal safe categories at exact request/transport/response error branches; GREEN.

## Task 2: per-result propagation and authenticated display

Owned: internal/agent/agent.go, attachment.go, document.go, model_failure.go/test.go; internal/service/status.go and model_failure_test.go; internal/web/system.go, model_failure_test.go, assets/app.js.

Interfaces: agent.Result.ModelFailure; service.Status.LastModelFailure; channelDTO.model_failure. Preserve existing internal answer callers with optional per-call diagnostic pointer. Per-handle local value, no shared Agent field or background work. Only actual model-call errors set it. Success, duplicate, command or other nonmodel result clears latest category. UI appends one textContent-based diagnostic row; no HTML from errors.

- [x] RED fallback/vision/document/duplicate-reset/counter/authentication/enum/canary tests.
- [x] Implement result forwarding and normalized UI; GREEN.
- [x] Verify existing JSON-mode, cooldown, dedup, attachment and management regression cases.

## Review focus

- Wrapped/injected error chains must not stringify raw errors or allow arbitrary categories.
- Cancellation/timeout/HTTP status classifications must not alter retry admission or number of requests.
- Vision/document callback cleanup errors must not be confused with model-call failure.
- Nonmodel/duplicate/success observations clear prior failure and preserve counters and ACK semantics.
- Both status and diagnostics authenticated endpoints normalize invalid/injected category values; UI stays escaped.

## Task 3: integrated local gate and evidence

- [x] Run focused affected packages, full module test command once, full vet and native builds; report actual Windows failures by name without erasing old evidence.
- [x] Announce and dispatch one GPT-6.1 Sol Medium reviewer with exact owned paths/design/test receipts; primary resolves actionable findings and reads the final diff.
- [x] Integrate inspected changes with unchanged baseline guards; primary reruns integrated targeted checks and updates phase docs/checkpoint.
- [x] Public policy/diff/secret checks; no commit/push/CI or deployment implied. Final/live/root-cause acceptance remains open.

Rulings/evidence2026-10-31: baseline32PASS; first RED missing APIs plus2 status assertions, GREEN66PASS. Concentrated review0C/1I/0M found lost body-read context identity; Full1 interrupted, not accepted. Correction RED66PASS/5FAIL, GREEN71PASS, final targeted75PASS. Preserve only known context sentinels at transport origins so synthetic nested HTTP errors cannot expand retries. Bounded I1 verification0 unresolvedC/I/M. Primary adopted15 reviewed files before full completion to use waiting time; baseline270 unchanged, all277 final inputs byte-identical. Final immutable full/vet/build/integrated gate remains pending; see [local verification](../../research/model-failure-diagnostics-local-verification.md).

Final gate supersedes that pending state: full3434PASS/10FAIL/8SKIP, no timeout/build failure; all10failures are6 authentic hardlink setup denials plus4parents, with all8 skip reasons retained. Vet0,3builds0, PE/ELF/architecture/hash checks pass, integrated root75PASS; all277 inputs still match. Full Windows remains mixed, new native CI/live/update/final acceptance remain outside the approved local scope.

Final scanning: public570-file policy0 findings, frozen-source Gitleaks0, diff check0, private evidence/binaries/source-copy ignore coverage verified, staged files0. No commit/push/deployment performed.
