# Finite WeChat image capability check implementation plan

> **For agentic workers:** Use superpowers:subagent-driven-development for isolated implementation and a fresh review. Existing owner instructions authorize routine preparation; live execution requires the concrete additional approval described below.

**Goal:** Build an ignored, once-only synthetic verification tool for actual static PNG preparation/upload and text-then-image strict acknowledgements, without opening or migrating the owner's legacy database.

**Architecture:** Reuse the reviewed Weixin Client, privatefs, lifecycle lock, image processor, library Manager, replyimage Preparer and schema6 receipt APIs. A separate owner-private synthetic directory and pre-network exclusive attempt latch prevent reruns; the owner's existing authorization is read only in explicitly approved live mode. This tests wire capability, not the random selector, normal private service or native stickers.

**Tech stack:** Existing native Go1.27.2 and dependencies only. No shell/model subprocess, new runtime, production source changes or provider calls.

**Spec:** REQUIREMENTS.md/SECURITY.md; approved D082 text then prepared image; docs/research/weixin-live-marker-root-verification.md; current storage/image_replies.go and replyimage/prepare.go contracts.

## Global constraints

- Engineering78%; live capability and package40 remain unaccepted until actual results and peer confirmation. No new live allowance is inferred from implementation authorization.
- ONLY gpt-6.1-sol/medium for Codex children. Agy is excluded from this backend/authorization task.
- Never read/open/migrate the owner's `miskoai.db`, WAL, SHM or ordinary chat. Never start Core.Run.
- No paid model/search requests. Static generated32x32 PNG, fixed text reply and fixed fresh marker only.
- Live network maximum: one GetUpdates, one GetImageUploadURL, one CDN Upload, one SendText, one SendImage; zero retries. Prepare before either send; text ACK durably precedes image attempt.
- Any uncertain send/persistence/cleanup stops, retains partial acknowledged state, and never restarts/resumes/replays. Preserve synthetic evidence and attempt latch.
- Credential/token/context/upload parameters/IDs/recipient/media/key bytes never enter logs, output, process arguments or development memory. Only fixed codes and attempt/ACK/persistence/cleanup booleans may be reported.
- Existing endpoint/SSRF/TLS/permission, size, cancellation and30s transfer policies remain. Total run context90s; state-only failure recording bounded2s; actual Close joins before successful exit.

## Review focus

1. Duplicate process/restart/crash must stop before another poll, upload or send: an exclusive durable pre-network latch in the fixed synthetic directory is mandatory.
2. Marker admits only the previously authorized direct user/account, sole completed text, exact marker, allowed state/recipient, nonempty context and no same-item alternate payload, using the previous strict marker guard.
3. Prepared image failure, text unknown or receipt persistence failure must prevent later parts; image unknown must preserve text ACK and prohibit retries.
4. No ignored Close errors or early successful result; temporary bytes are synthetic and actual cleanup must join.
5. Real dependencies must be bound only by explicit live composition; ordinary/default/test modes must be incapable of accidentally reading authorization or networking.

## Task1: Isolated finite tool and synthetic tests

**Owned files:** In NEW `.tools/reviews/weixin-image-capability-oct19-src`, only `cmd/weixin-image-check/main.go`, `cmd/weixin-image-check/main_test.go` and `docs/superpowers/reports/2026-10-19-weixin-image-check.md`. Base is immutable `image-serial-root-oct19-src501`; do not change main or existing dependency snapshots. Frozen source/owned/evidence manifests and patch live under ignored `weixin-image-capability-oct19-*`.

**Interfaces:** Default invocation prints fixed usage and performs no I/O. `preflight` performs bounded read-only authorization/permission checks only when later authorized; `live` requires the explicit owner action. Tests use private per-instance dependency injection, never exported production hooks. Live uses existing public package APIs; actual synthetic Store lives in a new fixed private sibling test directory, never the legacy DB. Marker is `MiskoAI synthetic image capability test 20261019`; reply is `MiskoAI image capability test passed.` No marker received means no sends. Explicit manual capability choice is clearly separate from Agent frequency selection.

- [ ] Read prior strict-marker implementation and accepted APIs completely. Freeze dependency identities; reproduce meaningful failing tests for once-only latch, envelope guard, independent ACK transitions and cleanup.
- [ ] Implement bounded instance-local probe plus real constructor/composition. Reuse actual Manager validation/Store receipts/Preparer; genuine generated PNG must pass the actual processor. The existing CDN has no public transport seam: live construction must always bind real replyimage.New, while command tests may use a private per-instance preparation function plus separately test actual media.Encrypt. Existing package-local Preparer tests supply independent crypto/upload-boundary evidence; do not claim the command exercised the real CDN. No production hook, reflection, alternative crypto, scanner suppression, optional private DB access or manual unvalidated artifact.
- [ ] Cover admission refusal, duplicate marker/process/latch, auth expiry, upload failure, text unknown, text ACK persistence failure, image unknown, both ACK, canceled/failed cleanup, no retry after every failure and default mode zero effects. Inspect actual per-boundary call counters and receipt phases; no assertion mirroring unsupported behavior.
- [ ] Run narrow RED/GREEN/vet/build after primary signals no competing full Go suite. Never start more Go while current primary serial regression is active. Offline normal-owner synthetic tests only; no live execution or authorization read.
- [ ] Freeze exact source/patch/manifests, commands/actual exits and limitations. Primary reads/retests and requests fresh permitted-model review. Keep code isolated; no commit/push/release.

## Task2: Primary acceptance and explicit live authorization

- [ ] Primary verifies complete implementation, finite counters, private data boundaries, shutdown, receipt/latch durability and all identities; reruns synthetic tests/vet/build and obtains fresh review.
- [ ] Present exact reviewed binary hash, five-request cap, no retries, separate synthetic state and two-message expected result to owner. Request new limited live authorization and fresh marker; wait for actual answer/action before any authorization preflight or live request.
- [ ] After approved actual run, read fixed result/exit and ask owner to confirm new text plus PNG receipt. Unknown outcomes remain unknown; never rerun or infer delivery. Record actual evidence scope without claiming selector, GIF/native sticker, reconnect/auth-expiry recovery, server/resource or final acceptance.
