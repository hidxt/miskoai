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
