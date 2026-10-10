# Web binding readiness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans task by task. Root dispatch after the current implementation child freezes. This plan grants no actual private service startup.

**Goal:** Give the approved CLI serve integration an exact signal that the management listener has successfully bound before any chat workers start.

**Architecture:** Retain Server.Run ownership of its canonical loopback listener, HTTP admission and actual shutdown joins. Add one read-only stable channel, closed after successful binding and launching Serve; CLI later observes this together with cancellation and the actual Run result. Readiness signals a completed startup event, not continuing health.

**Tech Stack:** Existing Go1.27.2 stdlib context/net/http/channels; no dependency, runtime, database or deployment change.

**Spec:** Existing docs/superpowers/specs/2026-10-09-core-web-design.md and docs/superpowers/plans/2026-10-09-authenticated-management.md Task4, plus REQUIREMENTS.md/SECURITY.md/ARCHITECTURE.md. Root preparation .tools/reviews/serve-readiness-design.md records inspected current Server.Run/Core.New/Core.Run contracts. This narrowly fills Task4's bind-before-external-work coordination requirement.

## Global Constraints

- Children ONLY GPT-6.1 Sol Medium (`gpt-6.1-sol`, reasoning_effort `medium`), no delegation/Git/network/private credentials or data/global changes.
- Preserve accepted Server.New/Run/Handler interfaces and canonical loopback/auth/Host/Origin/CSRF/password/deadline/handler32-admission policies; existing single-use lifetime and actual joins remain.
- Native Go only, no containers/WSL or extra permanent runtime;512MB/RSS and actual service remain separate unverified gates.
- One stable notification channel per successfully constructed Server, no callback, exported mutable hook, arbitrary supplied listener or polling/repeated binds.
- Routine internal design is already authorized by user master task/D006/AGENTS; final integration/publication remains root-owned.

## Review Focus

- Occupied configured address refuses binding without signaling readiness; later CLI must not start Core.Run.
- Already-canceled Run returns through existing semantics without signaling readiness or binding.
- Readiness is stable before/during/after Run and closes once; a second Run retains existing refusal.
- Successful readiness does not allow abandoned HTTP handlers or listener leaks on cancellation.
- Readiness cannot be mistaken for permanent availability: CLI must observe Run termination and cancellation even after the signal.

## Task1: Stable successful-bind signal — Medium

**Files:** modify ONLY internal/web/server.go and internal/web/server_test.go; create docs/superpowers/reports/2026-10-10-web-readiness.md. No application/assets/Core/CLI/media/config/storage edits.

**Interfaces:**
- Produce `func (s *Server) Ready() <-chan struct{}` returning the same channel allocated by successful New. Calling on nil Server is unsupported, consistent with Handler/Run.
- Run closes it exactly once, only after `net.Listen` succeeds and the HTTP Serve goroutine is launched. Bind failure, nil-context refusal, pre-canceled Run and refused duplicate Run do not newly close it. A successfully ready instance remains historically ready after shutdown; this is a startup event only.
- Consumers must concurrently observe Ready, Run result and their context. A canceled consumer must never start workers merely because Ready is also closed. The eventual CLI integration owns sibling cancellation and actual joining of both Run lifetimes before Core.Close.

- [ ] Write meaningful compiled runtime RED with minimal interface scaffold if needed: TestHTTPReadyStableSuccessfulBind, TestHTTPReadyBindFailure, TestHTTPReadyPreCancelled, TestHTTPReadySingleUseAndShutdown. Use only synthetic canonical loopback addresses and existing password fixtures. Stable channel is initially open; successful binding closes it; occupied port returns exact safe web_bind with channel open; pre-canceled Run preserves existing nil result/channel open; nil context safe refusal; repeat Run returns web_lifetime with no new close; cancellation joins actual Run/listener and held-handler behavior stays covered by the existing shutdown fixture.
- [ ] Run `go test -json ./internal/web -run '^TestHTTPReady' -count=1`, preserve compiled behavior RED separately from setup errors. No intentionally orphaned goroutine; register cancellation/release/join cleanups before assertions and retain existing10s shutdown watchdog needed for native StateNew/header timeout behavior.
- [ ] Implement minimal ready field/New allocation/Ready method and one close at the exact successful-bind point; use existing Run single-use serialization. Do not redesign listener/admission/shutdown or add a fake timeout join.
- [ ] Run focused GREEN, then `go test -json ./internal/web -count=1` and `go vet ./internal/web`, verified offline toolchain/caches/dedicated synthetic fixtures. Require actual exits0; report named passes/failures/skips and exact unchanged-policy boundaries. Root handles full integration/static/native checks, no duplicate author-wide scans.
- [ ] Self-review, report concrete RED/GREEN/commands/results/changedpaths/unresolved lifecycle limits and freeze three hashes. Stop for fresh permitted Medium review; root alone accepts and commits after independent checks.

## Root design ruling and coverage check

Task1: complete at local gate (base539501c, publication pending; fresh review Spec/Quality Approved0findings). Root full965namedpass/0fail/4existingplatformskip/fullvet0, targetedproductionWebstatic2MEDIUMretained/0Goerrors/0nosec/0new/0removed. Three hashes match immutable243file export. Package30 remains open; engineering60%. Root evidence docs/research/web-readiness-root-verification.md. CLI can now implement the dependent approved coordination; no private service authorized.

Ruling: a stable channel is the smallest notification that preserves Web listener ownership. A repeated-bind probe can race another process; a listener/callback API would expand ownership and allow unintended binding choices. Cost if wrong: the small notification/API and CLI sequencing require reversible rework; no actual service or deployment is activated.

Each Review Focus condition maps to the named readiness fixtures or existing actual handler-join fixture and later CLI Task4 failure/cancel tests. This prerequisite implements no CLI itself and earns no independent package30 completion credit. CLI will run Web first, await successful startup with noncanceled context, then run Core; any sibling ending cancels/joins both actual lifetimes before Core.Close. No provider/SQLite/channel/deployment/safety/resource-target change, paid request, actual database migration or real WeChat action is authorized here.
