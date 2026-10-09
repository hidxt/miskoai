# SDD ledger — plan: docs/superpowers/plans/2026-10-09-authenticated-management.md

Root owns integration/publication/acceptance. Every child uses ONLY GPT-6.1 Sol Medium (gpt-6.1-sol, reasoning_effort medium), with isolated context and no delegation. Windows uses plan-scoped ignored .tools/reviews/web-* briefs/exports/reports; durable progress lives here. Existing codex/foundation development branch is authorized; main is untouched. The owner overrides routine process confirmation gates.

Preflight on 2026-10-10: Core locally accepted at 0362dfe42c87862167a73630dcf7b2e31783eb3a after root702pass/0fail/4platformskips/fullvet0 and scoped review. Full-history publication gate/native CI pending. Task1 is not dispatched until the Core native gate succeeds. Engineering progress48%.

| Tasks | Producer / consumer boundary checked | Finding / ruling |
|---|---|---|
| 1 / 2 | Server authenticates supplied Application handler; strict JSON helper reused by DTO adapters | Serial ownership; no adapter edits to auth policy without concrete root ruling. GET reads remain scoped; all mutations require Origin and CSRF. |
| 1 / 3 | Server wraps supplied Assets handler; exact Host and CSP protect embedded routes | Assets serve only three known routes and have no filesystem fallback. Static assets can be public after Host validation; private API data requires authentication. |
| 1 / 4 | Options/New/Handler/Run consumed by serve; same login flow consumed by status | Server owns actual HTTP handler shutdown only; CLI cancels siblings and closes Core after actual joins. Synthetic loopback listener fixtures are permitted, external/private service calls are prohibited. |
| 2 / 3 | Fixed API DTOs, safe errors, download and restore responses consumed by UI | Task2 must freeze actual response shapes before Task3; UI cannot invent backend support or expose credential fragments. |
| 2 / 4 | Application(Core) composed by serve; safe status consumed by status client | No direct Web SQL/CLI import and no database opening by status. ActiveProfile forwarding already exists, so no Core edit is currently required. |
| 3 / 4 | Assets() embedded handler consumed by serve | UI/browser evidence is synthetic only; Task4 joins reviewed handlers in one native binary. |
| 1 internal | Auth/session/token bounds and tests versus owned server/auth/json files | Digest-only token state conflicts with returning the original CSRF again from /api/session. Ruling below specifies bounded rotation rather than retaining raw tokens. Handler fixtures test real joining. |
| 2 internal | Exact DTO/query parsing, fixed scope and completed downloads versus adapter tests | Caps and IDs are checked before materialization; no arbitrary path/scope. Restore consumes Core's validated bounded reader and remains paused. Read deadline integration is checked in Task2 with concrete runtime evidence. |
| 3 internal | Embed files, CSP-compatible external assets, inert text and browser claims | No auth/backend edits. No sticker view until that backend exists. Blob downloads stay within backend limits and revoke ObjectURLs. |
| 4 internal | RunContext/signal ownership, bind failure and shutdown tests versus CLI/main edits | Preserve existing Run and commands. Bind failure should unwind before channel side effects; actual canceled work must join before Core.Close. Native CI remains a separate gate. |

Ruling: /api/session may mint and return a fresh random32-byte CSRF token for the authenticated session, replace only its stored digest, and invalidate the previous CSRF token. Login also returns its newly minted token. This satisfies digest-only token storage and a usable session refresh endpoint without recovering or retaining raw secrets. No protected application state is changed by refresh; table/session lifetime/admission stay bounded. If wrong, concurrent tabs require refreshing their in-memory token, an explicit UX limitation to handle in the view, rather than weakening CSRF or persisting tokens. Implementer must test old-token refusal and new-token acceptance and record this behavior. No provider/SQLite/deployment/resource-target change.

- Task1: notstarted — prepared isolated .tools/reviews/web-auth-brief.md, waiting Core native gate.
- Task2: notstarted — fixed-scope adapter after Task1 acceptance.
- Task3: notstarted — embedded panel after frozen adapter DTOs.
- Task4: notstarted — serve/status after preceding acceptance.

Core prerequisite satisfied:0362dfe exact nativeCI37974404440 all configured steps success, read by root. Task1 notstarted; single independent codec implementation/review gate runs first. Engineering progress48%. No private/live service is started.

Task1 started on reviewed/nativeCore0362dfe: /root/web_auth, ONLYGPT-6.1 Sol Medium (gpt-6.1-sol,medium), exact isolated .tools/reviews/web-auth-brief.md. Model/effort+48% announced before dispatch. Own six new server/auth/json source/tests and author report only; noCore/store/actualprivate/network/Git/delegation. Disjoint frozen codec read-only review/root tests run concurrently without shared edits. Web task inprogress, no completion credit; codec subsequently localaccepted brings project50%.
