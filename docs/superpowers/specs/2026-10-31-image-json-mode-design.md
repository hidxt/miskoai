# Image advice JSON-mode correction proposal

Status: **owner approved local implementation/synthetic verification on2026-10-31; public push/deployment and real calls remain unapproved for this correction**. Engineering98%. Diagnostic source bf521038/nativeCI41115905560/all12 passed and the fixed diagnostic trial update succeeded. One fresh owner-sent joy-context test then returned text only; authenticated status recorded `plain_text`,1 received/1 processed/1 sent,0 failed/ambiguous and1 logical chat attempt. No chat body or raw model output was logged.

## Evidence and approaches

`plain_text` is assigned after the eligible, captured-catalogue structured branch parses an accepted legacy plaintext response. Thus this request reached image-advice generation, but the application received no structured image ID; image selection/upload/send was not entered. The current DeepSeek request body has no `response_format` parameter. This identifies an unenforced reply-format contract, not a CDN failure or proof that an image must be selected.

Official [Chat Completions](https://api-docs.deepseek.com/api/create-chat-completion/) documents plain-text output when that parameter is omitted. Official [JSON Output](https://api-docs.deepseek.com/guides/json_mode/) requires `response_format: {"type":"json_object"}`, the word JSON and a format example in the prompt; its sample uses deepseek-flash. It also documents possible whitespace-only content. Existing rejection of empty/invalid output must remain; its retry suggestion does not override this project's bounded retry rules.

Primary recommends explicit JSON mode only for the existing eligible image-advice reply request. Adding examples alone is a softer alternative, since the current text instruction was already not followed. Do not switch endpoint/model/provider, adopt a new tool-calling protocol or require images on every reply.

## Proposed wire and application change

- Add exactly `"response_format":{"type":"json_object"}` to that existing nonstream reply request. Same endpoint, deepseek-flash, model admission/transport, one logical attempt, original timeouts/retries/context/output bounds. No separate emotion request.
- Preserve Model.Chat compatibility through an explicit optional JSON capability. DeepSeek implements it using the existing request/response/error path; the Core counted wrapper preserves that capability and counts once. Agent chooses it only after its trusted eligible/captured-catalogue branch, never by scanning untrusted prompt text. Legacy injected models can retain current behavior.
- Add a compact JSON format example with reply/emotion/image_id strings, while allowing empty image_id. A suggestion must still resolve to the exact offered same-scope static-image metadata; model output never grants authority.
- Ordinary chat with images disabled/no catalogue, commands/search, summaries, streaming, vision and documents retain their current paths. Whitespace, unsupported response-format errors or malformed/truncated output do not trigger a second paid call or fallback provider request. No raw protocol leakage, automatic resend or relaxed parser/selection/ACK rule.

## Local verification after approval

Use only public synthetic inputs and existing project tools. Capture the actual mocked HTTP body to prove JSON mode is explicit only for this capability; prove ordinary/stream/vision request behavior is unchanged. Test blank/malformed/length output and existing bounded429/503 handling. Exercise the actual Agent/Core wrapper to prove JSON capability survives counting, exactly one underlying call occurs, and disabled/search/document/image contexts do not acquire it. Retain strict image-ID, cooldown, once-only receipt and no-replay cases.

No new dependency/tool, SQLite/schema/resource change or real provider call is needed for local work. Any subsequent source publication/test-server replacement uses a new concrete reviewed candidate and authorization; the completed bf521038 once-only update is not reused. JSON syntax enforcement does not guarantee semantic correctness or an image on every message; real natural-image acceptance remains open.

## Authorization boundary

AGENTS.md requires explicit owner approval for scope changes involving the provider. The owner approved this exact JSON parameter/example correction for local implementation and synthetic verification on2026-10-31. This satisfies the provider-scope gate; no additional local permission is needed. New real calls, public push and deployment are outside that approval. The existing diagnostic trial keeps running unchanged.
