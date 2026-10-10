# Web successful-bind readiness prerequisite

2026-10-10. Author: `gpt-6.1-sol`, reasoning effort `medium`. Dispatch baseline: `539501ca5251cf41f593c8c869dfa3dedb27c631`. Engineering remains **60%** (30 of 50); this prerequisite earns no independent package completion credit. Implementation and scoped local Windows evidence are supplied below. Root integration, fresh dual review, publication, native CI, live tests, resource measurements and final acceptance remain separate gates.

## Scope and result

Owned paths only:

- `internal/web/server.go`
- `internal/web/server_test.go`
- `docs/superpowers/reports/2026-10-10-web-readiness.md`

`New` allocates one stable channel per successfully constructed Server. `Ready() <-chan struct{}` exposes its receive-only view. `Run` closes that channel immediately after successful canonical loopback `net.Listen` and launching the HTTP Serve goroutine. Existing single-use Run serialization prevents a second close. A bind failure, nil context, pre-canceled context or refused duplicate lifetime never newly signals startup. After successful shutdown the same channel remains closed, recording historical successful startup.

Readiness is a startup event only. Later CLI coordination must observe context cancellation and actual Run termination concurrently, recheck cancellation before starting chat workers, cancel sibling lifetimes and join their actual returns before Core.Close. No CLI behavior is implemented here.

Existing New/Run/Handler interfaces, canonical addresses and password rules, Host/Origin/auth/session/CSRF controls, handler admission cap32, HTTP deadlines/header limit, cancellation and actual handler joins remain unchanged. No dependency, provider, storage, channel policy, deployment or512MB target changes. Image six files and other accepted work were not edited. No global mutable hook, callback, caller-supplied listener, polling/repeated startup binds, extra production goroutine or fake timeout join was added.

## Execution and decisions

Read the dispatched brief and mandatory AGENTS/REQUIREMENTS/SECURITY/ARCHITECTURE/TASKS/MEMORY/DELIVERY, root readiness design, referenced core/Web spec, authenticated-management plan and progress document. Applied executing-plans, TDD and verification-before-completion skills, including writing-good-tests guidance.

Ruling: use the supplied narrow brief and this report as the task ledger; root owns shared plans/decisions/integration/review and forbids Git mutation, extra workspace scripts, delegation and full-root test duplication. Preserve separate focused logs under the explicitly permitted `.tools/tmp` rather than running general skill workspace/commit/review/full-project workflows. This respects the explicit ownership and verification contract. Cost if wrong: root requests reversible additional documentation or checks before acceptance.

The minimal RED scaffold consisted only of the channel field, New allocation and Ready method; it had no close. The four compiled real runtime fixtures then exercised the notification consumers. Removing the successful-bind close breaks the two success cases; signaling before Listen breaks occupied-address refusal; treating nil/pre-canceled refusal as startup breaks the cancellation case; closing again on reused Run breaks single-use behavior. New Run fixtures register cancellation and actual goroutine join cleanup before assertions. Synthetic listeners and HTTP response bodies register cleanup immediately; the HTTP client disables proxy use. The pre-canceled fixture deliberately uses an occupied configured address so attempted binding would produce `web_bind` instead of the required nil result.

The existing held-handler/admission/idle-TCP shutdown fixture remains unchanged and ran in the complete Web package. Its10s shutdown watchdog is preserved for native Go StateNew/header-timeout quiescence; its75ms held-handler no-early-return assertion and actual handler/request joins remain covered.

## Exact local environment and commands

Native Windows PowerShell in `C:/Users/auzasr/Documents/Projects/miskoai`; verified Go1.27.2 and existing offline caches. Each Go invocation used:

```powershell
$taskRoot = 'C:/Users/auzasr/Documents/Projects/miskoai'
$taskGo = "$taskRoot/.tools/toolchains/go1.27.2-verified/go/bin/go.exe"
$env:PATH = "$taskRoot/.tools/toolchains/go1.27.2-verified/go/bin;" + $env:PATH
$env:GOTOOLCHAIN = 'local'
$env:GOPATH = "$taskRoot/.tools/go"
$env:GOMODCACHE = "$taskRoot/.tools/gomod"
$env:GOCACHE = "$taskRoot/.tools/gocache"
$env:GOTMPDIR = "$taskRoot/.tools/tmp"
$env:TEMP = $env:GOTMPDIR
$env:TMP = $env:GOTMPDIR
$env:GOPROXY = 'off'
$env:GOSUMDB = 'off'
```

Exact executed commands and completed results:

| Command | Exit | Named results | Evidence path |
|---|---:|---|---|
| `& $taskGo fmt ./internal/web` | Not separately captured | No diagnostics; no extra Web file change in read-only diff | Ran before RED |
| `& $taskGo test -json ./internal/web -run '^TestHTTPReady' -count=1` | 1 | 2 pass / 2 fail / 0 skip; compiled runtime RED | `.tools/tmp/web-readiness-red.jsonl` |
| `& $taskGo test -json ./internal/web -run '^TestHTTPReady' -count=1` | 0 | 4 pass / 0 fail / 0 skip; focused GREEN | `.tools/tmp/web-readiness-green.jsonl` |
| `& $taskGo test -json ./internal/web -count=1` | 0 | 82 pass / 0 fail / 0 skip; complete Web package | `.tools/tmp/web-readiness-web-tests.jsonl` |
| `& $taskGo vet ./internal/web` | 0 | Empty diagnostic output | `.tools/tmp/web-readiness-web-vet.txt` |
| `git diff --check` | 0 | No whitespace errors; read-only inspection | Console |

Test JSON was piped to `Out-File -Encoding utf8` at the specified paths; `$LASTEXITCODE` was captured immediately after each Go command. Vet used `*> .tools/tmp/web-readiness-web-vet.txt`. Named terminal events were parsed from all three completed JSON logs, counting only events carrying a Test field; package events were excluded.

RED named results: `TestHTTPReadyStableSuccessfulBind` FAILED1.01s and `TestHTTPReadySingleUseAndShutdown` FAILED1.00s, both with `successful binding did not signal readiness`. `TestHTTPReadyBindFailure` and `TestHTTPReadyPreCancelled` PASSED. This was executable missing behavior, not setup/compiler failure. The RED package elapsed3.382s and command exit1.

After adding only `close(s.ready)` at the required point, focused GREEN passed all four names (stable-success0.01s; others0s), package elapsed1.211s/exit0. Complete Web GREEN passed all four readiness names and `TestHTTPHandlerAdmissionAndShutdown`5.37s; package elapsed11.146s/exit0, total82 named passes. No failure or skip was omitted. Scoped vet completed exit0.

## Self-review and frozen handoff

Read-only source/test diff inspected after GREEN. Production diff is one private field, channel allocation, documented receive-only method and single successful-bind close. New test result publication uses close(done) before result reads; fixtures join actual Run returns. No existing test was rewritten. Read-only overall diff showed only the two owned code paths before adding this report.

Frozen source SHA256:

- `internal/web/server.go`: `d6ca4dd0c8633ee245bf91e95612f4238091ed8218a758e413398162f5c9648c`
- `internal/web/server_test.go`: `e679b954a68d9d04641b0c60e9d7f22e54ce386d9b00d2997142f6e158fca609`

The third frozen hash is this report's SHA256, emitted in the final handoff after writing its final bytes; a report cannot embed its own final hash. Root must verify all three immutable bytes independently before fresh review and integration. No author code change is planned after freeze.

Limitations: author self-review is not independent acceptance. Root must independently check integrated source and fresh spec/quality review. No full-project suite, race check, scanner, secret scan, Linux build/runtime, native CI, actual private service/credentials/database, provider/WeChat/CDN request, RSS measurement or final acceptance was performed or inferred here. Newly allocated loopback addresses can be claimed by another process between fixture reservation and actual Run bind; such a conflict correctly causes test failure rather than changing production binding policy. Calls to Ready on nil Server remain unsupported, consistent with Handler/Run. Cancellation can race a successful bind: consumers must check their context and Run result even when Ready is closed.
