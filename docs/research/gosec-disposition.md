# Phase1 product-scope static scanner disposition

2026-10-09. Tool: github.com/securego/gosec/v2 v2.29.0 (development-only). Command: `gosec -quiet -fmt json -out .tools/gosec-product.json ./cmd/... ./internal/...` on Windows amd64. Product source only; tests/development caches excluded by explicit package scope. The earlier `./...` run recursively traversed .tools and was terminated as invalid product scope. No suppression was added.

Final scan after synthetic-only WeChat filtering:20 source files,2548 lines,24 findings, no GolangErrors. Exit1 reflects reported findings; **not a zero-warning pass**. Seven G304 medium, one G103 low, sixteen G104 low. Independent Medium source/call-site review and storage specialist review found no actionable vulnerability in these listed findings under the current owner-operated local PoC. This does not certify the future service or replace Phase8 audit.

## G304: administrator-selected local paths (7)
- storage/store.go: configured private database or application-created candidate; privatePath validates final file/parent and owner-only Unix permissions.
- storage/backup.go (2): locally selected backup destination is exclusively created, path is bound to VACUUM INTO, completed snapshot reopened for Sync/Close; all success-critical errors checked.
- cli/weixin_probe.go: fixed private authorization filename under local data directory, regular-file/size checks, no model/remote path input.
- cli/probes.go: explicit local vision PATH, regular-file4MiB/dimension/full decode limits; only synthetic PNG used for authorized live batch.
- cli/operations.go (2): private lifecycle lock and new restore candidate; source comes from owner-operated CLI. Exact snapshot schema/integrity checked before installation.

These conclusions require the process owner to control configuration and directory ancestors. Current code does not promise complete resistance to a concurrent malicious local process replacing ancestor directory entries. Future Web/model file APIs or privileged handling of lower-trust paths require a new review.

## G103: Windows security descriptor SID access (1)
The controlled unsafe conversion reads the SID starting in a validated ACCESS_ALLOWED_ACE returned by x/sys GetSecurityInfo/GetAce. The descriptor, ACE type/count/mask and owner SID are checked; the descriptor remains alive through comparison. Runtime.Pinner holds the Go SID used as a Windows ABI trustee. Four actual Windows synthetic ACL tests pass, including existing-reader rejection. Windows administrator privileges may take ownership independently of DACL.

## G104: non-success-changing cleanup (16)
- storage/validate.go (6): result Close after failed Scan/manifest/integrity rejects is cleanup before unconditional error; complete iteration reaches EOF and database/sql closes rows automatically. Rows.Err checks include driver close errors before any success.
- storage/backup.go (3): integrity rows follow the same EOF/error path; foreign-key zero rows means EOF/Rows.Err checked, while any first row unconditionally rejects.
- cli/operations.go (5): private lock/candidate/WAL/SHM cleanup; failure cannot permit a rejected restore. Success-critical copy/Sync/Close/integrity/rename errors are handled. Cleanup failure may leave a private stale lock/temp requiring owner inspection.
- cli/verification.go (1): input Close on context cancellation is best effort; caller returns timeout, terminal restoration runs before close and its error is checked.
- cli/probes.go (1): file Close after ACL protection has already failed; no credential bytes have been written and caller returns error.

Storage EOF/close reasoning was checked against actual Go1.27 database/sql (Rows.Next/Rows.close) and modernc.org/sqlite v1.60.1 rows.Close. Reviewer details and fixed six pre-scanner findings are in foundation-review.md. Dynamic evidence is synthetic local tests; no live API/QR/Linux/VPS pass is inferred.

## Reviewed Storage3, summary and ACK checkpoint

Verified Go1.27.2 source scan:34 files,5940 lines,45 findings; zero GolangErrors, zero high,7 medium G304,1 low G103 and37 low G104. Exit1 means findings remain. The frozen source includes reviewed Storage3, summary and ACK code; agent integration is not included. No suppression was added.

The older gosec build could not read Go1.27.2 export-data version5 and its partial scan is invalid. The complete scan uses isolated development-only gosec v2.29.0 built with Go1.27.2 and golang.org/x/tools v0.51.0, whose official reader supports version5. This tool adjustment changes no application dependencies and does not use scanner AI features.

Root inspected all11 additional G104 branches: admission3.go86/90/105/109/173/178/193/197, derived.go248 and profiles.go136/141. They close rows immediately before an unconditional Scan, validation or decode error return. A cleanup error cannot admit refused input. Successful iterations call closeValidatedRows and check iteration/close errors before committing. Earlier G304/G103/G104 dispositions remain applicable to their unchanged paths, with current line numbers shifted. Summary and ACK code introduce no scanner finding. Full final service security review remains pending.

## Text-core development rerun

After storage2 and frozen text agent, product-only `gosec -fmt=json -out=.tools/gosec-text-core.json ./cmd/... ./internal/...` returned exit1. On this Windows shell its JSON was emitted to the captured output; root extracted and parsed that report into the ignored JSON artifact.25source files/3747lines,34findings:7mediumG304,1lowG103,26lowG104; no high and no Golang errors. No suppression or zero-warning claim.

Ten additional lowG104 findings are migrations.go row-Close calls at188/192/207/211/258/262/277/281/296/300. Each is cleanup immediately before an unconditional Scan/policy error return; a failed close cannot turn refused data into admission. Completed successful iterations use closeValidatedRows, checking Rows.Err and Close before continuing on the single connection. Root inspected all ten branches; prior storage2 Medium review inspected the same admission/iteration paths. New agent files produced no scanner finding, and fresh Medium task review approved its exact frozen patch. Full product/final service audit remains pending.
