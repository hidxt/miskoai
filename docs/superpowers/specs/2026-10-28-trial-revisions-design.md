# MiskoAI consolidated trial revisions

Owner instructed “先修复这些，开始修复” after supplying F01–F08. This revokes the record-only implementation hold for those requirements. Engineering remains98%=49/50 at prior gates; revised behavior has no test/native/live/final credit yet. Public release remains deferred. Routine details below are primary design choices under the authorized task, not invented owner quotations.

## Scope and invariants

One existing native Go/SQLite process and embedded HTML/CSS/JS; Go1.27.3 baseline, no new dependencies/runtime/schema/index, no local model/vector database. Keep current settings limits, per-user scope, plaintext-private credentials boundary, exact Host/Origin/CSRF, limits, atomic durable sends/ambiguity refusal and restore pause. Do not touch real credentials/chat stores, original backups or server until a reviewed candidate and concrete maintenance steps exist. No automatic new paid/live test, Git publication or release.

## F01: bounded Chinese recall

Agent factQuery expands unspaced Han text into bounded deduplicated Chinese keyword n-grams while preserving ordinary Latin/numeric terms. Retain32 terms/1024UTF8-byte limits and existing scoped, escaped FTS/keyword SQL. Suppress obvious question-only Han fragments to avoid retrieving unrelated facts from generic pronouns/question words. Do not add a semantic provider call, dictionary dependency or send all facts. Prove the reported natural question retrieves the saved fact into the actual model context; test unrelated/foreign/expired facts and hostile/long/mixed input with synthetic stores. Same deterministic retrieval path applies to document/image captions where factQuery is reused.

## F02: contextual static-image suggestions

For eligible ordinary text chat with an active custom persona and available enabled static images, use the same existing model.Chat call for a bounded reply plus emotion/image suggestion. Supply only a bounded catalogue of same-scope enabled PNG/JPEG metadata (at most16 entries, total at most4096UTF8 bytes, no binary/paths). Suggested image IDs are decimal strings and must resolve to an actual candidate from the captured catalogue; model output grants no authority. Context and output admission must account for the catalogue/instructions; retain existing context/output budgets and safe truncation behavior.

No independent emotion API request. Model may choose no image or unknown/neutral emotion. Invalid image advice falls back to text when a safe bounded reply exists; malformed envelopes must never leak protocol JSON as chat text or bypass output validation. Preserve plain-text paths when image selection is disabled/no candidates; legacy model/test callers need not change their Model interface. Schema/parser details are implemented within internal/agent and covered by tests.

Remove random/literal-dual-keyword gates for accepted semantic suggestions. Normal minimum interval30s, low180s, same image600s, off never sends. Keep text followed by at most one image, all immutable-byte checks and independent once-only persisted acknowledgements; no automatic replay/restart continuation. Commands/search/documents/images keep their existing eligibility unless explicitly ordinary text. Images remain static PNG/JPEG only. No storage schema/meaning change to prior send history.

## F03: one owner management account

Add Config.AdminUsername (private, json:"-") from MISKOAI_ADMIN_USERNAME, default `admin` when absent so old private installation can upgrade without inventing/changing a password. Bound nonblank username to64UTF8 bytes, reject controls/edge whitespace; validate only as an owner identifier, no multi-user/roles/registration. Web Options gains Username, empty means admin for compatible constructors; Server stores/computes fixed hashes and compares BOTH username/password without early credential-specific errors. POST /api/login requires exactly username/password/nonce; preserve nonce consumption, rate/JSON/session/CSRF rules. CLI serve and status pass the effective username; update synthetic login helpers and meaningful negative cases. Existing private password remains unchanged; `.env.example`/operator docs explain username. No credential reset via browser.

## F04–F08: management contracts and visual design

- Preserve all existing nonsecret settings APIs. GET /api/settings/defaults returns config.DefaultSettings() directly; it is authenticated, read-only and never resets credentials/data. UI reset fills an editing draft only; explicit Save persists through current save/restart semantics. Environment overrides remain visible on demand.
- GET /api/events?offset=0&limit=20 returns `{items:[{kind,at}],next_offset,has_more}` newest first from the bounded safe Status events. Max limit32, max retained128; strict query parsing and normal authentication. This is a current-process event history, not indefinite durable audit logging.
- `/events` serves the same embedded entry document, with a dedicated event-page view, back navigation and paging. Login and app resources work there with same origin. Dashboard shows latest3 events, “加载更多” navigates to `/events`. No caller-controlled URLs/routing/files.
- Status DTO gains diagnostic_code, derived from actual channel/state/error categories, e.g. receiving/backoff/auth-expired/unconfigured/restore-paused/rate/transport/model/storage/send-ambiguous. Do not infer unobserved timeout/network subtypes or print raw errors/secrets. Keep existing state/last_code for compatibility; new UI renders concise Chinese labels and expandable codes/details.
- Uptime is hours+minutes. Compact channel/summary and necessary counters; remove repetitive explanatory labels/ALL-CAPS eyebrows and secret-presence disclaimers. Preserve meaningful operator confirmations and truthful RSS versus heap distinction.
- System uses one Backup & Restore workspace; clear choice between full bundle and database-only while retaining both formats/confirmation/pause behavior. No restore operation is executed by this implementation task.
- Model fields default readonly, explicit Edit, Save, Cancel, Restore defaults (draft). Restore defaults does not silently change live config.
- Rename expression profiles to 人物设定, remove built-in options from user-facing choices without deleting stored history or the internal safe fallback. Main persona editor has name + concise description/style; generate ID internally, preserve existing IDs/fields when editing. Address/length/image preference/humor are optional advanced settings. Enforce existing UTF8 byte bounds, including512-byte style, with user-friendly validation; do not silently enlarge budgets or truncate user input.
- Glass visual direction: quiet cool-white/blue-green palette, legible dark ink, translucent layered navigation/panels, restrained blur/borders, varied section hierarchy and compact useful content. Proposed tokens: mist#F1F6FA, glass#FFFFFFCC, ink#17304A, muted#617386, primary#2164D8, accent#238C8A. Use local Chinese system sans fonts (Microsoft YaHei UI/PingFang SC/system-ui), clear type scale; no downloaded fonts/assets/dependency. Blur has opaque fallback; reduced-motion/focus/mobile constraints remain. Primary reviews visible screenshots before adoption/deployment.

## Interfaces/ownership

Backend owner: internal/config, internal/cli, internal/web/*.go, .env.example, README/native operator docs if needed. Produces the auth/default/events/diagnostic contracts above; no HTML/JS/CSS/agent/storage edits.

Agent owner: internal/agent only, existing Model/Options/Core adapters unchanged. Produces Chinese recall and same-call semantic selection using current storage/provider APIs. No web/config/CLI/storage/provider edits without primary coordination.

UI owner: internal/web/assets/{index.html,app.js,style.css}, scripts/test_{web,library,library_bundle}_view.js. Consumes contracts above; preserves IDs where practical, all existing auth/privacy/cancel/restore/preview guarantees. No Go/private/server edits.

Primary owns this spec/plan, integration, independent review, native builds/checks, documentation phase reconciliation and any future server maintenance.

## Acceptance

Semantic RED→GREEN affected Go/JS tests, exact-byte source baseline, root integrated Go tests/vet, JS regressions, fresh consolidated review and synthetic visible browser flows. Build native amd64/arm64 separately from runtime evidence; paid/WeChat/server work remains separately bounded. Preserve old source-bound failures/results. Open release gate remains deferred even if local revisions pass.
