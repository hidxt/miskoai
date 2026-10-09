# Authenticated Embedded Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans. Execute serial task/review gates; the Medium view task cannot alter authentication or backend policy.

**Goal:** Make the reviewed native controller usable through a bounded authenticated loopback panel and serve/status CLI.

**Architecture:** Web wraps known embedded assets and a narrow fixed-scope controller adapter with one session/origin/concurrency boundary. CLI owns signal cancellation, joins workers and HTTP, then closes controller/store. No frontend runtime or arbitrary filesystem route.

**Tech Stack:** Go1.27.2 stdlib HTTP/embed, HTML/CSS/vanilla JavaScript; no dependency.

**Spec:** docs/superpowers/specs/2026-10-09-core-web-design.md.

## Global Constraints
- Requires reviewed Core/config/privatefs/maintenance/management-storage; no child live/private-data testing, delegation, staging or commit. Children only GPT-6.1 Medium.
- Exact loopback IP/port Host, exact configured HTTP Origin on state changes, no forwarded trust/CORS/public binding. One native process, Linux amd64/arm64,512MB target.
- Credentials only request bodies/private environment, never URL/log/status/static assets. No model-controlled scope/permissions.
- Bounded requests/sessions/downloads, complete strict JSON, cancellation and actual joins. Static UI textContent/escaped text only; no HTML interpretation of user/model/profile data.
- Restore/download/clear use controller ownership; no direct Web SQL/filesystem or reopening live DB. Restored older dedup state remains paused pending explicit operator reconciliation.

## Review Focus
- DNS rebinding Host, cross-origin login/CSRF, forged forwarded headers and session fixation all refuse before application mutation.
- Expired/full session and bootstrap tables stay bounded; brute-force attempts never allocate per attacker IP/key.
- Uploaded duplicate/trailing/huge JSON or restore stream refuses safely, preserving current DB when validation fails.
- HTML/script payload in profile/history/facts stays inert text; private errors never become innerHTML or raw server logs.
- HTTP bind/cancellation/download failure cannot leave background chat, open store, stale owned lock or private incomplete download behind.

## Task1: Loopback HTTP/auth boundary — Medium
**Files:** create internal/web/{server.go,auth.go,json.go,server_test.go,auth_test.go,json_test.go}; report docs/superpowers/reports/2026-10-09-web-auth.md. Task2 later supplies real application routes and assets through the produced constructors.

**Interfaces:**
- `Options{Listen,Password string}`; `New(Options,application http.Handler,assets http.Handler)(*Server,error)` validates exact127.0.0.1/::1 port1..65535/password16..256validUTF8. `Handler()http.Handler` useful for synthetic request tests; `Run(ctx context.Context)error` binds only configured loopback, serves and joins HTTP shutdown. Own no Core/store; CLI coordinates cancellation.
- Known public routes GET /api/bootstrap, POST /api/login; authenticated GET /api/session returns CSRF token only to session owner, POST /api/logout removes session. Other /api/ routes go to application only after session and origin/CSRF checks; known assets go to assets after exact Host check. Unknown routes 404 safe constants.
- GET bootstrap produces crypto32-byte random base64url nonce (43 chars), stored as digest with60s lifetime, at most64, request cleanup. Reject cross-site Sec-Fetch-Site when present; never CORS-enable response. Login POST exact Origin and application/json <=2KiB strict object `{password,nonce}`; consume nonce once, constant-time compare SHA256 password digests, shared local10attempt/min rate state, no per-IP table. Nonce/session cap full safely refuses instead of unbounded allocation. Private clock/random seams support deterministic tests.
- Session and CSRF each crypto32-byte random base64url, store token digests; cookie MiskoAI_session Path=/ HttpOnly SameSite=Strict no Domain, absolute20min lifetime/max64 active sessions; protected server mutex/no per-session goroutine. Loopback HTTP cookie is not Secure; TLS variant would require Secure. Clear cookie on logout/expiry. Correct login creates a fresh session and never adopts supplied cookie. /api/session and every authenticated response Cache-Control:no-store.
- State-changing methods require exact Origin and X-MiskoAI-CSRF constant-time compare; absent/wrong refuses. GET application reads require authenticated cookie and exact Host, no CORS. No method override/query tokens; reject unsafe unrecognized methods. Never derive allowed Origin from attacker Host/Forwarded fields.
- HTTP global active-handler limit32 immediate503 when busy, release only on actual handler return. ReadHeaderTimeout5s, ReadTimeout15s, WriteTimeout35s, IdleTimeout60s, MaxHeaderBytes8KiB. Backup/restore application contexts30s; general reads/writes10s. Shutdown cancels admitted application work and joins active handlers before Run returns; no fake join at deadline.
- Headers CSP `default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'`, nosniff, Referrer-Policy:no-referrer. Panic/ErrorLog emits fixed safe code without payload/path/query/password/token. No access log.
- Local strict JSON helper checks complete validUTF8 object, exact known fields, duplicates/escaped aliases, wrong/null types, trailing values within explicit caps before materialization. Error enums/HTTP bodies safe. This helper does not copy credential/config fields into public output.

- [ ] RED TestHostAndOriginRefusal, TestLoginNonceAndSessionFixation, TestCSRFAndLogout, TestSessionExpiryAndCaps, TestLoginGlobalRateBound, TestHTTPHandlerAdmissionAndShutdown, TestStrictRequestObject. Assert no application invocation for refused inputs, headers/cookies, controlled clock expiry, cancellation joining a deliberately held real handler, and secret-canary absence.
- [ ] Implement minimal stdlib boundary with supplied handlers; absence of application/assets is safe404, not a claimed management feature. No provider requests or Core ownership in this task.
- [ ] Run web tests/vet and report RED/GREEN/security evidence, freeze for fresh Medium review and root checks before application adapter.

## Task2: Real fixed-scope management adapter — Medium
**Files:** create internal/web/{application.go,memory.go,settings.go,system.go,application_test.go,memory_test.go,system_test.go}; report docs/superpowers/reports/2026-10-09-web-management.md. No auth policy changes except a root-approved concrete integration fix.

**Interfaces:**
- `Application(*core.Controller)http.Handler`, with narrow private backend interface for synthetic tests; no Web import of CLI or Store directly. All scopes derive only from Controller.
- GET /api/status; GET /api/settings returns desired/effective/overrides/restart_required; POST /api/settings strict Settings<=16KiB persists desired, never exposes key fragments.
- GET /api/profiles returns bounded custom rows plus three builtin expression IDs and active selection; POST /api/profiles strict Profile<=8KiB, DELETE /api/profiles?id=... and POST /api/profiles/select `{id}`. Profile strings are data. Use current scoped ActiveProfile controller method; add the narrow forwarding method in core/management.go with its test if absent, no lifecycle redesign.
- GET /api/facts?q=&offset=&limit= (limit1..16/default16); POST /api/facts add explicit fact, PATCH /api/facts edit, DELETE /api/facts?id=. Exact fields id/content/category/importance/expires_at; source/confidence assigned by controller, no scope input. GET /api/history?q=&offset=&limit= (limit1..8/default8); fixed bounded page responses.
- GET /api/memory returns derived and at most8 candidates; POST /api/memory/confirm `{id}`, POST /api/memory/reject `{id}`; POST /api/memory/clear `{acknowledge:"clear_memory"}` cancels/joins through Core. IDs strict positive signed int64, no float/truncation. All scalar queries unique known names, no duplicate alias/unknown fields accepted.
- POST /api/export and POST /api/backup no supplied filename/path, obtain completed Download then send fixed application/json or application/octet-stream Content-Disposition basename; Close on all transfer paths. No URL session tokens; no database lease during transfer. Request body at most1KiB exact empty object. Total normal JSON response<=2MiB; pages/profile/candidate shapes guarantee bounded encoding, checked before response success.
- POST /api/restore with application/octet-stream<=256MiB+1/30s total passes reader to Core.Restore, responds prior-retained/channel-paused booleans only. Validate content type/method before reading. POST /api/channel/resume exact `{acknowledge:"reconciled_restored_history"}` invokes explicit reconciliation action; UI requires a human confirmation, startup never invokes it.
- GET /api/diagnostics returns safe build/config/local readiness/status only, no external probe. Synthetic local doctor is CLI; HTTP must not reopen a running database. No action invokes cloud/QR login implicitly. Stickers are absent until real media/library integration; no pretending unsupported route works.

- [ ] RED TestManagementAuthAndScope, TestManagementStrictInputs, TestManagementSettingsNeverSecrets, TestCRUDAndCandidates, TestDownloadCancelCleanup, TestRestorePreservesAndPauses. Use bounded synthetic controller fixtures/fact/model canaries; assert hostile scope keys rejected, correct data isolation, no paid network, and normal JSON caps.
- [ ] Implement methods/endpoints with existing controllers. Add only narrowly missing ActiveProfile forwarding to owned core/management.go/test; never SQL in HTTP.
- [ ] Run web/core/maintenance/storage/config tests/vet; report/freeze for fresh Medium spec/security review and root integrated checks.

## Task3: Embedded usable panel — Medium
**Files:** create internal/web/{assets.go,assets_test.go,assets/index.html,assets/app.js,assets/style.css}; report docs/superpowers/reports/2026-10-09-web-view.md. The view implementer cannot edit auth/server/backend/config/storage files.

**Interfaces:** `Assets()http.Handler` serves only exact embedded /,/app.js,/style.css with correct MIME; no filesystem fallback or arbitrary path traversal. Task4 composes New(options,Application(controller),Assets()).

- [ ] Add a login form using bootstrap nonce/password POST and same-origin credentials, in-memory CSRF from login/session, no localStorage/query token. Password field clears after request. Dashboard displays true safe metrics, heap explicitly labeled; unconfigured/paused state and unavailable RSS honest.
- [ ] Build Dashboard, AI/personality, Memory/history and System views with responsive readable Chinese labels/MiskoAI brand. Use accessible labeled forms, visible loading/error/empty states. Settings distinguish saved/effective/ENV overrides and restart requirement; profiles create/edit/select/delete; memory search/pagination/edit/delete/candidate confirmation/rejection/clear; history searchable and text only. No sticker tab until that subsystem exists.
- [ ] Implement CSRF headers for every mutation, explicit confirmation for clear/restore/resume, logout; downloads use POST/fetch Blob within supplied export/backup limits and immediately revoke ObjectURL after triggering fixed filename download. Restore raw file cap256MiB checked before upload; output states preserved backup and paused channel. Never build HTML from remote strings; textContent/createElement and fixed templates only. No inline script/style/eval/third-party assets.
- [ ] Run Go embed/static security tests checking exact routes/no filesystem fallback/CSP-compatible script/style and malicious text inert. Report actual browser checks versus code-only checks. Root performs browser UI validation against a synthetic backend, never actual private service. No invented screenshots or successful nonexistent backend claims.

## Task4: Native serve/status integration — Medium
**Files:** modify internal/cli/{cli.go,cli_test.go}; create internal/cli/{serve.go,status.go,serve_test.go,status_test.go}; modify cmd/miskoai/main.go only if signal ownership requires it. Report docs/superpowers/reports/2026-10-09-serve-cli.md. Root owns README/environment/systemd/release docs separately.

**Interfaces:** preserve Run(args,out) and add `RunContext(ctx context.Context,args []string,out io.Writer)error`; main uses signal.NotifyContext(SIGINT/SIGTERM native variants if needed). serve no secrets in arguments; config/privatefs load, Core.New, Web.New and joined runs. On either server/controller failure cancel sibling; join actual channel/summary and HTTP handlers, then Core.Close. Bind failure closes lock/store without starting external work where possible. Startup safe message only local address/version; never auth IDs/tokens.
- `status` uses configured loopback URL with direct bounded HTTP transport/no env proxy/redirect. Bootstrap/login/session exact Origin with admin password from private Config only, retain cookie/CSRF in memory, fetch safe status then logout; total10s/response2MiB. No credentials in URL or errors. Unavailable server returns safe stopped/unreachable result, never opens/migrates DB. Explicit admin password missing fails safely.
- [ ] RED TestServeBindFailureUnwinds, TestServeSignalJoins, TestStatusAuthenticatedFlow, TestStatusNoCredentialLogging. Tests inject synthetic dependencies/private data directory; no real credentials/server calls. A management-only compiled-binary smoke test may use private synthetic settings and missing WeChat auth with explicitly synthetic env password.
- [ ] Wire actual reviewed packages into one binary and update CLI help; retain existing init/doctor/config/backup/restore/version/licenses behavior. No auto-login/probes or background service installation.
- [ ] Run all Go tests/vet and build Linux amd64/arm64 using verified Go1.27.2; report actual evidence/freeze. Root fresh Medium review/native Linux race/manual security/scans/UI checks before development publication and any VPS request.

## Root gate/self-review
The four tasks cover auth, fixed-scope panel, embedded view and process integration with explicit interfaces; underlying Core/maintenance/storage/config have prior gates. All Review Focus cases appear in owning tests. UI uses implemented endpoints and honest states; stickers/media later. Root validates imports/composition, private metadata redaction, meaningful browser behavior, actual binary linkage and shutdown ordering. Hosted CI and synthetic UI do not claim real WeChat or512MB performance. User's standing autonomous method overrides routine plan confirmation; dangerous actual deployment/restore still requires its own informed authorization.
