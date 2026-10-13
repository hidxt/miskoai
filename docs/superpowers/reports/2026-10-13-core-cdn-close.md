# Core owned CDN closure observation

2026-10-13 root test-only implementation, engineering **74%** unchanged. The retained CoreM1 assertion accepted any error from an invalid CDN request and did not distinguish live admission refusal from closure. The corrected test first requires media.ErrInvalid from the live client, then the exact media.ErrClosed after the actual joined Core.Close. Empty query reaches URL refusal before transport; the closed client refuses at its lifetime admission. No provider/CDN/WeChat request or actual private data is used.

## Actual evidence

All four isolated sources derive from the exact accepted355-file foundation export8c06761. Native WindowsGo1.27.2 used local/offline caches and fixed synthetic environment. Commands were `go test -json -count=1 -timeout=30s -run '^TestCoreProductionVisionSharesTransportAndMediaLifetime$' ./internal/core`.

- Original assertion with a synthetic production-copy mutant omitting only owned CDN.Close:1namedPASS/testexit0(0.34s), demonstrating the old assertion gap.
- Corrected assertion on the same missing-close mutant:1namedFAIL/testexit1(0.28s), specifically `owned CDN closed state missing after Core.Close media invalid`.
- Corrected negative control with only a test-copy mutation closing CDN early:1namedFAIL/testexit1(0.26s), specifically `owned CDN must be live before Core.Close media closed`.
- Corrected test on unchanged actual production:1namedPASS/testexit0(0.62s); scoped `go vet ./internal/core` exit0, empty diagnostics.

The wrapper exits0 for expected mutant failures; the actual Go test exits1 and failed JSON events are retained. Mutant code never enters main. Root verifies every other source byte against the accepted basis and preserves all actual commands/events/exits, stderr and isolated variants. The early preparation JSON used a generic lifetime-path label; actual complete byte comparison proves this variant changes only the test path, with production unchanged.

## Ownership and limits

Only internal/core/attachment_test.go and this report are adopted/frozen. All production Go, dependencies, workflow and notices remain exact. NativeCI38281939206/all12SUCCESS covers the preceding foundation, excluding this new test. Fresh independent review and integrated root/native checks remain required before marking CoreM1 resolved. Constructor CDN cleanup still has source evidence only; no factory seam, network transfer/cleanup observation,512MB/live or final43/45 completion claim is introduced.
