# Native serve and authenticated status implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development task by task. Root dispatches only after the readiness prerequisite is frozen, reviewed and independently checked. This plan grants no actual private service startup.

**Goal:** Make the reviewed Core/Web usable through one native binary, with real cancellation joins and authenticated local status that never opens the database.

**Architecture:** CLI loads existing private configuration, constructs Core/Web and starts Web first. After successful Ready notification with a live context and no completed Web result, start Core. Either lifetime ending cancels both; join actual results before Core.Close. Status independently authenticates to the configured literal loopback service and prints only bounded safe fields.

**Tech Stack:** Existing native Go1.27.2 context/os/signal/net/http/encoding/json and reviewed Core/Web/config/privatefs; no dependency or permanent runtime.

**Spec:** docs/superpowers/specs/2026-10-09-core-web-design.md and docs/superpowers/plans/2026-10-09-authenticated-management.md Task4. Root inspected current composition in .tools/reviews/serve-cli-design.md. REQUIREMENTS.md/SECURITY.md/ARCHITECTURE.md remain binding.

## Global constraints

- Children ONLY GPT-6.1 Sol Medium (`gpt-6.1-sol`, reasoning_effort `medium`), no delegation/Git mutation/scanners/paid or live calls/private actual data/global changes.
- Preserve module github.com/hidxt/miskoai, brand MiskoAI, LICENSE, existing command behavior, canonical loopback binding, Host/Origin/CSRF/session controls, private directory/store lock and actual worker joins.
- No auto-login/QR/probes/retries/provider/channel/schema/deployment/resource target change. Four finite cloud probes allowance spent. Native Go only; no containers/WSL/permanent runtime.
- Existing reviewed Web/Core/config implementations are dependencies, not owned edits. Actual private schema2 must not be migrated/opened for development; all fixtures newly synthetic.
- Status is bounded direct local HTTP only, no proxy/redirect/arbitrary URL and no database fallback. Never print credential/account/user/token/session/password/body/URL-query/raw network errors or attacker-controlled strings.

## Review focus

- Occupied management port or pre-cancellation produces zero Core.Run/external worker calls and releases constructed Core resources.
- Web readiness is a historical startup event; cancellation or a completed failing Web result prevents worker startup even if Ready is closed.
- Cancellation, sibling errors and output failure join both real Run lifetimes before Close; no detached timeout cleanup.
- Status sends credentials only to configured canonical loopback origin, stores cookie/CSRF only in memory, bounds complete response bodies/total lifetime and attempts logout without restarting/extending the deadline.
- Status is safe for missing/invalid database and never creates locks/journals/migrates it; malformed/auth/error/oversized responses produce fixed safe errors.

## Task1: native CLI composition — Medium

**Owned files:** modify internal/cli/cli.go and cli_test.go, cmd/miskoai/main.go; create internal/cli/serve.go, serve_test.go, status.go, status_test.go and docs/superpowers/reports/2026-10-10-serve-status.md. Build-tagged cmd/miskoai signal helpers only if truly needed for native compilation. Do not edit other files. Root owns documentation/systemd/scans/review/publication.

**Interfaces and behavior:**

- Preserve `Run(args []string,out io.Writer)error` as a Background wrapper; add `RunContext(ctx context.Context,args []string,out io.Writer)error`. Nil context returns fixed `cli_context`; already canceled context returns its context error before config/side effects. Help/version/licenses keep their existing no-config path for valid contexts. Thread ctx into existing backup/restore; do not silently alter probe/login authorization/deadline mechanisms or invoke them from serve/status.
- Help lists serve/status. No credential flags/arguments. Native main owns signal.NotifyContext for interrupt and POSIX SIGTERM as supported, calls RunContext and stops its signal context; main prints only already-safe returned errors.
- Serve requires existing valid private Config/admin password, then Core.New, Web.New(options,web.Application(core),web.Assets()). Preserve cleanup on any construction failure. Start Web.Run(ctxChild), select Ready/actual result/context. After Ready, check ctx and any completed Web result before Core.Run. Startup output only `MiskoAI <version> listening on <canonical address>` after binding; writer failure cancels/joins before returning fixed safe error. No account/auth ID/provider credential output.
- Join results exactly once, track which lifetimes started/which results consumed, cancel sibling on either result including unexpected nil completion, join both before Core.Close and preserve safe cleanup failure. Parent cancellation with clean actual joins/Close is normal serve shutdown (nil); non-cancellation construction/lifetime/output/close failures fixed safe `cli_serve` (can use narrower fixed enums if tests/documentation define them). Never pretend hard-deadline means actual completion.
- A private per-invocation immutable lifecycle constructor/interface seam may test ordering/errors/held workers. Default production path uses actual reviewed Core/Web. No exported/global mutable hooks, caller-selected listener or replacement ownership. Real synthetic Core tests prove lock release and no network; fake orchestration alone is insufficient.
- Status uses existing config canonical Listen and private AdminPassword, never Core.New/storage/maintenance/authorization loading. Missing password safely refuses. Dedicated http.Transport with Proxy nil, fixed direct literal loopback dial, no redirects, no retries, bounded header/read/dial deadlines and total10s context. Responses max2MiB each including malformed/non2xx bodies; Close bodies and idle connections. Only fixed routes under `http://<cfg.Listen>`.
- GET /api/bootstrap; POST /api/login with JSON password/nonce and exact Origin; GET /api/status with in-memory authenticated cookie; POST /api/logout with JSON{} Origin and X-MiskoAI-CSRF. Token responses must be complete valid JSON with exact canonical43-char/32-byte base64url nonce/CSRF; refuse duplicate or alternate-spelling known token keys/trailing values and invalid HTTP/content type. Never retry an uncertain login/logout. Logout after authenticated failure is bounded best effort under the SAME remaining deadline/context, with no new background timeout or repeating request; do not mask a primary failure.
- Parse actual internal/web/system.go statusDTO within bounds, preserve numeric integers, validate required summary fields and output ONLY safe channel state, uptime/heap/goroutines/GC and logical attempt totals. Explicit allowlist channel_state: closed/closing/unavailable/authorization_expired/restore_paused/unconfigured/provider_unconfigured/stopped/receiving/backoff/paused/quarantined. Unknown strings refuse or fixed unavailable, never echo. Heap labeled Go heap; RSS unavailable honest (backend currently always false). Never print arbitrary nested events/error/status bodies. Unavailable process returns fixed stopped/unreachable guidance/error; no database probing/fallback.

**Tests and steps:**

- [ ] Compiled meaningful runtime RED TestServeBindFailureUnwinds, TestServeSignalJoins, TestServeReadinessOrdering, TestStatusAuthenticatedFlow, TestStatusNoCredentialLogging. Minimum scaffold only where needed for compilation; preserve RED JSON separately from compiler/setup errors.
- [ ] Lifecycle coverage: occupied configured port plus actual synthetic Core resources re-acquirable; ready+precanceled/no external calls; held actual worker/handler waits and no early Close; each sibling error/normal unexpected exit; constructor failure/output writer failure/Close failure. Register release/cancel and actual cleanup joins before assertions; preserve existing10s synthetic shutdown watchdog.
- [ ] Status coverage: actual reviewed Web auth bootstrap/login/status/logout flow with dedicated synthetic config/password/missing auth; wrong password; exact Origin/cookie/CSRF; redirect refusal and no credential forwarding; hostile env proxy ignored; canceled/deadline/unreachable; 2MiB+1/non2xx/invalid/trailing/duplicate/alternate token keys; canaries absent from output/errors; malformed numeric/state fields safe; DB sentinel bytes unchanged/no lock/WAL/SHM even when invalid/missing. No live endpoint.
- [ ] Implement minimal composition and safe transport, run focused GREEN. Existing CLI help/version/licenses/init/doctor/backup/restore and explicit PoCs remain valid; no implicit probes.
- [ ] Run `go test -json ./internal/cli ./cmd/miskoai -count=1` and `go vet ./internal/cli ./cmd/miskoai` under verified offline env; actual0 exits and named counts/skips reported. Child does not duplicate full-root/native builds/scanners. Root owns full integrated/static/native/compiled binary smoke gates.
- [ ] Self-review, full report exact RED/GREEN commands/exits/logs, source hashes/changedfiles/synthetic call counts and limitations. Freeze for fresh permitted Medium spec/quality/security/lifecycle review; no Git mutation.

## Root design ruling

Use one writer for related CLI/main serve/status composition. Stable Ready preserves Web listener ownership; per-invocation private seam enables meaningful orchestration tests without production globals. Status prints a narrow useful summary rather than arbitrary server JSON and never reads DB. All review focus cases map to named fixtures; scope larger than one method earns one fresh gate. Cost if wrong: reversible CLI ordering/status DTO/transport rework; no actual service, migration, deployment or new paid call is activated. Engineering60% until package30 fully passes its root local gate. Existing approved spec/user autonomy authorizes routine implementation; dangerous actual deployment remains separately authorized.

Task1 preflight ruling: the actual reviewed Web.checkCSRF and existing embedded client require `X-MiskoAI-CSRF`. Root corrects the initial brief header spelling to this exact existing interface; send only that header, no extra guessed alternate. Preserve server controls. Cost if wrong: logout fails safely and private session expires; actual local authenticated fixture must prove the contract before acceptance. No source/auth policy change.
