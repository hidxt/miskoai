# Embedded private library view implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development or executing-plans. Root integrates/reviews/Git; routine work is owner-authorized.

**Goal:** Give the administrator usable upload/edit/enable/disable/delete/preview management for actual local PNG/JPEG/GIF assets.

**Architecture:** Add one embedded Chinese library section using the existing serialized request/session/confirmation helpers and exact Task6 HTTP contract. Local listing remains unchecked until a requested preview; native sticker transport stays unverified. Clear every library ID/form/file/preview/cursor with the existing restore and logout reset.

**Tech Stack:** Existing embedded HTML/CSS/vanilla JavaScript, optional development-only Node VM regressions; no frontend dependency/build service/CSP change.

**Spec:** docs/superpowers/specs/2026-10-12-private-library-design.md Core/Web section and docs/superpowers/plans/2026-10-13-library-management-http.md. Full bundle UI and reply capability follow their backend gates.

## Global constraints

- Children ONLY gpt-6.1-sol / medium; engineering74%, no39 completion until complete bundle/recovery and root/backend/browser gates. No real credentials/private chats/database/provider/CDN/WeChat/service start.
- Preserve existing auth/CSRF/same-origin/Host/CSP, request cancellation/epoch handling and no automatic mutation retry. Render all metadata/error strings through textContent, never innerHTML or fetched HTML. Images only from fixed authenticated `/api/library/preview?id=<canonical-id>` under existing img-src self; no data/blob/arbitrary remote image URLs or CSP relaxation.
- Raw upload1..4MiB, supported image file input hint plus actual backend validation. New upload is disabled with empty metadata; do not silently patch duplicate or new records. Explicit metadata Save is separate. Client validates UTF8 byte bounds, at most16 unique nonblank tags1..32bytes, emotion64bytes/control refusal, description1024bytes/noNUL. Backend stays authoritative.
- ID/uses/last_used_millis remain exact canonical strings. Numeric cursor conversions prohibited. Display recorded usage, actual format/size/dimensions/frames/state and enabled; metadata listing's availability=unchecked must never be labeled checked/usable. Missing/corrupt preview shows fixed honest refusal.
- Existing SQLite backup wording becomes database-only, includes library metadata but excludes binary images; old-content retention/channel pause retained. Add no full bundle button before its API exists and no sticker sending control/capability claim. Profile off/low/normal remains expression preference.

## Review focus

1. High IDs, hostile tags/description and duplicate payload do not cause precision loss, markup execution, metadata overwrite or automatic enable.
2. Restore dispatch (including failure/abort), logout/session expiry removes library edit IDs/file selection/cursors/private rows and preview before any reused ID can be submitted. Cancel before dispatch preserves current edits.
3. Image load/error arriving after reset cannot repopulate private state; remove src and handlers on clear and use a local preview generation guard in addition to session epoch.
4. Invalid/oversized upload or metadata is rejected before fetch; uncertain mutations are never replayed. Confirmation precedes delete; a recovery response never presents success.
5. Four-page bounded keyset navigation, keyboard labels/confirmation and mobile375px retain usable forms and layout; no hidden unbounded all-catalog download.

### Task8: Usable embedded library administration

**Owned six files:** modify internal/web/assets/index.html, app.js, style.css, scripts/test_web_view.js; create scripts/test_library_view.js and docs/superpowers/reports/2026-10-13-library-management-view.md. All Go/backend/server/auth/CSP/Core/library/storage/other tests/rootdocs/Git readonly. Own ignored `.tools/reviews/library-management-view-*`, isolated exact392 root source/manifest. HTTP author owns application.go and new backend library files; no conflicts. Do not adopt main.

**Interfaces/behavior:**
- Add nav `data-section="library"`, section id library, refresh-library, library-list, library-prev/next/page, library-upload-form/file, library-editor, library-form (tags multiline one per line, emotion, description, enabled checkbox), cancel-library, library-preview and library-preview-status. Accessible labels/state/empty/unconfigured hints, existing card/actions/pager style; responsive previews bounded240px/high and container width, descriptions/tags wrap. No new external assets.
- Extend state with library editing ID/digest, bounded after/previous cursor stack and preview-generation counter; existing helpers `stringID`, boundedText, request, action, run, confirmOperation used. List GET `/api/library?after=<string>&limit=32`; next_after canonical string, items<=32, only a full page permits next; previous stack bounded by global128 entries. New section selection fetches actual page, displays safe metadata/actions; preserve earlier pages/auth/settings/memory/system.
- Upload snapshots selected File then POST `/api/library` with options.raw; refuse empty/>4MiB before fetch. Reset file after known success, refresh bounded list to0, explain new item disabled or duplicate retained; don't automatically PATCH or enable. A failed/aborted response requires manual state refresh; no retry.
- Edit populates fields and captures canonical ID. Explicit Save PATCH `/api/library` exact fields id,tags,emotion,description,enabled; enabled is boolean. Cancel clears editor. Enable/disable action may PATCH all captured metadata only after normal deliberate click; serialized run prevents concurrent UI mutation. Delete uses existing confirmation, DELETE?id once, clears same editor/preview and refreshes list on known success. No public record-use request.
- Preview creates only fixed same-origin img URL from canonical ID. Display `本次预览已通过服务端校验` only after that image's load event with unchanged preview/session generation; errors display fixed refusal. Clear removes src/handlers/node/status and increments preview generation. Neither preview nor list invents use counts, availability across restore or native animated transport.
- Add library containers/forms/file/cursors/editor/preview to resetPrivate. Restore still captures its chosen File before reset. Every dispatched restore resets all library state BEFORE request, regardless response outcome; cancelled confirmation preserves it. Successful restore need not fetch library while system section is active: next section selection fetches restored metadata. Logout/session expiry uses same reset. Existing fact/profile regression behavior must remain unchanged.
- Keep SQLite backup route/limits; change system text to 数据库快照/恢复数据库, explicitly binary images excluded and restored metadata may need full content recovery. No unimplemented controls.

- [ ] Add executable semantic RED against actual unmodified production app.js with a synthetic DOM/fetch fixture for library actions and reset; no source-text-only mirror tests/compile failure credit. Preserve actual commands/source/results. Optional Node is development-only, exact available bundled runtime; no installs/permanent processes.
- [ ] Implement six-owned view/fixtures/report. Enhance old test fixture only when new DOM behavior needs it, retaining original four tests/assertions; do not weaken earlier restore regressions.
- [ ] New runtime tests exercise high IDs/hostile text, exact raw upload+duplicate/no extra writes, metadata boundaries/boolean, pagination, delete cancel/confirmed, preview late load/reset and success/error, dispatch restore success/failure/abort versus cancel, logout/401 and no retries. Extend synthetic fetch exact Task6 route/DTO contract, assert no foreign scope/path/usage payload.
- [ ] Run both optional existing four-regression script and new finite runtime cases plus relevant embedded assets Go tests/scoped vet. Explain DOM fixture is not browser/CSP/file-picker/backend evidence. Root actual synthetic browser with integrated real backend later validates desktop/mobile/preview/restore; unavailable tool checks remain honest.
- [ ] Freeze six owned/complete-source/evidence actual/copied SHA256+byte manifests, report actual commands/results/original failures/limits then quiesce. No main adoption/root docs/Git/private network effect, no39/native/live/resource/final credit.

Self-review: every visible control has an implemented Task6 endpoint or existing supported action; backend is a separate integration dependency. Existing session/restore reset extends to all sensitive new state and late image events. Local management and unverified native sticker transport remain distinct, database-only backup wording is corrected, and mobile/accessibility/runtime tests cover all five focus cases.
