# Natural-image trial and decision diagnostics

2026-10-30, engineering **98%**. This is a joint trial/diagnostic record, not an image-behavior acceptance claim.

## Real trial observations

The owner authorized two generated, nonprivate static PNG test photos. Both were uploaded through the visible management UI, received emotion/tag descriptions, were enabled and each passed its individual server preview. Existing persona and low-frequency preference were preserved. After the owner's celebration-context and comfort-context messages, the owner reported text-only replies. The final refreshed image list showed both ready/enabled, uses0 and last-used0. Dashboard showed receiving,4 received/4 processed/4 sent,0 duplicate/failed/ambiguous,4 logical chat attempts and no recent channel error. No previous test message was replayed.

This does not establish whether the model declined an image, returned plaintext or invalid optional advice, or whether an earlier eligibility/admission gate suppressed the image path. Existing history persists the final user-facing text, not the raw structured model response or decision reason. Re-reading it cannot recover that missing evidence. Static tracing did not establish a concrete send/selection fault; no provider/model blame or behavioral fix is claimed.

The specific natural Chinese memory-recall regression already passed owner confirmation. The optional natural-image behavior is still an open live acceptance item. Historical direct PNG capability evidence is separate.

## Bounded diagnostic candidate

Primary prepared a diagnostic-only candidate in an ignored public-source snapshot. `Agent.Result.ImageDecision` follows the existing image branches; Service retains only the latest fixed code in RAM, and the authenticated management DTO independently allowlists it. The existing collapsed state-details panel shows `配图判定码`. New requests do not gain authority from this field. No raw output, emotions, image/message IDs, paths, credentials or chat bodies enter the observation. No schema/persisted diagnostic log, provider request, token/context budget, selection condition, cooldown, send order, ACK, retry or replay rule changes.

Key codes:

| Code | Observed stage |
|---|---|
|unavailable / profile_invalid / profile_off|Capability or captured profile did not admit image advice.|
|empty_catalogue / context_limit / snapshot_error|No offered static image, context admission refused, or snapshot read refused, respectively.|
|plain_text|An accepted legacy plaintext reply did not carry structured image advice.|
|no_suggestion / invalid_advice|A valid text envelope carried no eligible suggestion, or optional advice failed validation.|
|invalid_output / output_limited / model_error / canceled|Output, finish reason, provider or cancellation prevented an ordinary image attempt.|
|not_offered / cooldown / repeat_cooldown / selection_refused|The unchanged selector refused the proposal; these codes do not bypass it.|
|image_pipeline / image_acked|Image path entered without a final ACK observation, or durable image ACK observed; inspect delivery/error state independently.|
|not_applicable / unknown / empty|Non-image handling, rejected unknown diagnostic value, or no retained observation.|

A restart clears these process-local observations. They cannot reconstruct the completed trial messages, prove that an image was suitable, or imply owner receipt.

## Verification ledger

- Existing affected Agent/Core/Storage synthetic checks passed before the diagnostic change. An initial command used the user-level default Go cache and failed with access denied before package setup; corrected offline project-owned cache command passed. No dependency update or product failure is inferred.
- Diagnostic TDD: initial18 named tests failed because observations were absent, then18 passed. UI RED failed on missing diagnostic row; GREEN passed12 view tests. Initial edit helper used the wrong text encoding, leaving an incomplete candidate that failed compilation; corrected UTF-8 helper and subsequent passing evidence are separate retained receipts.
- One fresh GPT-6.1 Sol Medium review of the10-file candidate found0 Critical/Important and3 Minor diagnostic inaccuracies: rejected plaintext, snapshot failure and pre-selection cancellation. These were reproduced by real Handle tests before a single correction pass. The reviewer found no changed send/parse/claim/ACK/retry authority.
- Final corrected local suite, build, integrated-source binding and review disposition are recorded at completion below; while absent, they remain pending. The earlier candidate's full regression is retained separately.

No new native Linux CI, instrumented test-server deployment or live decision observation has occurred. A new explicit owner-approved conditional source push/update scope is needed before changing the running trial. The previous fixed-candidate update authorization is consumed; publication remains deferred.

## Owner pause checkpoint

Owner requested pause before corrected full regression completed. Both owned local regression roots and their child test processes were stopped (7 matching processes); preserve partial receipts as interrupted, not test passes/failures. Corrected22 targeted checks, vet and three platform builds passed; existing view checks total62 pass across the three scripts. The10 explicitly owned code/test files were integrated from the final isolated byte-matched snapshot. No new commit/push, native CI, server mutation or live decision observation occurred. Prepared update scope and5-file local packet remain unapproved. Resume from these existing files/receipts; do not replay any real message or reuse old update approval.

## Completed sandbox run and pending owner-native fixture proof (2026-10-31)

The resumed exact-source run completed in896.53s, exit1: **3281 named PASS /8 FAIL /8 SKIP**. All266 frozen source/test/asset inputs matched before/after; embedded notices were independently matched. The8 failure events are5 denied hardlink creations plus their parent-test failures: CLI source-admission hardlink; media stage-publication hardlink; library destination/stage install hardlinks; library physical-content hardlink. These fixture failures occur before the intended product refusal assertion. They are retained as failures, not waived or changed to skips. A cause specific to the execution sandbox versus the ordinary owner context still needs comparison.

Additional host-permission skips: configuration substitution junction, private-filesystem junction traversal, private hardlink/empty-file protection and original bundle-stage hardlink preservation. Other4 skips concern declared platform/privilege limits. No package failure beyond CLI/media/library and no stderr output occurred. Do not equate this with the prior accepted normal-host full run or a new clean suite.

A fixed PowerShell proof script was prepared under the project, not executed. It verifies the same266 input hashes and Go executable hash, strips provider/application environment, places all data/cache/temp/output in the project, refuses an administrator token and runs8 existing test functions in5 packages serially with2m per-package test timeouts. It uses only the existing project Go tool and native Windows file operations. It covers the5 failed fixture sites and4 newly unavailable fixture gates; no assertion, permission policy or product code is changed. Script SHA256: `64de58709fafc30d3b657eaee51937b4b849a48a54677b2c6a5f60418092b873`. Owner authorization to run outside the execution sandbox is pending under the new explicit authorization boundary. No new installation, external user-file access, SSH, live/paid call or deployment is part of that request.

New local verification: source-bound web-view fixture12pass/0fail; existing library/bundle view checks23+27 retain unchanged-source evidence; Gitleaks51-commit history scan exit0/no leaks. The5-file deployment packet is byte-verified and its operational deltas are exactly fixed paths/hashes plus the previously corrected owned-HOME cwd; it remains unapproved and unuploaded. Existing source-bound vet/3 builds are unchanged. Public push/new CI/trial update/live image decision/final acceptance remain separate pending gates.

## Explicitly authorized normal-owner supplement completed

Owner approved only the exact prepared non-admin local test script. Primary rechecked SHA256 `64de58709fafc30d3b657eaee51937b4b849a48a54677b2c6a5f60418092b873`, then ran it once outside the execution sandbox. The administrator-token guard passed; existing project Go and project-only data/cache/temp/output were used. Exit0 in25.45s; **24 named PASS /0 FAIL /0 SKIP**, all266 input hashes unchanged before/after. No installation, external user-file read, server, model or public Git operation occurred.

Primary matched exact package/test identities across both logs: all8 original failure events now have native PASS results, and all4 host-permission skips have native PASS results. Four original platform/privilege skips remain: Windows live-lock directory rename, explicit broad-DACL coverage handled by the separate Windows-specific test, unavailable symlink privilege, and Unix permission-bit execution. This establishes that the previously unavailable synthetic link fixtures and their intended rejection assertions work in the declared ordinary-owner context. No product assertion was relaxed and no product fix was needed for these fixture failures.

Keep the original sandbox full run and all interrupted attempts. This is a completed sandbox suite plus a bounded ordinary-owner supplement, **not a single clean native full run**. Diagnostic22 tests are part of the passing integrated source checks; the unchanged-source Node62, vet0 and three verified builds retain their recorded scopes. Review's3 diagnostic Minor findings were corrected and verified; primary integration and operational constant/cwd-delta checks are complete. Candidate binary remains13,893,792 bytes/SHA256 `bbdb154e49e5aeaa6d1822ff3e174501d57835ec2f1c5f48e5965926efa732eb`.

Local preparation is complete at these stated gates. New exact-head Linux CI, any public source push, supplied external SSH-key use, trial-server replacement and real image-decision observation remain pending explicit authorization; public Release remains deferred. The real natural-image text-only cause is still unknown until an instrumented candidate is separately approved and observed.

## Deployed diagnostic and fresh real observation

Owner-approved sealed24-file commit `bf5210387ff7354c12721bdc8c19d2ac3a68755b` was pushed normally. [Exact native CI41115905560](https://github.com/hidxt/miskoai/actions/runs/41115905560), job124269944986, completed all12 steps successfully, including nativeLinux race, vulnerability and history checks. The compiled candidate is bound to267 source/test/asset/notice inputs:257 raw matches and10 CRLF/LF-only differences, with0 other differences. These are LICENSE; library/bundle_io.go; storage/attachments.go,image_replies.go,library_backup_test.go,schema4_test.go,schema5_test.go; web/assets/app.js,style.css; scripts/test_web_view.js. No reproducible/CI-binary-byte-equality claim.

Only after exact CI acceptance, primary used the explicitly authorized SSH key against the pinned host. Fixed shell syntax, old process/hash/listener/resource identity and10 management observations passed. The5 files (13907268bytes) were uploaded to a fresh private packet directory and all size/hash/ownership checks passed. Fresh stop preflight passed; one graceful stop confirmed schema6,0 unresolved messages and0 unreconciled image receipts. Complete consistent database-plus-current-images bundle and old binary/private input copies remain on the server. Fixed `bbdb154e49e5aeaa6d1822ff3e174501d57835ec2f1c5f48e5965926efa732eb` installation and protected config validation succeeded, followed by one successful start and13 management observations. Process53036 is nonroot/receiving on127.0.0.1:18790, Restart=no/NRestarts0, existing192MiB external guard; sampledRSS/HWM18920KiB andSwap0 are one post-start snapshot, not a new resource workload acceptance. No system/firewall/Swap change, history restore, manual journal deletion, extra paid-test batch, failed remote retry or rollback occurred. Normal owner trial billing resumed; public Release remains deferred.

Owner then sent one fresh joy-context test and reported text only. Read-only authenticated status showed **image_decision=plain_text**, receiving,1 received/1 processed/1 sent,0 failed/ambiguous,1 chat attempt and0 summary/vision/search attempts. The raw chat and raw model response were not read or saved. By the actual code branch, a captured-catalogue eligible request reached image-advice parsing, but returned accepted plain text without an image ID; selection/upload/send was not entered for this turn. No old message was replayed.

Source `DeepSeek.request` omits response_format; [official Chat Completions](https://api-docs.deepseek.com/api/create-chat-completion/) states that this leaves plain-text output. [Official JSON Output](https://api-docs.deepseek.com/guides/json_mode/) documents explicit json_object output plus a prompt example; its sample uses deepseek-flash. This supports a narrow format-contract correction, not a guarantee that every utterance warrants an image. The [concrete JSON-mode design proposal](../superpowers/specs/2026-10-31-image-json-mode-design.md) awaits the owner's explicit provider-scope decision. No provider/product code for that proposal has been changed or sent to the live API.
