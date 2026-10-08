# MiskoAI delivery status

Checkpoint 2026-10-09: **Phase0 complete; Phase1 local foundation implemented. Not a release or full product acceptance.**

## Implemented
Initial13 root artifacts were committed before business code (c9a432d). Existing Apache-2.0 license preserved. Module github.com/hidxt/miskoai. CLI: version/licenses/help/init/doctor/config validate/backup/restore, explicit cloud PoCs and WeChat login/text PoC. Environment is authoritative; settings.json is a private non-secret template and is not loaded as runtime configuration.

Direct stdlib DeepSeek chat/stream/usage/inline vision and Ollama hosted search; bounded TLS, DNS pinning/public address validation, no redirects/proxy, classified safe errors, timeout/concurrency/size limits. Retry only explicit429/503 within total deadline, never ambiguous transport/send. PNG/JPEG validation rejects truncated input and bounds decoded dimensions. GIF is explicitly unsupported in this PoC.

Go WeChat text protocol derives from pinned Tencent/openclaw-weixin2.4.9: QR/status/IDC validation, authenticated long poll, sender identity/context mapping, stable text clientID and durable dedup/ambiguous states. Backend acceptance and full media/cursor/reconnect behavior are not claimed.

Pure-Go SQLite modernc.org/sqlite v1.60.1: schema1, SQLite3.53.4, FTS5, one connection, WAL/FULL, scoped facts/CRUD/export, Chinese keyword fallback, durable message states/history and consistent VACUUM INTO backup. Restore uses exact immutable read-only schema validation, integrity checks, exclusive lock and preservation of prior database. Global fact/account quotas remain required before exposing a general service.

## Actually checked
- Final integrated Windows synthetic suite after token ACL integration:63 tests passed, one Unix permissions test explicitly skipped. go vet passed. Tests use workspace .tools/tmp; the sandbox's default OS temporary-directory restore rename remains unverified (access denied in that separate attempt).
- Independent GPT-6.1 Medium review found six issues, fixed and re-reviewed within Phase1 PoC scope; docs/research/foundation-review.md records evidence. Final product audit is pending.
- govulncheck v1.8.0 final dependency rerun reported No vulnerabilities found. Gosec product scope completed with24 findings (7 mediumG304,17 lowG103/G104), no high and no compile errors; exit1 is not a clean scan. Independent reviewers assessed each as non-actionable in owner-operated local PoC; see docs/research/gosec-disposition.md. Earlier ./... scans traversed ignored development caches and were stopped, not counted as product results.
- Linux amd64/arm64 CGO_ENABLED=0 cross-builds succeeded, as did Windows build. ELF64 architecture verified62/183 with no dynamic/interpreter segments; Windows embedded notices match source byte-for-byte. Hashes/sizes in docs/research/build-evidence.json. Windows version/doctor/licenses executed. Cross-build is not native Linux runtime evidence.
- Scoped SQLite FTS5/CRUD/claims/restart and consistent backup/strict restore passed. Chinese retrieval benchmark100 synthetic facts: approximately762009ns/op,23885B/op,163allocs/op on Windows N5105, not a512MB/RSS measurement.
- Public repository/name checked; no obvious serious AI brand collision found (not trademark clearance). Final pre-commit Gitleaks working-tree388333bytes and full existing2commit history43019bytes reported no leaks; policy history90contents and staged64contents reported0findings. New code commit history is scanned again before first push. git diff --check corrected an extra license EOF blank; no sensitive artifacts are staged.

## Mock / unverified / blocked
All provider/channel requests so far are mocks. User authorized a single bounded live batch: one synthetic text/stream/PNG/search each, explicit429/503 retries≤2, total≤12 HTTP attempts, plus human QR authorization. Temporary keys are not present at this checkpoint. No real cloud request or QR authorization has executed. Never publish credentials, private QR links, tokens, real messages or raw responses.

No integrated chat service, durable receive-batch cursor, summaries/personality/tool orchestration, Web/auth/CSRF, document parser, sticker library, systemd installation or release yet. Media/reconnect/auth-expiry live tests remain open. Phase2–8 acceptance is pending.

Windows race requires absent gcc and was not passed. Native Linux CI has tests/vet/race/govulncheck/history secret policy/cross-builds, pinned Actions and no containers; it has not run and is not a pass. Unix permissions are unverified on this host.

Native Linux and real2vCPU/512MB memory/soak/recovery tests: **waiting for an authorized supplied server**. No server accessed; targets idle<80MiB, normal<128MiB, sustained<192MiB remain targets.

## Resource/platform/dependencies
Development binaries reside in ignored dist/, not published releases. No permanent non-Go runtime/cloud SDK is required by the application. Dependency versions/licenses/original notices are in docs/research/dependency-licenses.md and embedded licenses output. Python/PowerShell/scanners are development tooling, not server requirements. Windows QR token ACL helper protects/reads back owner-only DACL before writing and rejects preexisting readers; four real Windows synthetic ACL tests plus independent rerun passed. API key helper creates protected ACL atomically; synthetic creation/write/read-back passed. Actual interactive terminal/real-account behavior remains unverified. Linux authorization files use0600 in dedicated0700 directories.

Restore assumes a stopped service, restricts snapshots to256MiB/exactschema1 and preserves existing data on rejected candidates. Future-schema migration needs review. Existing parent directories are never chmodded to disguise unsafe permissions.

## Next gate
Execute the already-authorized finite live tests when local keys and human QR interaction are ready, recording only redacted counts/status. Expand Phase1 media/recovery evidence before independent adapter acceptance. Continue independent work without inventing live results. Request VPS consent only once a representative integrated workload is ready. Actual commits/remote state are verified through git log/status; no release is published by this checkpoint.
