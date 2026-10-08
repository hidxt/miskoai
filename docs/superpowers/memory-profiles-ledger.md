# SDD ledger — plan: docs/superpowers/plans/2026-10-09-memory-profiles.md

Spec: docs/superpowers/specs/2026-10-09-memory-profiles-design.md. Synthetic-only local implementation; real WeChat deferred; no actual private database migration. Existing codex/foundation isolates main. Windows-compatible .tools/reviews packages supplement this durable ledger; no Bash helper available.

## Preflight

| Tasks/interfaces | Produces and consumes | Finding / ruling |
|---|---|---|
| Task1 itself | Schema3, snapshot helpers, bounded APIs and tests | Exact manifests1/2/3; effective WAL recognized versions2/3; existing1 snapshot migration preserved. Admission must precede materialization; no pruning. |
| Task2 itself | One fixed-scope worker and strict JSON | Input is captured snapshot, output is unconfirmed derived data. Only stop finish accepted; failure eligibility60s and no autonomous API loop. |
| Task3 itself | Agent context/commands and service observer | Quoted lower-role derived/profile data; fixed safety/current input preserved. Clear caller serialized/quiesced. Observer wake cannot spawn work. |
| Task1 → Task2 | DerivedHistory + SaveDerived CAS | Captured count/revision correspond to history; visible-user proof plus storage source proof required. Clear increments revision/watermark. |
| Task1 → Task3 | ChatContext snapshot + ClearMemory/candidate/profile APIs | Sole-connection transaction cannot call public Store methods; factor rowQuery/fact insert helpers. Clear erases context but retains every ID/state. |
| Task2 → Task3 | Wake after acknowledged chat; same model object | Model admission cap2 is shared; task3 does not call summary model. Runtime joins worker before Store close. |
| Runtime → Task3 | service.go/service_test.go reused after its gate | Serial ownership; only pre-Run observer installation and ack wake added. No concurrent edits. |
| Task1 → backup/restore | Existing CLI uses ValidateBackup then candidate Open | Schema1/2 fixtures must pin those exact manifests, not accidentally latest3; strict backup recognition includes3; old CLI replacement tests still apply. |

- Ruling: candidate IDs do not reuse deleted IDs and revisions refuse overflow — a stale user review must not confirm a newer generated guess, and clear CAS must not wrap — cost is AUTOINCREMENT metadata and explicit overflow refusal.
- Ruling: compatible schema3 extends existing SQLite; it does not change the approved database route — memory/profiles are requested features — cost is migration/admission tests and retained old manifests. No real private migration is performed during development.
- Ruling: retain claimed message IDs/state when clearing memory, while erasing scoped content/reply — privacy clearing must not enable duplicate remote effects — cost is bounded dedup metadata remains until explicit future reconciliation/retention policy.

Tasks1/2/3: not started. Runtime service gate is a prerequisite.
