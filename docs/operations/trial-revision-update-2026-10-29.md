# Reviewed-candidate test-server update scope

Status: **owner authorized on 2026-10-29, not executed**. Engineering **98%**. Owner approved pushing the reviewed source to existing `codex/foundation`, then updating the existing isolated trial only after exact-source native CI succeeds. The fixed maintenance boundary below is approved; public release remains deferred. The executable maintenance steps still require primary implementation/review and current server precondition checks before execution.

## Fixed candidate and prerequisites

- Existing branch `codex/foundation`, base `df32a022b631ee993dcf509388ed1af46fbb6715`; exact final source commit will be recorded after local evidence/secret/staged-diff gates.
- Existing Go 1.27.3 Linux amd64 candidate, 13,885,600 bytes, SHA256 `4662ee3458227996673cb1984a4f3b9f332467279f730f804c3f4486c8909a16`.
- Full primary local 3319/0/4, vet 0, Node 61/0, known-vulnerability scan 0, consolidated independent review 0 unresolved C/I/M. Exact-source native CI/race must pass before live replacement. Public tag/Release/assets remain deferred.
- Read-only pinned-host inspection must recheck the recorded unit, UID/GID, executable hash, real single-link owned paths, process/listener, disk and available memory. Changed identity/state or an unresolved send requires investigation; do not proceed on old evidence alone.

## Proposed authorized maintenance boundary

1. Upload only the checked binary and reviewed maintenance files to a new owner-private candidate directory, with a 40MiB upload budget. Re-read remote hashes before any replacement. No credential upload/download.
2. Stop only `miskoai-test.service` once, using its existing graceful timeout. Verify inactive state, no owned process/listener, and successful stop. Timeout, unknown SSH result or residual process stops the procedure; do not force-kill or retry.
3. Create a fresh 0700 backup directory under `/home/miskoai-test`, owned by the existing trial user. Preserve the old verified binary plus private settings/authorization/provider/admin configuration locally on the server, using checked ordinary single-link files and restrictive permissions. Never print their contents or place secrets in arguments.
4. As the existing nonroot account, use the **old verified CLI** `backup --library /absolute/new/private/current.bundle`. It opens only this trial data with the service stopped, uses the application's consistency/maintenance locks, includes all current database scopes and image bytes, and closes normally. Existing protected environment values are read inside the local server process without shell sourcing or output. No worker, listener, paid provider or WeChat request is started by this backup command. Verify completion, output identity/size/hash and cleanup. No manual DB/WAL/SHM deletion or live database file copy.
5. Require at least 512MiB free disk and a 400MiB maintenance-directory budget; refuse if current content exceeds the prepared scope. Preserve every result/backup on any failure. No restore is submitted by this procedure.
6. Install into a new same-directory executable temporary file with existing UID/GID and 0700 mode. Recheck both old/new identities and hashes, then atomically replace only `/home/miskoai-test/.local/bin/miskoai`; retain the old binary. No unit, firewall, boot-enable, restart, Swap or system-package change. The new version adds no schema/dependency change; all existing data and profile identities remain.
7. Validate existing configuration with the new binary. Keep existing private password/Keys/WeChat authorization and `deepseek-flash`. The existing launcher clears username overrides, so the new account is **admin**, with the **existing password**. No credential reset or new QR.
8. Start the same unit once. This resumes the already authorized normal owner trial and can process queued/new owner messages, including billed calls/summaries and selected text-plus-image replies. Verify exact new executable/hash/nonroot process/loopback port and authenticated status without printing chats or credentials. Any uncertainty, failed startup or unavailable/expired authorization stops further actions; no automatic rollback/start retry or ambiguous send replay.
9. Reopen the same SSH-forwarded management address for owner testing. Owner checks the natural Chinese recall example and ordinary semantic-image behavior; these actual outcomes are separate from local mocks. If rollback is needed, stop and reconcile pending sends first; preserve the private consistent bundle and old binary. Never restore old dedup/send history automatically.

## Limits

The maintenance proposal makes no new universal 512MB guarantee; the existing 192MiB external service cap remains. Prior finite resource evidence and all unsupported Windows large-library restore/animated-sticker/natural-TTL limits remain. No extra finite model/WeChat probe budget is inferred from the old tests. Service start is normal owner-approved trial traffic; no automated test message is sent by the updater.
