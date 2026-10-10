# Attachment storage root verification

2026-10-11. Engineering progress **62%** remains31of50: Task1 supplies durable descriptor storage; package34 additionally requires Task2 pure normalization. This is local acceptance, not an operational image/document pipeline or actual private database migration.

The immutable candidate consists of255public files in ignored `.tools/reviews/attachment-storage-reviewed-src`, eleven owned SHA256 entries in `.tools/reviews/attachment-storage-owned-hashes.json`, and61380byte patchSHA256 `8f6a67c450e729fa52bcb3d6f1b622bb7fd606ff56f8b3f62e1b416524cdebf5`, against9a0bbaa80b19326a60df88b3a8f372ee7e6c58a2. Root mechanically checked every owned workspace/snapshot hash before and after checks. Root read the complete new attachment implementation/tests and schema4 tests, complete modified production paths and exact changed existing assertions/diff; the fresh reviewer independently read all ten complete Go files and surrounding storage/maintenance/service/agent/export contracts.

## Independent review

Fresh `attachment_storage_review`, ONLY GPT-6.1 Sol Medium (`gpt-6.1-sol`, reasoning_effort medium), returned Spec Approved / CodeQuality/schema/security Approved,0Critical/0Important/0Minor. The owner pause interrupted the first readonly pass; SAME reviewer resumed the unchanged candidate after explicit owner continuation, preserving its read progress. Reviewer changed no files and ran no tests/network/scans. Author and reviewer claims are separate from root runtime evidence.

## Actual integrated execution

Verified offline native Windows Go1.27.2, normal owner permissions for existing ACL fixtures, explicit synthetic config/key environment and no inherited credential file:

| Actual command | Complete evidence | Result |
|---|---|---|
| `go test -json ./... -count=1` | `.tools/reviews/attachment-storage-root-tests-resume1.jsonl` | exit0;1059namedpass/0fail/4existingWindows platformskips;14packagespass/2no-testpackages |
| `go vet ./...` | `.tools/reviews/attachment-storage-root-vet-resume1.txt` | exit0;empty0byte log |
| Windows complete production gosec | `.tools/reviews/attachment-storage-gosec-resume1-windows.json` and log | actualexit1;68files12823lines61warnings;11MEDIUM/50LOW;0Goerrors/0nosec |
| Linuxamd64 complete production static gosec | `.tools/reviews/attachment-storage-gosec-resume1-linux.json` and log | actualexit1;68files12641lines64warnings;2HIGH/13MEDIUM/49LOW;0Goerrors/0nosec |
| Frozen255public-file Gitleaks | `.tools/reviews/attachment-storage-gitleaks.json` | exit0;~2184280bytes0findings |
| Current working public policy | `python scripts/secret_scan.py` | exit0;255contents0findings(valuesredacted); no publication claim |

Four unchanged platform skips: privatefs.TestPrivateDirNeverRepairsExisting, privatefs.TestPrivateSymlinkTraversal, maintenance.TestLockRejectsMovedDirectory, storage.TestExistingParentPermissionsArePreserved. Names/counts include actual top-level tests and subtests; no added skip. Author's scoped480namedpass0fail2skip/vet0 is separate evidence. Both command exits and complete JSON terminal package events were inspected.

The first root runs were stopped for the owner's explicit pause: sessions89504/61140/15238 completedexit-1 after exactly verified task process-tree termination; their original partial logs are retained and never used as passing evidence. Resume1 uses separate complete files. No tests/scanners kept running through the pause.

## Static disposition

Compared with accepted serve/status static baseline after normalizing snapshot paths and code-line offsets, each target adds precisely2LOW G104 findings and removes0. Raw location-bearing fingerprints initially showed18new/16removed because unchanged storage cleanup lines shifted; normalization preserves file/rule/details/code while removing only numeric source-line prefixes. Root inspected the actual new `attachments.go` rows.Close calls at177/181: both run while returning an already classified scan/descriptor failure; success uses `closeValidatedRows`, propagating Err/Close. These are secondary cleanup reporting warnings, not ignored success admission or detached rows. Record them without suppression; do not claim a clean static scan. Prior findings, including two Linux HIGH-labelled UID conversions previously triaged against the target64bit ABI, retain their documented disposition. Windows-host Linux static analysis is not actual Linux/arm64 execution or RSS evidence.

## Accepted compatibility and boundaries

Compatible candidate schema4 adds only `inbox_attachments(sequence INTEGER PRIMARY KEY REFERENCES inbox(sequence) ON DELETE CASCADE,body BLOB NOT NULL)` and user_version4. Schemas1/2/3 DDL/manifests remain pinned. Existing five test assertions advance only genuinely fresh/migrated/restored-current expected versions; exact legacy refusal/immutability fixtures remain. D049 records bounded ownership; D050 preserves all pending receive evidence on ClearMemory/ClearDerived; D051 preserves old ValidateBackup1/2/3 versus independent Open admission. derived.go and its original clear regression remain byte-identical toHEAD.

Metadata is canonical bounded16KiB, SQL type/length/count/FK/combined global8MiB and1024rows admission precedes descriptor materialization; scope/dedup includes every historical message state. Descriptor insertion, inbox, cursor and raw-frame deletion share one transaction; changed duplicates cannot overwrite metadata. Scoped CompleteInbox alone cascades the descriptor. Empty genuine caption remains empty; metadata never becomes fact/history/candidate USER evidence.

Operational receiving remains reviewed text-only. New source development publication/nativeCI remains pending; accepted CLI9a0bbaa/nativeCI38067031368/job114256653293 already passed EVERY configured step and excludes this candidate. Actual private schema2 remains untouched. No actual credentials/auth/database were loaded, no general product service ran and no provider/CDN/WeChat calls occurred. Paid live allowance is spent; WeChat explicitly deferred. Actual arm64 execution,512MB server workload, final integrated/security/license/UI/artifact/release acceptance remain open.

Publication preflight found Git's configured text normalization converts the author report's CRLF bytes toLF. This is report-only: all ten reviewed Go/test index blobs remain byte-exact SHA256 matches, and normalized report bytes are independently compared to `workspace_report.replace(CRLF,LF)` with no other difference. Preserve original author hash for review and record separate actual publication hash; do not mislabel eleven published blobs byte-identical. Initial index-export assertion refused the report newline mismatch; that candidate is retained and not used as publication evidence. Fresh final export verifies every indexed blob SHA1 and the ten code hashes plus exact normalized report equivalence before scans/commit.
