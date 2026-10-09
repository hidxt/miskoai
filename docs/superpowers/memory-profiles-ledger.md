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

Runtime service local gate passed afterfreshMediumreview/root258pass/1skip/vet0. Service development publication/nativeCI pending; actualcore stillnotexposed.

Task1 started: freshGPT-6.1 Medium /root/memory_storage3; exactTask1storage-only ownership/report. Tasks2/3 notstarted. No actualprivateDBmigration bychild. Ownerresumed realwx tests separatelyhandledROOT; storagechildremains synthetic-only.

Task1 frozen: freshMedium review found schema2sequenceadmission I1; originalMedium fixround1 observedRED unsafeacceptance/source mutation, version-awareboundedmetadata guard and24legacy2 snapshot/WAL cases fixed it. SameMedium scopedre-review approved; verifiedGo1.27.2 final183passes/0fail/1Unixskip/vet0. Rootintegratedsuite underway; noactualprivateDBmigration. Task2/3 notstarted. RootboundedWeChatvalidation separatelyusesimmutable reviewedStorage2+ACKexport.

Root integrated gate completed on verifiedGo1.27.2:436 test/subtest passes, zero failures, one Unix permission skip; all-package vet exit0. Task2 freshMedium implementation subsequently froze30 scoped passing test entries/vet0 and received freshMedium approval with no findings; root task-specific verification pending. No actual model call or private migration.

Task3 preflight refinement: the original ChatContext captures only stored active selection, but Agent Options may force another custom profile. Resolving that custom profile in a second read would violate the one captured snapshot requirement. Task3 therefore owns a narrow ChatContextWithProfile wrapper/private shared implementation and scoped profile-by-ID helper in existing storage files, tested without schema/admission/quota change. Empty override preserves active selection; explicit override does not mutate it. Root ordinary internal-API decision, no product/security/provider route change. Task3 still gated on root Task2 verification and publication checkpoint.

Task2 complete: fresh Medium spec/quality review approved; root independent source inspection and30 summary test entries/0fail/0skip/scopedvet0 on verifiedGo1.27.2 passed. Task1 root436/0fail/1Unixskip/fullvet0 remains separate evidence. Task3 dispatch follows the reviewed publication checkpoint.

Native Linux checkpoint358fc7a: [run37898057339](https://github.com/hidxt/miskoai/actions/runs/37898057339) succeeded on unit/integration, vet, native Linux race, govulncheck, full-history policy scan and Linux amd64/arm64 builds. Includes reviewed Storage3/Summary/ACK and Go1.27.2; subsequent Memory Task3 working changes are excluded. Hosted runner/cross-build evidence does not establish actual WeChat/media/arm64 runtime or512MB/VPS acceptance.

Task3 frozen implementation received fresh Medium spec approval but quality I1: candidate/fact bounded-array encoders duplicate the same marshal/separator/limit block. Root inspected and accepted the finding; original Medium implementer fixround1 factors the boundary while preserving one-item allocation and8candidate cap. Root full pre-fix integration is running; current fix is not yet accepted. Storage3/summary cross-task validators and Core lifetime remain their prior/separate gates.

Task3 local accepted after fresh Medium I1 fix/re-review:0unresolved findings. Root pre-fix full suite484pass/0fail/1Unixskip/fullvet0; post-fix affected agent47pass/0fail/0skip/scopedvet0, verifiedGo1.27.2. Persistent/explicit scoped profiles and derived summary enter one captured lower-role context; deterministic candidate review/clear and sent-only observer integrated. These are separate verification runs. Updated source scan/development publication/nativeTask3CI pending; no actual DB3migration/general live service. Next privatefs/config serial plan, followed by scoped administration/Core/Web/media. Real strictACK awaits new synthetic marker;512MB/VPS/finalrelease remain open.

Native Linux c17c6ea gate: [run37918656096](https://github.com/hidxt/miskoai/actions/runs/37918656096) succeeded on exactc17c6eaea41e22668357d153a0d77a8672e021d5: full unit/integration, vet, native race, govulncheck, full-history policy and Linux amd64/arm64 builds. This includes Memory Task3/shared encoder correction. Current privatefs working source is excluded. Hosted runner is not real WeChat/media/arm64 runtime or512MB/VPS acceptance.
