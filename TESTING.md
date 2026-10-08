# MiskoAI verification strategy

## Evidence classes
1. Mock: httptest synthetic provider/channel responses, bounded reads, error/stream parsing, timeout/cancel/rate limit.
2. Host: Go unit/integration/benchmark on Windows Go 1.27.0 amd64; SQLite migrations/FTS/backup/restore, scope isolation, dedup and restart states.
3. Native cross-build: CGO_ENABLED=0 Linux amd64/arm64 build; successful build proves compilation, not Linux execution.
4. Live cloud: explicitly provided temporary DeepSeek/Ollama keys, authorized spend; real text/vision/stream/search/usage/errors. Missing keys mean unverified.
5. Real WeChat: user QR/account authorization, controlled synthetic chats/media, reconnect/auth expiry and limits; no account supplied means unverified.
6. Real VPS: actual authorized 2 vCPU / 512MB native Linux. No container/WSL substitutes.

Run go test ./..., go vet ./..., applicable race detector, benchmarks, govulncheck, gosec or equivalent, full-history/working-tree secret scan, license inventory and manual review. Windows race may require a local C compiler; report availability honestly and use native CI evidence only after its run completes. Never treat queued CI as a pass.

## Required adversarial checks
Cross-user reads/updates/delete/export; malformed/oversized JSON/SSE; timeout cancellation and bounded goroutines; duplicate concurrent arrivals and ambiguous send recovery; 401/402/429 safe errors; SSRF/redirect traps; auth/CSRF/session fixation/login cap; XSS escaping; zip bombs/path traversal/upload sniffing; truncated/oversize media; SQLite integrity and consistent WAL backup; interrupted migration/restore.

## Remote test authorization protocol
Request server only when executable and test workload are ready. Explain purpose/functions (cold start, 100+ messages, long polling, facts/summary/media/doc parsing, timeouts, restart and soak); Linux distro/arch and minimal software (systemd optional, CA certs, ssh, native resource tools); ordinary-user access or user-run commands; temporary API keys and QR authorization only if required; exact uploaded paths, binary, data and optional systemd changes; CPU/RAM/disk/restart risk; cleanup paths and credential revocation. Obtain informed consent before any connection/action. Never read unrelated server data or require root unnecessarily.
Record OS/Go/build revision, RSS/Peak RSS/CPU/goroutines/GC/errors/OOM with native tools and opt-in loopback pprof. Targets are 80/128/192MiB; no forced-GC concealment. Cleanup only named test paths; retain user-requested evidence/data.

## Current evidence
Final Windows synthetic suite after token ACL integration:63 passed, one explicitly skipped Unix permissions case; provider/channel boundaries, scoped SQLite FTS5/CRUD/claims/backup/restore and CLI image/QR/cancellation/owner ACL tested. go vet and govulncheck passed. Linux CGO0 amd64/arm64 cross-builds and ELF architecture/no-dynamic-loader checks succeeded; Windows doctor/licenses executed and embedded notices matched bytes. Gosec reported24 findings, independently triaged rather than suppressed or called clean; see docs/research/gosec-disposition.md.

No real provider, WeChat or server test yet. Cloud/QR permission already granted; temporary keys/human QR interaction still needed. Native Linux CI is configured but unexecuted; Windows race cannot pass without gcc. scripts/Test.ps1 places Go cache/temp inside workspace to respect sandbox rename permissions. Final evidence belongs in DELIVERY.md.
