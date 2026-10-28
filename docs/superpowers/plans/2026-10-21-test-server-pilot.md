# MiskoAI isolated test-server pilot plan

> **For agentic workers:** Execute the existing native operations task in coherent stages. Use superpowers:executing-plans and verification-before-completion as applicable; owner continuous-work instructions override redundant process approvals. No product feature/code change is planned.

**Goal:** Deploy the reviewed candidate privately on the owner's supplied test server for hands-on feedback before any public release.

**Architecture:** One native Go process under a fresh locked unprivileged test account, fresh private SQLite/config/data, authenticated loopback management through same-port SSH. Root SSH performs scoped installation only. A separately reviewed system unit runs with explicit User/Group and no automatic restart or boot enable; this is a test installation adaptation, not the packaged user-manager template or a linger policy change.

**Tech stack:** Existing Debian12/systemd252/OpenSSH; verified0.1.0/Go1.27.3 Linuxamd64 artifact fromdce3c1b. No package installation or additional product runtime.

**Spec:** REQUIREMENTS.md, SECURITY.md, TESTING.md, docs/operations/native-linux.md; current owner explicitly deferred release and requested a test-server deployment/full-flow trial.

## Global constraints

- Engineering remains98%=49/50; public release50 is deferred by owner, no tag/Release/upload.
- Use only fresh /home/miskoai-test and /etc/systemd/system/miskoai-test.service after verifying absence. Preserve earlier server fixtures/swap and all old local data/auth.
- No root application, public HTTP binding, firewall change, package installation, automatic restart/boot enable or user-manager linger change.
- Initial management stage has no provider keys/auth. Fresh server configuration uses explicit127.0.0.1:18790 anddeepseek-flash. Scoped features require subsequent intentionally authorized newQR.
- Secrets stay in owner-private files or hidden input/stdin; no chat, arguments, console logs, Git or public reports. Existing local credentials/auth are not automatically copied.
- Test unit externalMemoryMax192MiB protects the small supplied host; no changed product caps or claim of final worst-case memory acceptance. OOM stops without automatic restart; preserve state for reconciliation.
- Codex children onlygpt-6.1-sol/medium, public inputs/owned outputs. Announce exactmodel/effort andxx% before dispatch/resume. Primary alone handles server/keys/actual deployment.

## Review focus

Missing/forged inherited credentials must not activate workers; use cleaned process environment. Existing account/path/unit collisions must refuse. Port consistency is exact across settings/unit/tunnel/browser. Startup success requires actual authenticated status and zero worker/provider attempts, not only systemd active. Unknown install/start/write outcomes are inspected without blind overwrite/retry. Keys+auth together can immediately process queued messages, so activation requires the owner's concrete scope.

### Stage1: prepare and install authenticated management

- [x] Re-read constraints and exact artifact/source/notices; independent public-source readiness review.
- [x] Pinned-host read-only server inspection: Debian12/x64/systemd252, sufficient memory/disk, target account/home/unit/18790 absent.
- [x] Prepare public launch/unit/install files, secure password handoff and SSH helper; primary+independent review before mutation.
- [x] Install only checked binary/license/notices/public launch file, fresh private directories and locked account; install/verify the dedicated nonroot test unit. Keep it disabled at boot.
- [x] Privately install random management password, init fresh settings, run version/licenses/config/doctor; then start service and verify exact owned process/listen/nonroot permissions.
- [x] Open matching localSSH tunnel, check unauthenticated refusal/login/status/logout, record zero provider calls and release old sessions. Private password viewer prepared; final owner access follows the intentional setup stage.

### Stage2: intentional account and provider activation

- [x] Obtain required owner choice for fresh serverQR and either new testKeys or explicitly approved transfer of existing local temporaryKeys; specify manual-chat/billing scope. Owner selected new hidden-input Keys plus fresh QR/normal billing; old bounded test budgets remain spent.
- [x] Stop service for firstQR under same unprivileged config; preserve localauth/DB. Owner completed fresh firstQR on server, loginexit0; auth private metadata verified. Optional offline stage was not needed after owner already approved full activation.
- [x] After explicit activation, supply keys privately and start once. Verify actual receiving state, both keys loaded, scoped management and text/vision modelsdeepseek-flash. No ambiguous-send retries or fixed paid-call promises.
- [ ] Owner verifies actual ordinary text/image/document/search flows and provides feedback; safe retries/summaries may add model calls.

### Stage3: user trial and feedback

- [ ] Provide reachable authenticated management and bot instructions, scope/limits, stop command and fresh-test-data backup/recovery procedure.
- [ ] User trials and modification feedback precede any renewed public-release proposal.

## Evidence ledger

Read-only inspection exit0; actual fresh installation/effectiveunit/config/doctor/process/loopback, nine initial HTTP checks and manager-confirmed stop passed. Owner completed new hidden-input Keys/freshQR; private provisioning/config/activation succeeded, receiving/both keys, ten further HTTP/scoped management checks passed. Installer/helper refusals and precise limits retained in [pilot evidence](../../research/test-server-pilot-verification.md). Owner confirmed the first normal AI reply; wider full-flow feedback pending. Prior3266Windows tests/nativeCI39604537169/allseven artifact checks remain separate source-bound evidence. No public release or percentage change.
