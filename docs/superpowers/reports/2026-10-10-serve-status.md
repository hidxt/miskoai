# Native serve/status implementation evidence

2026-10-10. Author: `gpt-6.1-sol`, reasoning effort `medium`. Engineering progress remains **60%**. Implementation is frozen for fresh root-directed reviews and independent integration. These results are synthetic Windows-host local evidence, not native CI, live provider/WeChat/CDN, deployment, memory-target or final acceptance evidence.

## Scope and interfaces

Changed only `internal/cli/cli.go`, new `internal/cli/serve.go`, `serve_test.go`, `status.go`, `status_test.go`, `cmd/miskoai/main.go`, and this report. Existing `cli_test.go` remains unchanged and its tests run with the scoped suite. No edits to Core/Web/config/storage/readiness files, shared ledgers, dependencies, license or module. No delegation or Git mutation. Root independently published the frozen readiness prerequisite while these distinct files were being implemented.

`Run` remains a Background wrapper; `RunContext` rejects nil/pre-canceled contexts before config or effects, keeps help/version/licenses independent of config, adds serve/status and passes caller context to backup/restore. Existing explicit PoC/login authorization/deadlines are unchanged. Native main owns `signal.NotifyContext` for interrupt/SIGTERM and explicitly stops it after RunContext before error exit. Native Windows command-package compilation passed through the scoped test command; root owns native Linux builds and compiled-binary signal smoke.

Serve constructs the real reviewed Core and Web, starts Web first, observes Ready together with cancellation and actual Web result, checks again after startup output, and only then starts Core.Run. Any started sibling is actually joined exactly once before Core.Close. Unexpected nil exits are failures; parent cancellation with clean joins/Close is normal shutdown. Construction/lifetime/output/Close failure returns only `cli_serve`. Per-invocation private constructor/interfaces provide immutable orchestration fixtures; production uses actual Core.New/Web.New/Application/Assets with no global hook. Startup prints only product/version/canonical listen address. Occupied port tests exercise actual Core resources, zero Core.Run calls, lock reacquisition and store reopening; a separate public CLI-route fixture exercises the default production constructor.

Status calls only four fixed loopback routes through a dedicated direct literal-IP transport, Proxy nil, redirect refusal, compression disabled, connection reuse disabled (avoiding transport replay of a reused connection), bounded dial/header deadlines, header cap8KiB and one total10s caller-derived context. Every body, including malformed/non2xx bodies, is read at most2MiB+1 and closed. Cookie/nonce/CSRF exist only in memory; no database/storage/Core/maintenance/authorization access. Logout uses the same context/deadline, runs once after usable authentication even on status/output failure, and preserves primary failure. Complete JSON/content type, exact token key spelling, duplicate/case alias/trailing values and canonical43-character32-byte base64url tokens are checked. Object work is additionally capped at32 keys/64-byte keys to bound unknown-field parsing. Summary numbers decode directly to integer types (fixture9007199254740993 retained); output is restricted to allowlisted channel state, uptime, Go heap/goroutine/GC, honest RSS unavailable and four logical attempt totals. Nested events and attacker bodies/errors are not printed.

Preflight discrepancy: initial brief said `X-CSRF-Token`; actual reviewed `web.checkCSRF` requires `X-MiskoAI-CSRF`. Root explicitly ruled to use **only X-MiskoAI-CSRF**, corrected brief/plan and recorded the interface ruling. Production and exact-flow fixtures use the existing header; no server/auth policy edit or guessed alternate header.

Safe status failures are fixed `cli_status` or `cli_status_unreachable: MiskoAI stopped or unreachable; run miskoai serve`. Nil direct status context returns `cli_context`; public pre-canceled RunContext returns the original context error before config.

## Verification commands and logs

All commands ran from `C:/Users/auzasr/Documents/Projects/miskoai` with this per-process PowerShell isolation (no global environment/config edits, real secret value inspection, inherited credential-file loading or automatic probe):

```powershell
$taskRoot=(Get-Location).Path
$env:GOTOOLCHAIN='local'
$env:GOPATH="$taskRoot/.tools/go"
$env:GOMODCACHE="$taskRoot/.tools/gomod"
$env:GOCACHE="$taskRoot/.tools/gocache"
$env:GOTMPDIR="$taskRoot/.tools/tmp"
$env:TEMP=$env:GOTMPDIR
$env:TMP=$env:GOTMPDIR
$env:GOPROXY='off'
$env:GOSUMDB='off'
$env:MISKOAI_DATA_DIR="$taskRoot/.tools/tmp/serve-status-isolated"
$env:MISKOAI_ADMIN_PASSWORD='synthetic-offline-password'
$env:DEEPSEEK_API_KEY='synthetic-offline-key'
$env:OLLAMA_API_KEY='synthetic-offline-search'
```

New fixtures explicitly replace the data directory with a newly created private synthetic directory, admin password and **both** provider keys; inherited settings/credential-file overrides are cleared without printing prior values. Both keys bypass credential-file loading. Config/auth/database bytes are newly synthetic. No actual private schema2 database or authorization was accessed.

Exact compiled initial RED command:

```powershell
& .tools/toolchains/go1.27.2-verified/go/bin/go.exe test -json ./internal/cli -run 'TestServeBindFailureUnwinds|TestServeSignalJoins|TestServeReadinessOrdering|TestStatusAuthenticatedFlow|TestStatusNoCredentialLogging|TestRunContextRejectsInvalidContextBeforeConfig' -count=1 *> .tools/tmp/serve-status-red.jsonl
```

Exit1; named0pass/6fail/0skip. Five required feature fixtures compiled and failed at runtime against minimal unimplemented serve/status scaffolding: bind unwind, signal join, readiness ordering, actual authenticated status and safe-error logging. The sixth context fixture failed during setup because Windows Setenv rejects a NUL value; that setup failure is **not semantic RED evidence**. It was corrected to an invalid synthetic listen address, which public nil/pre-canceled context must reject before config.

Additional resource-parser RED, written before its bound was implemented:

```powershell
& .tools/toolchains/go1.27.2-verified/go/bin/go.exe test -json ./internal/cli -run '^TestStatusObjectFieldCountBound$' -count=1 *> .tools/tmp/serve-status-object-red.jsonl
```

Exit1; named0pass/1fail/0skip (`oversized object field count admitted`). After the32-key bound, this passes in the final complete scoped suite.

Final exact commands:

```powershell
& .tools/toolchains/go1.27.2-verified/go/bin/gofmt.exe -w internal/cli/status_test.go
& .tools/toolchains/go1.27.2-verified/go/bin/go.exe test -json ./internal/cli ./cmd/miskoai -count=1 *> .tools/tmp/serve-status-green4.jsonl
& .tools/toolchains/go1.27.2-verified/go/bin/go.exe vet ./internal/cli ./cmd/miskoai *> .tools/tmp/serve-status-vet4.txt
```

Actual test exit0, **73 named pass / 0 fail / 0 skip**. cmd/miskoai compiles and reports no test files. Actual vet exit0, empty diagnostic log. Earlier gofmt covered all six changed Go files. Intermediate complete scoped logs retained: green1 exit1 (27pass/1fail Windows context fixture setup), green2 exit0 (65pass), green3 exit0 (70pass), misleadingly named `serve-status-green-final.jsonl` exit1 (72pass/1fail): the newly strengthened public status-route fixture had not installed its synthetic listen address in the environment, causing fixed unreachable. Corrected fixture environment; **green4 is the final passing evidence**. No interrupted/failed log is cited as passing.

Coverage includes `TestServeBindFailureUnwinds`, `TestServeCommandOccupiedPort`, `TestServeSignalJoins`, `TestServeReadinessOrdering`, `TestServePreCanceledAndCanceledConstruction`, `TestServeCompletedReadyPreventsWorkers`, `TestServeSiblingResultsAndConstructionFailures` (core/web error and unexpectednil, construction, Close and output failures), `TestServeActualWorkerAndHandlerJoin`, and context-before-config.

Status coverage includes actual reviewed Web/Core auth flow through public RunContext status, wrong/missing password, exact methods/Origin/cookie/CSRF/logout, status-error logout, malformed/trailing/duplicate/null/float/negative/unknown-state/RSS responses, 2MiB+1 including non2xx, exact token spelling/aliases, canary omission, lossless integers, bootstrap and login redirect refusal/no forwarding, hostile proxy environment, cancellation/deadline/unreachable, same remaining deadline for logout, malformed logout, invalid database sentinel unchanged and missing directory contents unchanged/no DB/lock/WAL/SHM creation. Existing init/doctor/config/version/licenses/backup/restore/explicit-PoC tests are included in the scoped suite.

Actual held-worker fixture registers cancellation, release and real cleanup joins before assertions. It creates synthetic authorization and real Core, injects a synthetic poll delegate that waits after cancellation, admits a real Web handler that also waits, and proves Core.Close does not begin early. Final log contains:

```text
external_calls=0 synthetic_poll_calls=1 model_calls=0 send_calls=0 actual_worker_join=true actual_handler_join=true
```

The model/channel delegates are synthetic and counted; no provider or WeChat endpoint is used. Real bind/unconfigured fixtures use newly private directories with no authorization, so cannot start a channel generation; occupied port shows0Core.Run calls. Status uses local synthetic listeners only. No network-level systemwide tracing was performed; zero external-call evidence is fixture/dependency/route evidence, not a fabricated packet capture. Existing scoped PoC tests retain their prior synthetic mocks. Watchdogs preserve the existing10s synthetic join budget without treating timeout as successful completion.

## Frozen source hashes and boundaries

SHA256 after final scoped test/vet:

| File | SHA256 |
|---|---|
| internal/cli/cli.go | 5a6c95fb4c9a44df2cb57ae05fb6e6a56e9171843cc8babd4f846e58be80ed5f |
| internal/cli/serve.go | 82c2934c6778cf7a0927378e4a4e8f20ec2c93992a633efe750ea65fda916e68 |
| internal/cli/serve_test.go | 65c183d98edf8180343cf094129fe11e58871ee09828f5ffad3f8cc6894206ec |
| internal/cli/status.go | 11bfe01dcbbc394b495ccff9ee734617d42dd9e874c290f99104e3bb9145f785 |
| internal/cli/status_test.go | 7b15c4c3ea35101ec9e76f22a8816e542146f0fe4e9b43e1b217829b8ca25165 |
| cmd/miskoai/main.go | b26264fc49ce80cd2d5a3f901dc018a3499e904ddc318739f86babc3f192ae81 |

No known unresolved implementation finding from self-review. Root must independently review the diff, perform dual fresh review/full integration/static/security/native builds/binary smoke and publication gates. No full-root tests, native Linux/race builds, scanners, history scan, public push, real service startup, live provider/WeChat/CDN call, private database migration, deployment/systemd/global setting or 512MB measurement was performed by this child. No final acceptance claim.
