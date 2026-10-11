# Web cancellation fixture scheduling correction

Engineering progress remains **70%** (35 of 50 packages). Author: ONLY GPT-6.1 Sol Medium (`gpt-6.1-sol`, reasoning effort `medium`). This is a test-only implementation and local Windows verification handoff; independent review, root integrated checks, publication, native Linux race CI, live tests and final acceptance remain primary-owned gates.

The existing native Agent run 38136011777, job 114459903175, failed `TestDownloadHTTPClientCancelsDuringTransfer` at its five-second cancellation notification expectation, without a `WARNING: DATA RACE` report. The precise socket schedule in that run is unknown. This work proves a deterministic schedule of the same failure class, not a replay of that exact CI execution.

## Root cause and finite proof

The fixture flushed an 8192-byte partial response and the client read 4096 bytes before canceling. Those events do not establish that `io.CopyN` has entered the underlying reader's second `Read`. Production `downloadReader.Read` checks request cancellation before invoking the underlying read. If cancellation becomes visible first, that valid guard returns the cancellation error, closes the artifact and releases admission without entering `cancelDownload.Read` again. The old fixture nevertheless demanded the underlying `canceled` notification and its held-reader lifetime.

Before changing the product fixture, an isolated copy of accepted `.tools/reviews/core-image-reviewed-src` was created as `.tools/reviews/web-cancel-native-red-src`. Diagnostic-only channels observed second-read entry. A writer barrier flushed the first bytes, waited for the real HTTP request context to cancel, then returned the successful write count with no error. Production source and cancellation guards were untouched. The original cancellation expectation was retained.

The compiled RED run logged `deterministic schedule: canceled request closed artifact without entering underlying second read`, then failed with `server reader did not observe actual client cancellation`. There was one named failure, zero named passes/skips, exit **1**; test duration 5.20 seconds. The focused Go watchdog was 15 seconds, all diagnostic waits were at most five seconds, and deferred cancellation/barrier release let the server close normally. This is a semantic assertion failure, not a build error or watchdog timeout.

## Test-only change

The only existing source file changed is `internal/web/system_test.go`:

- `cancelDownload` signals `entered` at its second underlying `Read`. The held-read test waits for that signal before the actual HTTP client cancels. Its cancellation observation, artifact-still-open assertion, active admission of one, no early join assertion, real release, actual close/join and close-once assertions are retained.
- `TestDownloadHTTPClientCancelsBeforeNextRead` supplies a real synthetic HTTP request and first-write barrier to force the complementary schedule. It observes cancellation at that barrier, permits the next production wrapper read, and checks that the underlying second read was avoided. It verifies an errored, incomplete client body, actual handler return, artifact close once, actual join and released admission. A complete successful body would fail the test.

Both fixtures release their held barriers and cancel the client on failure paths. The barrier has a five-second watchdog; no sleeps, retries, high-count loop or production timeout increase was added. The tests would catch premature artifact close/admission release in the held-read path, a removed cancellation guard in the before-next-read path, missing cleanup, or a false complete transfer.

## Actual local commands and results

The explicit new runner `.tools/reviews/web-cancel-native-run.ps1` used `.tools/toolchains/go1.27.2-verified/go/bin/go.exe`, native Windows amd64, `GOTOOLCHAIN=local`, `GOPROXY=off`, `GOSUMDB=off`, and `.tools/{go,gomod,gocache,tmp}` caches. It cleared inherited `MISKOAI_*` variables by name without printing their values; Web tests configured synthetic inputs themselves. No historical runner was sliced or overwritten. Each invocation saved actual exit/timestamps/arguments/source metadata, stdout and stderr under its own prefix.

| Phase | Go arguments | Named results | Actual exit |
| --- | --- | --- | --- |
| Diagnostic RED, isolated accepted copy | `test -json -count=1 -timeout=15s -run ^TestDownloadHTTPClientCancelsDuringTransfer$ ./internal/web` | 0 pass, 1 fail, 0 skip | 1 |
| Corrected focused, isolated accepted copy plus owned fixture | `test -json -count=1 -timeout=15s -run ^TestDownloadHTTPClientCancels(DuringTransfer\|BeforeNextRead)$ ./internal/web` | 2 pass, 0 fail, 0 skip | 0 |
| Complete Web package, same corrected copy | `test -json -count=1 -timeout=60s ./internal/web` | 83 pass, 0 fail, 0 skip; 1 package pass | 0 |
| Scoped vet, same corrected copy | `vet ./internal/web` | empty stdout/stderr | 0 |

All test-run stderr files are empty. Actual metadata files are `web-cancel-native-{red,focused,web,vet}-exit.json`; test JSON events are the corresponding `.jsonl` files. Vet's `.jsonl` is an empty stdout capture, not an event stream. Diagnostic source and explicit setup helpers are retained with the evidence. The corrected full source copy excludes unaccepted PDF WIP by construction.

## Freeze and limitations

The two owned deliverables are this report and `internal/web/system_test.go`. SHA256 and byte-length manifests `.tools/reviews/web-cancel-native-owned-hashes.json` and `web-cancel-native-evidence-hashes.json` identify original paths and independent immutable copies beneath `web-cancel-native-owned-frozen` and `web-cancel-native-evidence-frozen`. The diagnostic/corrected isolated copies remain retained. No edits follow the freeze.

No production code, auth, deadlines, admission or actual join behavior changed. No Git commit/push, root ledger, Core/service or PDF edit occurred. No private database, general service startup, paid provider/CDN/WeChat/external request, arm64 runtime or 512MB measurement was involved. Full integrated/race/static/security scans were not run by this child under its narrow scope; the primary owns those gates. Native Linux scheduling remains to be checked by native CI; no native-success or final-acceptance claim is made. There are no unresolved issues within this fixture correction. Author is quiescent after freeze.
