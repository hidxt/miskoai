# Embedded library administration — Task8 local evidence

2026-10-13. Author: gpt-6.1-sol, reasoning_effort medium. Engineering progress remains **74%** (37/50). Implementation and local tests only; no package39 credit, native CI, live/provider/WeChat, server/512MB, browser integration or final acceptance claim.

The isolated source is `.tools/reviews/library-management-view-work-src`, based on all 392 entries of `.tools/reviews/library-core-snapshot-root-source-manifest.json`. Every basis and copied byte length/SHA256 was checked before editing. Main checkout was not adopted or edited. Backend HTTP integration remains a separate dependency and must be joined by the root.

## Changes and contracts

Exactly six owned files: `internal/web/assets/index.html`, `app.js`, `style.css`, `scripts/test_web_view.js`, new `scripts/test_library_view.js`, and this report. No Go/backend/auth/CSP/Core/library/storage/dependency/other test/root ledger/Git changes.

Chinese library navigation supplies upload, bounded metadata listing, separate editing, deliberate enable/disable, confirmed deletion and requested image preview. GET uses `after` canonical strings and `limit=32`; a full page permits navigation, capped at four pages / 128 entries. IDs and recorded uses remain exact strings, including values above JavaScript's safe integer range; last-use milliseconds stay exact strings. Rows render metadata solely through textContent, including hostile tags, descriptions, state and errors. Metadata availability stays explicitly unchecked until the requested image's load event.

POST `/api/library` submits the captured File as raw `application/octet-stream`, refuses empty and >4 MiB files before fetch, and never sends filename, scope, metadata or enable/usage payload. Known success resets the file and refreshes from cursor0. New-item feedback explains disabled/empty metadata; duplicate feedback explains retained state. Upload never automatically PATCHes. Uncertain writes require deliberate refresh and are never retried.

Explicit PATCH sends only `{id,tags,emotion,description,enabled}`. UTF8 decoded-byte bounds, 16 unique nonblank tags, control/NUL refusal and actual boolean enabled are validated before fetch. The backend remains authoritative for image and all persistent metadata admission. Enable/disable uses the captured row metadata after deliberate click. DELETE uses the existing confirmation helper and one request; cancellation preserves the editor and preview. Known delete success clears matching editor/preview state.

Preview uses only `/api/library/preview?id=<canonical-id>` on the authenticated same origin, under existing `img-src 'self'`. No preview blob/data URLs, remote image source or CSP change. Only current image load reports “本次预览已通过服务端校验”; errors use fixed honest refusal. Clearing removes src, handlers, nodes and status and advances a preview generation. Both generation and session epoch guards reject saved late callbacks after replacement, logout, session expiry or restore dispatch.

Every dispatched database restore clears all library rows/forms/files/ID/digest/cursors/preview before request, including failed or aborted results. The locally captured restore File still reaches the original raw request. Cancelled confirmation preserves edits. The next library selection fetches restored metadata from cursor0; successful system restore does not eagerly fetch library. Existing fact/profile/restore regressions remain unchanged. Existing SQLite backup wording now explicitly says database snapshot includes metadata and excludes binary images, retains old content, may require complete content recovery, and keeps channel pause/manual reconciliation. No full-bundle or sticker-send controls were added.

Responsive CSS bounds preview to container width and 240px height, wraps long metadata/editor identity, and gives the five-section mobile navigation three columns. Labels, status regions, keyboard-accessible native buttons/forms/confirmation remain. Actual 375px rendering and browser keyboard/image/file-picker behavior await root checks.

## Actual commands and results

Optional development-only Node was discovered via `load_workspace_dependencies`: `C:/Users/auzasr/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node.exe`, version `v24.19.0`. No install or permanent runtime. From the isolated source, the actual commands were:

```powershell
& 'C:/Users/auzasr/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node.exe' scripts/test_library_view.js
& 'C:/Users/auzasr/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node.exe' scripts/test_web_view.js
```

Initial executable RED, before production edits: **0 pass / 14 fail / exit1**, all missing usable library navigation. Original production JS, HTML, test and log retained. An additional control ran unchanged original production app.js against the new DOM fixture: **1 pass / 13 fail / exit1**; high-ID listing and upload request assertions fail semantically (empty listing / zero writes), while the no-invalid-upload-write assertion passes. These are behavioral failures, not compile/import failures. The first production GREEN was **14 pass / 0 fail / exit0**. Three further negative/cancel/recovery cases expanded the final suite to **17 pass / 0 fail / exit0**. All intermediate logs remain.

Final library cases cover high canonical IDs/uses and inert hostile text, raw new/duplicate upload with one write, file bounds, UTF8/control/count/duplicate/tag bounds and boolean metadata, four-page string cursors/previous/refresh, short-page stopping, enable preservation, editor cancellation/stale-ID refusal, malformed page refusal, delete cancellation/confirmed cleanup, preview load/error/late callbacks, restore success/failure/abort/cancel, logout/401, failed/aborted upload and recovery responses to save/delete. The synthetic fetch checks exact methods/query/body and same-origin/CSRF constraints without network effects. Existing script retains all four success/failure/abort/cancel cases and all prior assertions: **4 pass / 0 fail / exit0**. Its DOM fixture now exports reusable utilities behind `require.main` and models checkbox checked/defaultChecked/reset; earlier form numeric collection.length/namedItem behavior is preserved.

Go command runner `.tools/reviews/library-management-view-checks.ps1` uses only `.tools/toolchains/go1.27.2-verified/go/bin/go.exe`; clears MISKOAI environment entries by name only, and sets offline local toolchain/proxy/sumdb with separate repository cache paths `.tools/go`, `.tools/gomod`, `.tools/gocache`, `.tools/tmp` (GOTMPDIR/TEMP/TMP all the latter). Actual commands from the isolated source:

```powershell
go.exe test -json -count=1 -timeout=30s ./internal/web -run '^TestAssets'
go.exe vet ./internal/web
```

Scoped assets tests: **7 named pass / 0 fail / 0 skip / exit0**, one package pass; test stderr empty. Scoped vet: **exit0**, output empty. These finite existing asset/authentication-boundary cases verify embedded representation and unchanged server policy, not the new separate HTTP backend or an actual browser. No broad suite repeat.

## Frozen handoff and limits

Owned files, complete source and actual/copy evidence byte lengths/SHA256 are frozen under the author-owned `.tools/reviews/library-management-view-*` prefix. `library-management-view-freeze.json`, owned/source manifests, copied evidence manifest and textual diff provide root verification anchors. Complete source has 394 files (392 basis plus new test/report); all 388 unowned basis files must stay byte-identical. Root must read the full diff/code, independently verify identities/re-run integrated checks, and join the HTTP backend before desktop/mobile/CSP/file-picker/restore browser verification. Author is quiescent after handoff.

DOM/HTTP fixtures are synthetic and do not prove browser rendering, authenticated image networking, CSP enforcement, real file input, real Core/backend, native CI, live providers, native animated sticker delivery, arm64 runtime/systemd/server/512MB or final acceptance. No credentials/private chats/database/provider/CDN/WeChat/service starts or real network effects. No scope or resource change, public push, main adoption, bundle API or reply capability claim.

## FIX1 — reviewed existing metadata admission and deletion wording

2026-10-13, original author ONLY gpt-6.1-sol / reasoning_effort medium; engineering **74%**, unchanged37/50. Fresh review `library-management-view-fresh-review-oct13.md` requested changes with0Critical/1Important/1Minor. I1: storage and Task6 HTTP accept nonempty whitespace-only tags, while the view applied its stricter new-input nonblank policy to every response row, blocking an entire valid page. M1: confirmation promised removal of local bytes although retained-content recovery may preserve rollback bytes. Both findings are addressed in the FIX1 source, subject to original-reviewer scoped re-review and root integration.

The entire original394-file source freeze, six owned identities,23 evidence identities and original17/4/7/vet results remain exact. FIX1 starts from a separately copied and SHA256/length-verified original frozen source at `.tools/reviews/library-management-view-fix1-work-src`; main remains untouched. Only app.js, test_library_view.js and this appended report differ from the original, within the same six-file ownership. HTML/CSS/existing four-regression script and all Go/backend/storage/CSP/root ledgers/dependencies remain exact.

`libraryExistingMetadata` now supplies backend-compatible decoded metadata admission for listing and deliberate enable/disable: nonempty whitespace-only tags remain exact and manageable. The stricter `libraryMetadata` used by explicit Save rejects new blank tags after compatible admission. No persisted tags are trimmed, normalized, silently patched or restricted by a storage policy change. Existing records can display, edit, preview, delete and especially disable without first correcting whitespace. A user may explicitly correct the tags through Save. Exact-empty/null/control/over-bound metadata and nonboolean enabled remain refused; existing hostile markup remains inert text under original regressions. Confirmation now says `删除当前范围的图片记录；用于恢复的旧内容可能保留`.

Actual reviewer diagnostic reproduction, before production fix, from `.tools/reviews/library-management-view-fresh-review-oct13-src`:

```powershell
& 'C:/Users/auzasr/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node.exe' scripts/reviewer_library_whitespace.js
```

Result **17 pass / 1 fail / exit1**, semantic assertion `backend-valid record must be visible and manageable`. Exact diagnostic source was copied under the author FIX1 prefix and output retained as `library-management-view-fix1-reviewer-reproduction.txt`; no reviewer source mutation.

Added five regression cases first, before production fix: mixed whitespace/ordinary rows with preview; exact preserved tags during both enable and disable; edit population/strict Save refusal then deliberate correction; whitespace deletion and honest wording; retained invalid response refusal. Runtime RED **18 pass / 4 fail / exit1**, original17 unchanged and invalid-response case passed. The first deletion regression used a missing button dereference; that TypeError is retained and receives no standalone semantic assertion credit. It was corrected to an explicit missing-deletion-control assertion before production changes, rerun **18 pass / 4 fail / exit1** with all four missing behaviors asserted. Exact pre-fix app/test/log copies are retained. First fix GREEN **22 pass / 0 fail / exit0**.

A sixth case checks M1 directly on an ordinary record. An immutable original app.js control with this final test fixture produced **18 pass / 5 fail / exit1**, including the direct semantic wording failure `confirmation must not promise physical deletion of rollback content`. This additional reverted-production control is separately retained with all four source files and does not alter the original freeze. Final commands from FIX1 work source:

```powershell
& 'C:/Users/auzasr/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node.exe' scripts/test_library_view.js
& 'C:/Users/auzasr/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node.exe' scripts/test_web_view.js
```

Final **23 pass / 0 fail / exit0**, including all original17 assertions unchanged plus six FIX1 cases. Original restore script **4 pass / 0 fail / exit0**, exact source unchanged. No Go/vet rerun: no Go/backend/auth/CSP or embedded HTML/CSS changed; the original finite7pass/assets/vet0 evidence remains exact, while root integrated checks remain required. No broader suite repeats.

Separate FIX1 full-source/owned/evidence manifests and immutable copies are under `.tools/reviews/library-management-view-fix1-*`. The freeze proves394 source files,391 unchanged versus original source, only three allowed changed paths, preserved original source/evidence identities, main untouched and zero actual/copied mismatches. Root must inspect the correction and independently verify identities; original fresh reviewer must re-review I1/M1. No browser/file-picker/CSP/backend integration/native/live/resource/final claim is added. Author quiesces after this separate FIX1 handoff.
