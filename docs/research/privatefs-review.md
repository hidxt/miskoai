# Shared private filesystem Task1 root review

2026-10-09. Frozen base c17c6eaea41e22668357d153a0d77a8672e021d5; full task patch .tools/reviews/privatefs.diff (development-only). Implementer and fresh independent reviewer both gpt-6.1-sol / medium; no delegation.

Independent verdict: spec approved, quality approved, Critical0/Important0/Minor0. All six APIs and approved CLI integrations match the isolated brief. Root separately read all production privatefs files and tracked CLI patch. No outstanding task finding.

Evidence: root verified Go1.27.2 full Windows test -json ./... -count=1 exited0:494 passing test entries,0fail,5skip; full vet ./... exited0. Detailed synthetic output is ignored .tools/reviews/privatefs-root-tests.jsonl. Four new platform skips concern Unix-mode test on Windows and unavailable Windows symlink/junction/hardlink fixture privileges; existing storage Unix permission skip is the fifth. Cross Linux compilation/static evidence is in implementer report; no native Linux execution for this patch yet.

Reviewed: exclusive protected-before-payload creation; existing directory/ACL refusal without repair; current UID/SID, regular type and single-link checks; opened-object identity and metadata checks; Read MaxInt64 overflow refusal and exact byte count; Windows protected inheritable parent and owner-only inherited SQLite sidecars; compatibility ProtectEmpty preserves already-open-reader refusal. Approved unsafe WAL/SHM backup regression refuses before SQLite open/destination creation and preserves bytes/identity/metadata. Reviewer focused outside patch on storage/backup.go precreation0600, resolving destination inheritance under protected Windows parent.

Limitations: same-user malicious process and ordinary-check revocation of existing handles are outside documented boundary. Future strict configuration/Core/Web and final audit remain separate. No actual private credential/database access, network/live request or schema migration in this task. Actual schema2 and immutable live ACK probe remain separate. Hosted CI is not512MB/VPS evidence.

Complete product gosec47retained warnings/no high/zero type errors were individually compared and new paths reviewed; see gosec-disposition.md. Scanner exit1 retained honestly.

## Raw Windows alias follow-up

During strict config development, root independently reproduced raw trailing-dot normalization by Windows filepath.Abs: EnsureDir and Create accepted known synthetic aliases. Original Medium privatefs implementer wrote RED regressions across dot/space and both separators, then introduced shared lexical Resolve(raw)->absolute before normalization; existing boundary APIs/ProtectEmpty reuse it and Read final checks use canonical f.Name. Exact dot operators remain supported. Fresh same independent Medium reviewer approved spec/quality0findings on frozen .tools/reviews/privatefs-alias.diff; root read production correction and independently reran15pass/0fail/4unchangedskips plus privatefs vet0. Bytes/ACL/identity/no-new-file regressions are in task report. Resolve makes no existence/ancestor/privacy claim; config must retain those checks. New correction/config integration nativeCI remains pending; original85d60c8 CI success predates correction. No actual payload/permissions touched.
