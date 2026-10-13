# Private library management HTTP implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development or executing-plans. Continuous routine development is owner-authorized; root reviews/integrates/Git.

**Goal:** Expose actual scoped local image management through the existing authenticated management boundary.

**Architecture:** Add an optional narrow library backend implemented by the existing Core adapter, preserving legacy management fixtures. Raw upload creates disabled content with empty metadata; editing metadata is a separate explicit bounded JSON mutation. Preview borrows actual fully validated bytes synchronously and joins cleanup before returning.

**Tech Stack:** Native Go1.27.2, existing net/http/Core/library/storage; no dependency, CSP, provider/channel/schema or deployment change.

**Spec:** docs/superpowers/specs/2026-10-12-private-library-design.md Core/Web section and 2026-10-13-core-library-integration.md. This task implements HTTP management only; full bundle/install/retained recovery, UI and reply remain separate.

## Global constraints

- Only child gpt-6.1-sol / medium. Engineering74%, no package39 credit from this partial task. Synthetic tests only; private schema2/credentials/spent cloud/WeChat untouched.
- Existing Server session/CSRF/exact Host/Origin, admission and10s application timeout remain authoritative; Core30s methods preserve shorter caller deadlines. Never invoke network/model/channel or create another media pool.
- Upload1..4MiB, global128 rows/64MiB and existing full image validator unchanged. No multipart/base64 JSON/filename/header metadata: the worst escaped metadata would exceed the existing8KiB HTTP header budget. Separate PATCH preserves all decoded metadata limits without enlarging that budget. Upload duplicates retain original metadata/state; no automatic PATCH after upload.
- Scope/private paths are never supplied by request or returned. Positive signed64bit IDs, usage and millisecond timestamps are canonical decimal strings in DTOs. Listing is metadata only, with explicit availability `unchecked`; only preview performs complete content validation.
- Keep current SQLite-only backup/restore routes unchanged. No public usage-record endpoint or claim of native sticker capability.

## Review focus

1. Unauthorized, foreign scope, missing capability, bad Host/Origin/CSRF cannot invoke backend reads or mutations.
2. Malformed/duplicate/escaped alias JSON, query aliases, high IDs and noncanonical integers refuse before side effects; hostile fields remain inert JSON.
3. Unknown/negative/oversized/truncated/trailing upload length and unsupported encoding/type refuse safely; exact supported PNG/GIF bytes use the real shared processor.
4. Preview emits only fully validated image bytes with exact MIME/length; read/write/cancellation/cleanup failures after headers abort instead of appending JSON or reporting success.
5. Restore/missing/deleting/disabled content, duplicate metadata and unavailable legacy backend remain honest; metadata listing never implies content availability.

### Task6: Actual management HTTP routes

**Owned four files:** modify internal/web/application.go; create internal/web/library.go, library_test.go, docs/superpowers/reports/2026-10-13-library-management-http.md. All server/auth/CSP/assets/Core/storage/library/other tests/rootdocs/Git read-only. Own ignored `.tools/reviews/library-management-http-*`; isolate from the exact root392-file Core/snapshot source and record its manifest. Root acceptance of dependencies remains a separate gate.

**Interfaces:**
- Optional `libraryManagementBackend` matches Core methods LibraryAssets, LibraryUpload, LibraryUpdate, LibraryDelete, WithLibraryAsset exactly; do not extend managementBackend or force legacy fixtures to invent capability. coreBackend already embeds Controller. Missing optional capability returns503 fixed web_unavailable.
- `libraryRoute(w http.ResponseWriter,r *http.Request,b managementBackend,q url.Values)` handles paths below; separate `libraryManagementError` maps existing library invalid/capacity/recovery/content/cleanup/closed and context/Core/storage classes to fixed safe responses, without raw callback/OS strings. Context timeout retains408; notfound404; invalid400; capacity/recovery409; unavailable503; unknown500. No existing error class change.
- GET `/api/library`: optional canonical `after`0..MaxInt64 and `limit`1..32 (default32); use Core metadata list once, refuse backend over-limit/invalid row identity, respond `{items:[libraryDTO],next_after:"canonical"}`. Next_after is last row ID or supplied after for empty; client uses a full page to request another. DTO includes id,sha256,format,bytes,width,height,frames,tags,emotion,description,state,enabled,uses,last_used_millis,availability="unchecked"; exclude Scope. IDs/uses/last_used_millis strings; bounded other scalars native numbers. Validate count and strictly increasing IDs above after before response.
- POST `/api/library`: no query; exactly one Content-Type `application/octet-stream` without parameters; no Content-Encoding; ContentLength1..4194304 known (refuse unknown before read). http.MaxBytesReader plus cancellation-checking reader passed with exact size to Core.LibraryUpload using storage.LibraryMetadata{} (disabled). Core StageLocal probes exact EOF, so truncated/trailing bodies cannot succeed. Return `{item:libraryDTO,created:bool}` with duplicate metadata unchanged.
- PATCH `/api/library`: no query; exact required bounded JSON fields id,tags,emotion,description,enabled, max16KiB total (covers escaped maximum). ID canonical string; tags nonnull array at most16 unique1..32byte UTF8 strings/no controls; emotion<=64byte UTF8/no controls; description<=1024byte UTF8/no NUL; enabled boolean only. Reuse strictRawRequestObject/requestObject and existing decimal helpers; reject invalid metadata before Core invocation; Core remains authoritative. No hashes/scope/format/dimensions/usage fields allowed. Respond existing mutationResult.
- DELETE `/api/library?id=...`: one canonical ID, existing no-body DELETE admission and strictQuery. Invoke actual delete once; safe tombstone/recovery behavior unchanged.
- GET `/api/library/preview?id=...`: one canonical ID, WithLibraryAsset(ctx,id,false,consume) allowing disabled ready preview. Callback checks actual row/image/artifact metadata, correct supported image/png,image/jpeg,image/gif, exact length1..4MiB and context before headers. Emit Cache-Control no-store, X-Content-Type-Options nosniff, fixed inline filename, Content-Length and MIME. Stream exact size with4096 scratch/cancellation checks from io.SectionReader over borrowed Artifact, no retain/reentry/manual Close. Finish callback synchronously. Any subsequent read/write/context/Manager cleanup error after headers panics http.ErrAbortHandler, using existing Server abort handling. Failure before headers maps safe error. Backend success without callback is500. No full payload allocation.
- application.go adds exact method map/query allowance/dispatch only. Common strictQuery/DELETE rules and Server auth remain authoritative.

- [ ] Add compiled semantic RED using minimal route scaffolding, with real synthetic Core/PNG/GIF and legacy unavailable fixture plus backend spies for refusal-before-effects; preserve original command/source/results. Compile/setup failures do not count.
- [ ] Implement narrow route/DTO/input/error/stream contracts in owned files only. No assets/UI/bundle/server policy edits.
- [ ] Test all five review-focus cases with actual authenticated Server/Core synthetic storage; exact duplicate unchanged, metadata/enable/preview/delete, high canonical ID spies, scope/path redaction, bad auth/CSRF/Origin/Host, unknown query/JSON/nullable tags/escaped duplicate keys, decoded UTF8/control/length quotas and body/type/encoding/length errors before calls.
- [ ] Add held real preview/Close/cancel and response-write/cleanup failure tests, registering unblock/join before assertions; verify no premature lease release or success JSON after bytes. Missing restored content remains unchecked metadata and refused preview. Keep actual shared pool/legacy fixture contracts.
- [ ] Run disjoint finite30s new library tests and appropriate existing route/auth regressions plus scoped vet with exact offline native Go. Freeze four owned files, complete source/evidence SHA256/lengths, actual commands/results/limits, then quiesce. Root fresh review/full/static/native/publication required; author never adopts main or edits Git.

Self-review: this task covers the five failure classes with actual HTTP/Core boundaries; existing authenticated same-origin preview fits img-src self and requires no CSP relaxation. Separate raw upload/PATCH gives a useful backend while preserving decoder/header/metadata budgets. Bundle/UI/reply and availability across full recovery are explicitly not implemented here.


Root contract clarification applied before final freeze: invalid actual content is HTTP400/web_content; recovery409/web_recovery, capacity409/web_busy, closed/unavailable503, cleanup/unknown500 and context408. Joined cleanup dominates invalid. Core classes and existing authentication/lifecycle/resource policies stay fixed; actual author failure corrections and fresh review/root2401 evidence remain in reports.
