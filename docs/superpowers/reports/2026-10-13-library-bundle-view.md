# Full library bundle view groundwork

2026-10-13. Author harness: **gpt-6.1-sol / medium**, no children. Engineering progress remains **74% (37/50)**. This report covers implementation and local synthetic evidence only. Package39, actual bundle Core/Web integration, new native CI, browser/native file picker, live tests and final acceptance are not credited.

## Scope and basis

Four owned paths only:

- `internal/web/assets/index.html`
- `internal/web/assets/app.js`
- `scripts/test_library_bundle_view.js`
- `docs/superpowers/reports/2026-10-13-library-bundle-view.md`

Work is isolated in `.tools/reviews/library-bundle-view-src`, with a complete frozen copy in `.tools/reviews/library-bundle-view-final-src`. No main source adoption or Git action was performed. Styles, shared fixtures, Go, Core, backend, dependencies, root documentation and LICENSE remain unchanged in this source.

The supplied directory name initially appended the file count (`library-bundle-root-reviewed-src416`) and did not exist. The initial setup gate failed with exit1 before production editing. Its416 missing-path identities are retained in `setup-missing-supplied-path.json`; this is setup failure, not RED evidence. Root corrected the directory to `.tools/reviews/library-bundle-root-reviewed-src`. All **416** byte lengths and SHA256 identities matched `.tools/reviews/library-bundle-root-source-manifest.json` before copying; all416 copied identities also matched (`basis-check.json`). No mutable main source was used as the implementation basis.

Read governing repository documents, private-library design, bundle view/preparation/install/Core plans and the existing library management/reset plan. Parent owns decision/root-document updates, actual backend integration, fresh review, acceptance, full scans and publication.

## Resulting behavior

System gains explicit complete database plus current-image download and restore controls. The all-scope/private nature of the artifact,321MiB bound, exclusion of rollback-only images, validation before installation, possible receive/dedup/send history rollback, retained old-content uncertainty and separate manual channel reconciliation are visible in Chinese. Complete restore explicitly warns that generation stops before preparation and can stay stopped on invalid input. Failure or canceled waiting requires status inspection before a manual decision.

The fixed complete-download route is POST `/api/library/backup`, existing empty JSON body `{}`, filename `miskoai-library.bundle`,321MiB binary bound. It reuses the original bounded streaming/download helper and immediately revokes the temporary object URL even when the click throws. Browser Blob buffering is not VPS memory evidence.

Full restore reads the selected File once, refuses absent/unknown/noninteger/negative/zero/over321MiB sizes before confirmation or fetch, captures that exact File locally, and sends it unchanged as raw `application/octet-stream` to POST `/api/library/restore`. No original filename/path, base64, large JSON conversion or second request is submitted. After confirmation, the shared private reset clears old fact/profile/library IDs, digest/title/metadata, list/cursor/pager state, uploaded files, previews/generations and both restore results **before fetch**. Canceling confirmation preserves every seeded editor/cache/cursor/file/preview. Error, abort and401 leave old private state cleared. Success admits both response flags only when their types are boolean, displays their actual values, reloads existing authoritative management views and never resumes the channel. Missing/invalid flags remain ambiguous and do not reload or retry.

The original SQLite-only256MiB controls retain their route, filename, bytes and labels. Their existing reset now clears the new full-bundle controls too.

### Root ruling: shared post-text session/cancel guard

A controlled delayed Blob.text completion exposed an existing request-helper gap: the last epoch check preceded an await. Logout during JSON decoding could repopulate private DOM/results, and cancel during decoding could report success and dispatch follow-up reads. The initial local full-handler-only guard was then replaced under the primary's explicit ownership extension with the correct shared helper guard. `request()` now checks epoch and `controller.signal.aborted` after awaited text completion and **before JSON parsing/return**. It reuses original error classification, session clearing and cancellation helpers; routes/safety bounds are unchanged. The shared fix protects existing SQLite restore and private reads as well as the new handler. No duplicate handler guard remains.

Author self-review then reproduced the adjacent canceled-download EOF gap: abort during an awaited final reader.read could return done=true, letting the original request helper create a Blob despite cancellation. Under root's same shared-request ruling, the existing abort guard also runs immediately after boundedBody alongside the epoch check, before returning either Blob or JSON. The original26-test freeze/owned diff/manifests and exact four owned contents are retained under `pre-body-*` evidence. Actual body-guard semantic RED was26PASS/1FAIL/exit1 (object URL created after cancel); final regression exercises both full-bundle and original SQLite downloads with no artifact or success notice after cancel.

## Executed evidence

All Node commands used the development-only bundled executable:

`C:/Users/auzasr/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node.exe`

Final commands ran from the isolated source cwd. Exact original/corrected stdout/stderr and actual captured child exits live under `.tools/reviews/library-bundle-view-evidence`; commands below are separate runs, not an inferred aggregate.

| Command | Actual result | Boundary |
| --- | --- | --- |
| `node --check scripts/test_library_bundle_view.js` before production edit | exit0 | Fixture parses |
| Original `node scripts/test_library_bundle_view.js` against original production before editing | exit1;1PASS/21FAIL | Actual VM startup succeeded; missing complete control failed semantically. Seed setup failures were not credited as privacy RED |
| Corrected fixture against preserved original416-file source | compile0/runtime1;1PASS/22FAIL | Actual original UI still lacks complete controls; corrected baseline source retained separately |
| Controlled first late-JSON regression | exit1;22PASS/1FAIL | New result repopulated after logout; first duplicate-text hold fixture timeout retained separately |
| Root-expanded shared-helper RED | exit1;23PASS/3FAIL | Actual SQLite result repopulation, private fact-list repopulation and canceled restore follow-up observed |
| Canceled final download read RED | exit1;26PASS/1FAIL | Actual object URL/artifact creation after cancel |
| Final `node scripts/test_library_bundle_view.js` | **exit0;27PASS/0FAIL** | Actual final production JS and exported existing synthetic DOM |
| Final `node scripts/test_library_view.js` | **exit0;23PASS/0FAIL** | Existing library fixture unchanged |
| Final `node scripts/test_web_view.js` | **exit0;4PASS/0FAIL** | Existing database restore fixture unchanged |
| `go test -json -count=1 -timeout=30s -run '^TestAssets' ./internal/web` | **exit0;7 named/distinct PASS,0FAIL,0SKIP** | Windows host asset/embed/auth composition only |
| `go vet ./internal/web` | **exit0;empty diagnostics** | Scoped package verification only |

Go used exact `.tools/toolchains/go1.27.2-verified/go/bin/go.exe` with `GOTOOLCHAIN=local`, `GOPROXY=off`, `GOSUMDB=off`, workspace `.tools/go`, `.tools/gomod`, `.tools/gocache` and `.tools/tmp`. MISKOAI variables were removed by name without printing values. No provider, actual private database, Core.Run, CDN or external channel was used.

Retained initial green attempts:11PASS/11FAIL from an overlong synthetic tag rejected by production's32-byte bound; diagnostic rerun with the same setup issue (Node exit not separately captured; no exit credit);15PASS/7FAIL from a fixture `endsWith("file")` check incorrectly classifying `active-profile` as a file. Correcting the admitted synthetic tag and selecting exact file-control IDs resolved those fixture errors without weakening production policy. Abort could retain its45s request timer after an early failed assertion in the flawed run; its actual failure/output/exit remain separate. Original intermediate22/23-test green runs and pre-shared-guard production source are retained, not overwritten or represented as final evidence.

The27 final tests cover fixed complete and original SQLite download routes, exact raw File identity and CSRF/same-origin transport, response/header/stream limits and truncation, URL cleanup on click failure, file gates including exactly321MiB, confirmation cancel, confirmed success/error/abort/401 reset order, inactive old preview callbacks, disabled old pagers, reused high IDs/different restored metadata and inert hostile text, discarded old fact/library targets, SQLite reset of new controls, logout/401 clearing, stale response/body/text handling, canceled final binary-read completion, shared SQLite/private GET/cancel post-text guards, strict flag types and no implicit replay/channel resume. File-size ceiling tests use synthetic File metadata, not a321MiB allocation.

## Freeze and limits

`library-bundle-view-owned-manifest.json` records the four actual owned identities; `library-bundle-view-source-manifest.json` records the complete418-file source. `library-bundle-view-freeze-check.json` records exact actual/copied identity and unchanged-unowned gates. `library-bundle-view-evidence-manifest.json` records original/corrected evidence byte lengths and SHA256 identities. The unified owned diff is retained at `.tools/reviews/library-bundle-view-evidence/owned.diff`. All artifacts are author-local and quiescent for primary/fresh review.

Synthetic DOM/Response/Blob boundaries are deliberately controlled. They prove exercised JavaScript behavior, not HTML/CSP rendering, native File picker/download usability, responsive layout, actual large-file RAM, filesystem/SQLite restore consistency, real route installation, channels or live resource targets. Asset checks prove embedded bytes/auth composition only, not complete-bundle server behavior. The new endpoints must not be published/exposed until reviewed real Core/Web routes are integrated and the primary runs backend/browser/combined acceptance gates. No native Linux/runtime/server/512MB/power-crash/live/final acceptance follows from this task.
