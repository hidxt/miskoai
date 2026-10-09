# SDD ledger — plan: docs/superpowers/plans/2026-10-09-runtime-configuration.md

## Preflight
| Task/interface | Produces and consumes | Finding / ruling |
|---|---|---|
| Task1 itself | privatefs Create/Read/checks, old CLI token helper | Existing helper tests require empty-handle protection; add ProtectEmpty preserving reader refusal. Create remains atomic-before-payload. |
| Task2 itself | strict saved settings, explicit credentials and auth | Public Settings cannot contain private fields; missing/invalid auth distinct. Environment presence authoritative. |
| Task1 → Task2 | owner-only directories/files and bounded Read | Settings/credential/auth readers use opened-handle verification before payload; no guessed paths. |
| Task1 → future Core/SQLite | private parent, DB, WAL/SHM | SQLite creates child files with inherited DACL; parent is protected owner-only and every child allow entry must be current-owner-only. New app files protected; broad grants rejected. |
| Task1 → Task2 CLI ownership | operations.go | Serial implementation/review gates; no concurrent edits. |

Ruling: factor ProtectEmpty to preserve reviewed token compatibility — avoids copied unsafe ACL algorithm — cost is a narrow additional API with synthetic existing-reader tests.
Ruling: Windows children may retain verified owner-only inherited DACL under a protected owner-only dedicated parent — SQLite engine-created journals must be private before bytes, and mode bits cannot prove this — cost is explicit ACE/owner/parent checks rather than assuming protected-file flags alone. This does not permit another principal or silently repair existing broad grants.

Both tasks not started. Wait for the active memory integration implementation/review gate before serial dispatch. Root uses existing codex/foundation isolated from main; development-only Windows helper scripts substitute for unavailable Bash skill helpers. No actual private payload access by children.

Task1 started after local Memory Task3 gate/commitc17c6ea: freshGPT-6.1 Medium /root/privatefs_worker, exact privatefs/CLI ownership, synth-only. Task2 notstarted. Prior358fc7a nativeCI passed; c17c6ea publication/nativeCI stillpending. No actualdata/credential reads or migration bychild.

Privatefs Task1 interrupted by account usage limit before completing implementation/report. Owner returned and explicitly requested continuation; root resumed original /root/privatefs_worker on the same GPT-6.1 Medium configuration, preserving TDD progress and exact ownership. Root announced model/effort and whole-project progress per new standing instruction. Previous c17c6ea push automatic approval failed due usage review unavailable, not an unsafe verdict. After retry window elapsed and owner continuation, normal approval succeeded and c17c6ea pushed; no bypass or uncommitted privatefs publication.
Task1 ownership correction: cli_test.go is the existing synthetic fixture file; operations_test.go does not exist. Root approved fixture-only adaptation, no weaker privacy assertions.

Task2 preflight refinements before dispatch: InitSettings is exclusive-create while SaveSettings deliberately replaces, preserving CLI init's old no-overwrite contract. Strict LoadAuthorization also replaces the old probe's first-value/mode-only reader; bot_token/account/allowed_user/base_url exact schema and fixed-marker/one-send behavior remain. Key absence permits diagnostics; explicitly empty invalid env refuses instead of falling back. Canonical keys reject case/Unicode aliases. These ordinary internal choices add targeted tests/ownership, no actual credential access or external effects. Task2 remains notstarted pending Task1 review.

Task1 pre-freeze root checks: approved a synthetic unsafe-sidecar backup regression and metadata-only main/WAL/SHM CheckFile before storage.Open, with no new physical DB cap. Root source inspection identified maxBytes+1 overflow in Read at MaxInt64; implementer had also identified metadata-limit/read-limit distinction and is adding focused overflow/complete-length tests. CheckFile retains MaxInt64 metadata use; Read refuses unrepresentable max+1. These fixes remain inside the owned task and receive the same fresh review gate; Task1 not yet accepted.

Privatefs Task1 local accepted: fresh independent GPT-6.1 Medium spec/quality approved0findings; root full verifiedGo1.27.2 Windows494pass/0fail/5explicit platform skips, fullvet0. See docs/research/privatefs-review.md and task report for privacy ordering/ACL evidence. NativeLinux for newpatch pending; c17c6ea nativeCI remains preceding checkpoint. Strictconfig Task2 next; scopedadmin/Core/Web/media/realWX/512MB/finalrelease stillpending. No actualcredential/DB access or migration.
