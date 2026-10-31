# JSON-mode trial update: actual server verification

Engineering **98%**, historical49/50 phase gates. **The approved JSON-mode trial update is complete, and the owner confirmed text plus cat-image delivery in one natural celebration-context case without explicitly requesting an image.** A later authorized metadata snapshot succeeded but shows latest `model_error`; it cannot attribute that result to the earlier successful case. Broader live/final acceptance and the first public release remain open.

## Separately approved forward restoration and latest metadata

Owner follow-up confirms that after the successful image receipt, a subsequent message received the fixed unavailable-model notice. Thus a later user-visible model failure is confirmed, distinct from the earlier successful image case. Its underlying cause remains unknown; the snapshot and owner statement do not establish the HTTP status, precise failed-message identity or which lower-level branch failed. A [local-only finite-category diagnostic proposal](../superpowers/specs/2026-10-31-model-failure-diagnostics.md) is prepared for explicit owner scope approval; no repair or new live probe has been performed.

The owner explicitly approved one same-host loopback-forward restoration using the previously supplied key only through SSH, followed by at most6 fixed management requests. The two prepared helper hashes were rechecked unchanged, then distinct approval records were created. One `ssh -N -T` connection passed strict host pinning and restored127.0.0.1:18790 forwarding, PID11560, no remote command/automatic reconnect. The tunnel remains for owner trial.

All6 HTTP checks succeeded once: endpoint identity200, bootstrap200, admin login200, status200, logout200 and logged-out status401. Only safe status fields were retained; no chat/history, raw provider response or secret was printed/saved. No new model/WeChat test, server inspection command, upload, restart, deployment, configuration/data change or retry occurred.

| Latest process-local observation | Value |
|---|---|
| Channel | receiving |
| Received / processed / sent | 3 / 3 / 3 |
| Failed / ambiguous | 0 / 0 |
| Latest image decision / code | model_error / model |
| Logical chat / summary / vision / search attempts | 3 / 0 / 0 / 0 |

This is a delayed snapshot, not a per-message trace. It does not establish which message generated the latest code, whether all three model calls reached the provider, their HTTP attempt totals, or three successful AI generations. The earlier owner-confirmed text+image delivery remains valid. Latest model failure's underlying cause is not available in these bounded fields; no provider outage, account, JSON-mode, timeout or request-validation cause is asserted.

Local source investigation only: `internal/agent/agent.go` sets `model_error` when the selected model method returns an error, then permits one safe fixed text reply. `internal/service/status.go` counts `State=sent` as sent and keeps the result's model code; consequently failed=0 does not prove no model failure. The diagnostic is latest process-local result, not retained per-message history. The owner was asked whether a subsequent message received the fixed unavailable-model notice, without requesting chat content or replay. Further live diagnosis is not authorized by the consumed one-connection/six-request scope.

## Source and local/native evidence

Fixed source **eec21cd75228b870a130ce76a96775b3d1532936**, normally pushed to codex/foundation under explicit owner approval. Native Linux [CI41174113939](https://github.com/hidxt/miskoai/actions/runs/41174113939), job124458001006, **all12 stepsSUCCESS**, including full unit/integration, vet, race, vulnerabilities, history policy, cross-build and cleanup. All18 approved canonical blobs match;53-commit local history Gitleaks0. Tested266 source/module/embed/LICENSE inputs bind257raw+9 exact CRLF/LF-only,0other; no CI artifact byte-equality claim.

Local targeted32PASS, affected652PASS, vet0, three platform builds0 and concentrated review0 actionable C/I/M are separate evidence. Full Windows sandbox3169PASS/14FAIL/8SKIP plus3 package8m timeouts, then serial31PASS/2 hardlink-fixtureFAIL/0SKIP remain in [the local report](image-json-mode-local-verification.md). Native CI success does not erase or diagnose those failures.

## Upload interruption and separately approved recovery

Original binary upload returned unknown local/transport exception after7.74s, before any stop/backup/install/start. Primary stopped without retry. A new explicit owner decision allowed2 read-only checks and complete-only continuation. Both checks succeeded but found only11,188,224bytes/SHA3de21042…, so no writes followed. Local-only comparison proved that file was the exact approved-binary prefix, missing2,709,664bytes/SHA38676e27…; reconstruction hashes to approved full869ce5c2… . Cause of the original exception remains unestablished.

The owner then explicitly approved [one-time suffix recovery](../operations/image-json-mode-suffix-upload-2026-10-31.md). Frozen helper5434b559… had one concentrated GPT-6.1 Sol Medium review,0 actionable C/I/M, zero AST errors and exact local byte reconstruction. Fresh prefix checks and Bash syntax passed. The suffix was uploaded **once** and hash-verified. A new staging assembly matched13,897,888bytes/fullSHA, with original prefix and suffix retained under new private names. Fresh reconciliation passed; ownership/mode admission and first start.sh upload succeeded; all5 original packet files matched. No full-binary retransmission, deletion or failed-step replay.

## Actual maintenance and running state

- Fresh10-observation management preflight: receiving, effective text/vision deepseek-flash, existingadmin/password access, received=processed=sent1, failed/ambiguous0.
- One graceful stop, then schema6 read-only admission: unresolved_messages0, unreconciled_image_receipts0. No manual journal deletion or history restore.
- Existing CLI produced a consistent full database-plus-images backup, preserving old binary and4 private settings/auth/provider/management copies on the server. Full-bundle SHA256 **32fa570e246eba9f4b1bbabd02768a8008186c590c858a98a3f94d2a854d201a**, retained privately in the new packet's backup/current.bundle. No private backup or credentials transferred locally.
- Fixed install and protected actual configuration validation passed. Installed binary **13,897,888bytes**, SHA256 **869ce5c2714c52acfdaee6dacaa503d008759da534e1be24cab490b147d40488**.
- One start, then process/listener/hash/identity checks: MainPID**58897**, nonrootmiskoai-test/UID1001, active/running/receiving, NRestarts0, Restart=no, unitstatic, loopback127.0.0.1:18790 and unchanged192MiB external MemoryMax. Launcher/unit hashes match previous values. No service/firewall/public-port/Swap change.
- Post-update management **13 observations passed**: exact embedded assets, unauthenticated denial, bootstrap,admin login, receiving/provider readiness, deepseek-flash settings/defaults, bounded events/event page, logout and session denial. New process counters were received/processed/sent/failed/ambiguous0 at observation. Normal owner chat/model billing resumes under existing trial authorization; these checks do not prove a new real model reply.

The suffix authorization's complete sequence consumed exactly**16 SSH operations and23 management HTTP requests**, all steps exit0; new upload bytes**2,712,924**. Earlier interrupted staging and the separately authorized2-read-only checks remain distinct receipts. No automatic retry, rollback or extra automated paid/WeChat test occurred. Local reporting had one parse error before execution; corrected local-only receipt generation performed no additional remote operation.

Single post-start snapshot RSS/HWM**18,820KiB**, processSwap0; available memory230,812KiB. This is a point observation, not a new sustained resource acceptance run or a claim of512MiB visible RAM. Earlier resource evidence/limitations remain separate.

## Owner trial handoff

Management remains available through the scoped local forward at http://127.0.0.1:18790/ with accountadmin and the original private password. Restart may require a new login. Current Keys, WeChat binding, persona, memory/history and images are retained; schema and sender history were not restored or migrated by this update.

The owner sent one **new** natural celebration-context message without explicitly requesting an image and confirmed receiving both text and a cat image. This passes the owner-side single-case natural-image regression check. No chat body, recipient identity or raw model output is stored here. Other emotions, frequency/cooldown behavior and full F02 acceptance are separate; an image on every message is not guaranteed.

After the owner paused and resumed, the already-authorized bounded6-request metadata helper ran once. It stopped at the first endpoint identity request without capturing any HTTP status (`checks=[]`, `passed=false`), so no bootstrap/login/status/logout request followed. The failure receipt retains no underlying exception; server failure and the exact connection failure cause are unestablished. No automatic retry/chat-history read/message replay occurred.

Local process observation found recorded tunnel PID3076 absent. Native listener enumeration was permission-denied; its empty output is **not** evidence that port18790 is free. No SSH, external-key use, service command or new paid/provider/WeChat test followed. Restore the same pinned-host loopback forward only after a new explicit owner decision, then at most6 metadata requests in a fresh one-shot receipt. A delayed latest-code/counter observation cannot conclusively attribute a result to this case if intervening owner traffic occurs. Peer delivery evidence remains valid regardless of that limitation.
