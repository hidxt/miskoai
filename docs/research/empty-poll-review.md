# Empty poll storage independent review

2026-10-09. Reviewer GPT-6.1 Medium /root/empty_poll_review, frozen task patch .tools/reviews/empty-poll.diff based on ebcfa5c; focused fix1 .tools/reviews/empty-poll-fix1.diff. Root persists this synthetic-only review. No real data or requests.

Spec compliant; quality Approved. Production patch is exactly three root-selected lines: permit zero-length input, coalesce nil bindings to X'', remove only zero-length receive admission predicate. BLOB type,2MiB maximum, global4frames/8MiB, account unresolved block and prior cursor checks are retained (inbox.go:36/47/55/58; migrations.go:238).

Nil/empty lifecycle tests exercise actual SQLite BLOB/length, pending evidence, close/reopen, quarantine, account block, cursor retention and consistent snapshot validation/reopened admission (empty_poll_test.go:15/87/123). Four zero-byte frames still fill the global slot quota; oversized/non-BLOB remain rejected (:109/142). Initial reviewer found no Critical/Important issues.

Minor fix1: NULL assertion previously accepted any insertion failure (:149). Original Low implementer replaced it with exact pinned-driver NOT NULL poll_frames.body code1299 text (:151); successful NULL or unrelated errors now fail. Focused affected test passed6.143s; no product/import changes. Same Medium scoped re-review found original finding addressed and no new issue; exact driver text deliberately fails if a future pinned dependency changes its wording. Final quality Approved, no remaining findings in task scope.

Reviewer performed one unchanged fixture-helper check at store_test.go:14 for private child path; inherited Open permission enforcement lies outside this patch. Restored child explicitly0700/file0600. No reviewer tests/git/network/private-data reads/delegation or checkout mutation occurred.

Cannot verify here: runtime persists before decode, malformed zero-body quarantine/cursor invariants, CLI replacement/rollback, native Linux/race and512MB resource acceptance. Report distinguishes completed-snapshot copying/reopen from actual CLI restore. Root separately ran zero-body/storage and existing CLI backup/restore focused regressions: both passed(storage1.004s, CLI6.443s); existing privateDir creates0700 child on target Unix. Full root suite/native CI reserved for integration.
