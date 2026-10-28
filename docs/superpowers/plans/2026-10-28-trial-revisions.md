# Consolidated trial revisions implementation plan

> **For agentic workers:** Use superpowers:subagent-driven-development or executing-plans for the task owned by each worker. Follow TDD for meaningful behavior changes. Owner's explicit start instruction and continuous-work preference authorize the recorded F01–F08 scope; do not add redundant process approvals.

**Goal:** Resolve the actual Chinese recall failure and implement the owner-selected image/auth/management/UI changes as one reviewed candidate.

**Architecture:** Existing native monolith, same SQLite/schema/provider/channel transports; three disjoint implementation domains integrated by primary. Work in existing codex/foundation checkout with explicit file ownership and an immutable public-source baseline so earlier documentation changes are preserved; do not create/move a worktree or copy secrets.

**Tech stack:** Verified Go1.27.3, existing SQLite/HTTP/JS/CSS, pinned dependencies, local Node/browser development tooling only.

**Spec:** ../specs/2026-10-28-trial-revisions-design.md

## Global constraints

- Read AGENTS/REQUIREMENTS/SECURITY/ARCHITECTURE/TASKS/MEMORY/DELIVERY; preserve LICENSE/current docs.
- Children exclusively gpt-6.1-sol/medium, announced with98% before every dispatch/resume; no nested delegation/private/server access.
- No code owner overlap. Primary alone edits progress/recovery/decisions/research/spec/plan records and integrates.
- No external live/provider/server/private-data/Git-publication actions by implementers. All local fixtures synthetic.
- No new deps/schema/resource budgets/retry authority. Existing account+password env, configured model and bounded transport remain.

## Review focus

- Chinese question fragments must recall meaning-bearing facts without generic-word foreign/expired recall; test both actual model context and quotas.
- Model metadata is untrusted; test unknown/overflow ID, malicious advice, malformed/truncated envelope, no candidate, disabled/cross-scope/changed image, failure/ambiguity/cleanup and no extra model call.
- Wrong/missing username must never authenticate or disclose which credential failed; preserve strict JSON/nonces/rate/cookies/CSRF and CLI status.
- Events/defaults/page navigation require auth, strict bounds/safe labels and accurate current-process retention. Verify standalone event login/navigation and latest3 order.
- UI editing/defaults/cancel must not silently persist; unified restore still preserves warnings/pause/error behavior; persona simplification preserves IDs/metadata/UTF8 bounds and stale-editor/privacy clearing.

## Task A — auth and management backend

**Owned files:** internal/config/**, internal/cli/**, internal/web/*.go; .env.example and narrowly related operator credential documentation. No assets/agent/storage.

**Interfaces:** config.AdminUsername/defaultadmin; web.Options.Username; login exactly `{username,password,nonce}`. GET settings/defaults→Settings directly; GET events→items/next_offset/has_more; status.diagnostic_code; GET /events same embedded app entry.

- [x] Write and run semantic failing authentication/default/events/status/page tests; preserve original results.
- [x] Implement contracts and CLI status wiring with minimal scope; update existing synthetic helpers and no-key/private-secret fixtures.
- [x] Run affected config/web/CLI tests+vet; provide exact commands/results/changed files/limits and no secret output.

## Task B — Chinese recall and same-call AI image suggestions

**Owned files:** internal/agent/** only.

**Interfaces:** existing Model.Chat, Options, storage/provider functions unchanged. Local bounded keyword normalizer, metadata catalogue/envelope validation and selection. Normal30s/low180s/same600s; no random or literal-match selection gate.

- [x] Reproduce natural Chinese recall failure through a synthetic actual Agent/model-context test before correction; add mixed/long/injection/scoped/expired/unrelated cases.
- [x] Add failing semantic suggestion tests including one Chat call, catalogue/output budgets, neutral/noimage, malformed/foreign ID, frequency and durable send/refusal lifecycle.
- [x] Implement within owned package, keeping all existing non-image paths and acknowledgements meaningful.
- [x] Run affected agent tests+vet; provide RED/GREEN receipts and unresolved limits. Do not weaken existing assertions simply to make green.

## Task C — glass UI and interaction simplification

**Owned files:** internal/web/assets/index.html, app.js, style.css; scripts/test_web_view.js, test_library_view.js, test_library_bundle_view.js.

**Interfaces:** Task A auth/default/events/diagnostic contract; existing remaining APIs unchanged. Dedicated /events client view and safe route links.

- [x] Add failing JS behavior regressions for login account, edit/save/cancel/default draft, latest3/events route/paging, compact uptime, simplified persona and preserve prior cancellation/restore/privacy/image-preview assertions.
- [x] Build the agreed glass direction, compact copy/navigation, unified backup area and accessible responsive form hierarchy. Main persona name/style, generatedID/advancedfields, exact existing byte bounds.
- [x] Run all three Node regression suites and syntax checks. Return screenshots only if produced in synthetic environment, no browser private-data read/server access.

## Task D — primary integration and review

- [x] Preserve immutable baseline/public identity and verify baseline scope while disjoint workers proceed.
- [x] Read all actual deltas, reconcile interfaces; independently run integrated Go/JS/vet on immutable final candidate.
- [x] Fresh consolidated independent review on final changes; fix findings within owned files and rerun affected checks only as justified.
- [x] Synthetic browser visual/functional checks including auth/event page/model draft/persona/backups; show reviewed result to owner.
- [ ] Reconcile docs/decisions/progress and build source-bound native candidates. Public release stays deferred. Before touching live server, prepare/review exact maintenance/data/credential compatibility and obtain only any genuinely missing risky decision.

2026-10-29: implementation, primary full local/JS/vet/vulnerability checks, independent consolidated review and synthetic browser completed. Native CI/live update/owner acceptance remain separate. Evidence: [local checkpoint](../../research/trial-revisions-local-verification.md). Maintenance [scope](../../operations/trial-revision-update-2026-10-29.md) is prepared/reviewed, not an executed updater.
