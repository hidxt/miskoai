# Image decision diagnostics implementation plan

> **For agentic workers:** Use superpowers:executing-plans inline; one concentrated fresh review after integrated checks.

**Goal:** Locate the missing optional-image stage during the owner's real trial without recording model output or changing image behavior.

**Architecture:** Add one bounded process-local `ImageDecision` observation to the Agent result, Service status and authenticated management status. Display it only inside the existing expanded diagnostic details. Existing claims, sends, ACKs, provider calls, image selection, cooldowns and SQLite schema remain unchanged.

**Tech stack:** Existing Go monolith and embedded vanilla JavaScript; no dependency.

## Design and alternatives

The owner reported text-only replies for both generated-image test contexts; UI shows both assets ready/enabled/unused, receiving4/processed4/sent4, failures0/ambiguous0 and no recent channel error. This is an unpassed natural-image trial, not proof that the model chose no image. The current implementation has multiple silent optional-image exits.

Primary chooses fixed reason codes under the owner's autonomous routine-work instruction. Capturing raw model output would expose chat data; forcing an image would invalidate the natural-behavior test. Neither is needed. Codes identify unavailable capability, profile/off preference, empty catalogue, context admission, model/output rejection, plaintext response, no suggestion, invalid advice, length-limited response, unknown offered ID, cooldown, selection refusal and image pipeline/ACK outcomes. Only the latest handled result is retained in RAM. No message IDs, image IDs, paths, emotion strings, prompt/reply text or credentials enter this observation. Unknown injected values are reduced to `unknown` before status serialization. A restart clears it; historical decisions cannot be reconstructed.

## Global constraints

- Engineering98%; implementation/local/native CI/live/final evidence remain separate.
- Isolated ignored source copy under `.tools/reviews/oct30-image-diagnostics/src`; primary owns changes, then explicitly integrates reviewed files. Preserve existing dirty documents.
- No remote update, paid probe, WeChat send/replay, credential access, schema change, deployment or publication by this plan. Any next server update requires a concrete owner decision.
- Observation must never grant or suppress send authority or alter error/delivery counters.

## Tasks and evidence

- [x] Freeze public source; preserve baseline and run existing affected synthetic tests.
- [x] Add failing tests: actual Agent Handle separates plaintext/no-suggestion/invalid-advice/selected/ACK/cooldown and no-catalogue paths without changing reply count; Service and authenticated status reject a secret canary in this field.
- [x] Add `Result.ImageDecision` and fixed classifications around existing branches; propagate sanitized last result to status. Keep parser admission identical.
- [x] Display the last code in expanded diagnostics; add a DOM behavior assertion for visibility and safe text rendering.
- [x] Run affected RED/GREEN, full native Windows tests/vet, existing Node view tests and cross-build. Retain initial cache-path setup failure separately from product failures.
- [x] One fresh GPT-6.1 Sol Medium review of the whole patch, primary diff review, integrate explicitly owned files, rerun affected checks. Update TASKS/MEMORY/DELIVERY and record exact results and unresolved live gate.

## Review focus

Preserve every original parse/selection admission and exact send count; no permissive numeric IDs or replay. Clear rather than retain stale decisions after duplicate/command/attachment results. Revalidate untrusted codes in HTTP output even if Service was bypassed by a backend fixture. A selected candidate does not imply send or ACK. No raw-model diagnostic logging. The live text-only cause remains unknown until an instrumented candidate is separately approved and observed.

2026-10-31 completion: local diagnostic plan gates complete with retained sandbox3281PASS/8FAIL/8SKIP plus separately owner-authorized normal-owner24PASS/0FAIL/0SKIP exact supplement, covering8 failed events and4 extra unavailable gates.4 platform/privilege skips remain; no single clean full-run claim. New nativeCI/live update/owner image acceptance are outside this completed local plan and require the prepared separate scope.
