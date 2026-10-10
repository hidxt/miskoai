# Attachment storage Task1 implementation report

2026-10-10 plan / 2026-10-11 Windows runtime evidence. Author: gpt-6.1-sol, reasoning_effort medium. Engineering progress remains **62%**. Task1 is scoped implementation/local evidence; package34 needs later pure normalization plus fresh review/root integrated/native gates. No native CI, live media/provider/WeChat, actual private migration, resource or final acceptance is claimed here.

## Scope and candidate schema

Durable storage only: optional private `InboxEntry.Attachment *Attachment`, shape validation, canonical bounded BLOB encoding, combined queue admission, atomic scoped insertion/dedup and scoped completion. Storage imports no network or media package. No receiver activation, downloader/parser/model/send, CLI/Core/Web/service/agent pipeline change occurred.

The precise schema4 addition is:

```sql
CREATE TABLE inbox_attachments (
 sequence INTEGER PRIMARY KEY REFERENCES inbox(sequence) ON DELETE CASCADE, body BLOB NOT NULL
);
PRAGMA user_version=4;
```

`schema`, `schema2`, `schema3` DDL remain unchanged. The previous current manifest is now explicitly `schema3Manifest`; schema4's `currentManifest` adds exactly one table object with that DDL, no additional index/trigger/AUTOINCREMENT or sqlite_sequence entry. Versions1/2/3 retain their pinned manifests; only4 includes the companion. Unknown objects and future versions refuse. Sequence, derived/profile, legacy and receive admission remain applicable for4. store.go needed no change because its version admission branches already use `schemaVersion`/`>=2`.

Global receive count remains1024 and bytes remain8MiB across ALL scopes. Stored attachment BLOB bytes are added once to inbox text/context bytes, including canonical JSON overhead. SQL counts, aggregate lengths, BLOB types, per-body16KiB, sequence ownership/orphans and relationship counts are checked before descriptor BLOB materialization. SQL admission precedes bounded decoding; the requested PendingInbox limit remains1..32 and scoped joined rows use one validated transaction. Opaque raw frame4/8MiB/2MiB per-frame limits are unchanged.

Descriptor shape: kind exactly image/file/unsupported; unsupported has empty remaining fields; supported requires valid encrypted key and query/full URL. ImageHex has precedence and persisting any unused MediaKey is invalid. Image keys32ASCIIhex; media key24/44 canonical standardbase64 decoding to16raw bytes or32ASCIIhex. File disallows ImageHex; image forbids filename/MD5/length. Query<=4KiB, FullURL<=16KiB, FileName<=512bytes, validUTF8/no controls, no surrounding full-URL whitespace; optional MD5 exactly32ASCIIhex and canonical file Length1..4194304. Individual field checks precede marshal; whole encoded body<=16KiB. Stored decode requires byte-exact canonical struct JSON, excluding null, duplicate, escaped alias, omitted, wrong-type and unknown fields. This is structural URL metadata validation, never network authorization or authenticity proof.

ResolvePoll validates entry scope/text/ID/time before descriptor encoding, encodes validated bounded descriptors before transaction, retains existing all-state messages and inbox dedup, and inserts companion only with a newly inserted inbox sequence in the same transaction. Changed duplicates cannot replace data. Cursor advance/raw deletion occur only at transaction completion. CompleteInbox keeps existing scoped deletion and foreign-key cascade. Genuine empty caption stays empty; metadata is not copied into messages/facts/candidates/history/export.

## Rulings and compatibility

D050 from the root's final brief section supersedes the original clear-removes-inbox bullet: ClearMemory/ClearDerived preserve all pending receive inbox/raw frames/cursors and linked descriptors. No failed claims, queue purge or new clear side effect is synthesized. Pending private metadata remains bounded until normal scoped completion. derived.go was restored byte-identically and is not a changed file. The unmodified original receive-preservation regression and new scoped descriptor preservation/completion test cover this distinction.

Legacy backup compatibility: pre-existing ValidateBackup admits exact schema1/2 manifests before Open independently performs full immutable legacy/receive data admission; schema3 retains its existing derived/profile/sequence checks. The initial implementation accidentally tightened legacy backup data admission, and complete runtime tests exposed that. Corrected new full data/descriptor/combined-quota backup admission is confined to schema4; no old fixture assertion was weakened. All current-version changes in the three existing owned tests are only expectations from literal3 to `schemaVersion` for fresh/migrated/restored-current fixtures. Exact legacy constructions and refusal assertions stay pinned.

The initial RED tests had the five required names and runtime assertions that the companion table existed. All five failed because schema3 lacked it (not compile failure). The later behavioral cases extend those same tests and additional tests; no claim is made that every individual behavior assertion was independently RED before implementation.

## Runtime evidence

Verified command: `.tools/toolchains/go1.27.2-verified/go/bin/go.exe version` returned `go version go1.27.2 windows/amd64`, exit0. Go calls used GOTOOLCHAIN=local, GOPATH=.tools/go, GOMODCACHE=.tools/gomod, GOCACHE=.tools/gocache, GOTMPDIR=.tools/tmp, GOPROXY=off and GOSUMDB=off. Final verification explicitly removes the fixed config override names including MISKOAI_CREDENTIALS_FILE, sets synthetic MISKOAI_DATA_DIR=.tools/tmp/attachment-synthetic, MISKOAI_ADMIN_PASSWORD=synthetic-password-123 and synthetic DeepSeek/Ollama keys; TEMP/TMP=.tools/tmp. No inherited credential file is loaded. Existing Core fixture clears its own config overrides before config.Load. Fixtures use temporary synthetic databases and register their owned connection/transaction cleanup; no actual private DB/auth/env values were inspected or printed.

Actual command executables are the verified Go path above:

| Command suffix | Complete log | Exit / named results |
| --- | --- | --- |
| `test -json ./internal/storage -run '^TestAttachment' -count=1` initial RED | `.tools/reviews/attachment-red.jsonl` | exit1;0pass/5fail/0skip |
| same initial focused GREEN | `.tools/reviews/attachment-focused.jsonl` | exit0;32pass/0fail/0skip |
| `test -json ./internal/storage ./internal/maintenance ./internal/core ./internal/agent ./internal/summary ./internal/service -count=1` first scoped | `.tools/reviews/attachment-scoped-first.jsonl` | exit1;430pass/39fail/2skip; clear deletion + legacy backup admission regressions |
| focused after D050 / expanded fixtures | `.tools/reviews/attachment-focused-corrected.jsonl` | exit0;40pass/0fail/0skip |
| intermediate scoped before legacy backup correction was compiled | `.tools/reviews/attachment-scoped-green.jsonl` | exit1;442pass/38fail/2skip; legacy backup admission regressions; filename is historical and does not imply success; its scoped vet exit0 |
| final corrected six-package scoped test | `.tools/reviews/attachment-scoped-final.jsonl` | exit0;480pass/0fail/2 existing platform skips;6 package pass |
| final `vet ./internal/storage ./internal/maintenance ./internal/core ./internal/agent ./internal/summary ./internal/service` | `.tools/reviews/attachment-scoped-final-vet.txt` | exit0;empty0byte log |

Named counts include top-level tests and subtests. Existing Windows platform skips in scoped logs: storage.TestExistingParentPermissionsArePreserved and maintenance.TestLockRejectsMovedDirectory. No skip was added or used to disguise regressions. Complete error audit corrected an earlier partial-tail progress message that incorrectly described the first scoped failure as clear-only; complete39 named failures are retained above.

Final behavioral coverage includes restart and unchanged duplicate metadata; cross-scope ID/sequence; all five message-state dedup cases; empty caption; exact8MiB combined queue plus1byte refusal/raw retention;16KiB canonical descriptor plus1; escaping expansion; canonical base64 raw/hex forms and malformed keys/lengths; oversized/type-invalid/orphan/null/unknown/duplicate/escaped-alias JSON; aggregate and row-count admission; schema1/2/3/4 exact validation and compatible synthetic migration; pinned legacy refusal byte/journal immutability; schema4 backup descriptor roundtrip and unknown-object/future-version/invalid-body refusal; malformed batch, injected SQL fault and canceled transaction preserving raw/cursor with no detached inbox; memory/derived clear receive preservation followed by scoped cascade.

## Changed files / freeze

Changed existing files: internal/storage/migrations.go, validate.go, admission3.go, inbox.go; narrowly current-version expectations only in migration_test.go, empty_poll_test.go, schema3_test.go. New files: internal/storage/attachments.go, attachments_test.go, schema4_test.go. This report is the eleventh owned file. No store.go/memory.go/derived.go/maintenance/privatefs/CLI/main/Core/Web/agent/service/media/root-ledger edits remain. LICENSE preserved. No staging/commit/push/delegation/scanner/full-root build/network/live operation was performed.

Final corrected suite and scoped vet both completed exit0. Source hashes below were captured after the final source edit and before verification completed; final mechanical verification confirmed those exact files unchanged. Report's own hash is returned outside this report to avoid a self-referential hash.

Fresh independent Medium schema/security review and root integrated/scanner/native gates remain required. Author assertions are not acceptance evidence; Task2 and the operational attachment pipeline are unimplemented. Actual schema2 migration/private startup/general service, provider/media requests, real WeChat/GIF, arm64 execution and512MB workload remain outside authorization.


## Final source SHA256

| Path | SHA256 |
| --- | --- |
| internal/storage/admission3.go | 5c476ca1807e65c5cc558d31271108405addcdaf494102c4f89a9ee5e35c0972 |
| internal/storage/attachments.go | 590469e5b401bc3a38dae1d2554e456e31b07717a3a441a1e1896e472010a27f |
| internal/storage/attachments_test.go | da82a8f59bb935a07b3465e0c7238aaeb559a6c905fd4dcdcbc39943e2584dfd |
| internal/storage/empty_poll_test.go | fa1fd49bbc0357f2d5251e75062c7ed084c6547b44501397cf6cabe535903079 |
| internal/storage/inbox.go | dab5fe66bf978090133473f0719b2684a9dca4280a7dc6f0570747d430ad4437 |
| internal/storage/migration_test.go | 17b746a429593bbf42eb88d39d778a94a0ca42d6b2de4ad79e1bcdd9cff5f0ad |
| internal/storage/migrations.go | 229f9dbd0490dee3e5dc655f263d50389289fabdf777812cca2d92c0f7ff443f |
| internal/storage/schema3_test.go | 68e977864a08285f59d268802bbd7dcf281faf25858809364bc7e5d1bb32dc88 |
| internal/storage/schema4_test.go | f7ddbea538149e07f6652a7efa7363d8f56f3344bbe01038b6ed0527b2bd9217 |
| internal/storage/validate.go | e298c89a8018ae267e6996196ffad9b09c341afd46335f9dfe5d33f136f1668e |


Status: DONE. Final six-package suite480namedpass/0fail/2existingplatformskip (43attachmentnamedpass), sixpackagepass; final scopedvetexit0/empty. All ten source/test hashes mechanically matched the report after verification. Owned source/test/report files are frozen for root fresh review. No unresolved implementation failure is known; independent review and downstream/local/native/live/final gates remain pending as explicitly listed.
