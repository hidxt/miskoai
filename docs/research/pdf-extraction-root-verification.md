# PDF extraction primary verification in progress

2026-10-12. Engineering **70%** (35/50). Package38 is unaccepted. This record separates reviewed source/preparation, actual local tests, native CI, live/resource tests and final acceptance.

The initial Task2 frozen source has thirteen owned identities and32 evidence identities;327 additional entries identify actual historical RED copies, not public product files. Primary read the complete Task2 production, tests, report and changed document/core contracts. Original author scoped340 named passes, vet0 and18 fixed seed passes remain recorded; the fresh initial reviewer approved without findings, but the following primary Important finding supersedes that approval before acceptance.

## I1: character mappings reset their limit for each font

The approved shared ceiling is8192 cumulative CMap mappings during one extraction, with no reset across pages. `textFont.addMapping` checks only its individual map length and range expansion uses that same local remainder; `budget` has no cumulative mapping counter. A generated two-page PDF with separate fonts, each expanding4096 mappings, succeeds with exact8192 total. Increasing the second font to4097 expands8193 total and still returns text3bytes/pages2/errornil. This violates the existing limit even though other work/node/retained-byte limits remain finite.

Primary ran a separate immutable tiny fixture under verified native WindowsGo1.27.2, offline, `go test -json -count=1 -timeout=15s -run '^TestPDFRootCMapAggregate$' ./internal/pdfsafe`. Actual result1 named pass/2 expected named failures (overflow subtest and parent), exit1, package1.251s; no panic/watchdog. `pdf-cmap-root-proof-source.json` records the source identities; `pdf-cmap-root-proof-tests.jsonl`, stderr and exits retain actual observations. Original author freezes/logs were not changed. The original permitted Medium author resumes a narrow cumulative-budget correction with pre-expansion refusal and cached-font regression coverage. PDF acceptance, publication and new native evidence wait for corrected review/root gates.

## Independent application preparations

Processor Task1 in a separate accepted313-file copy: fresh SpecApproved/QualityApproved0Critical/Important/Minor; final document58 plus disjoint image34/18 equals110 named passes, actual scoped exits0/vet0. Its earlier whole-attachment15s watchdog attempt had78 passes before actual exit1 and is not a full-suite pass. Supported PDF and actual Agent integration are still deferred.

Agent Task2 in another isolated copy: fresh SpecApproved/QualityApproved0Critical/Important/Minor; twelve actual finite shards cover59 top-level tests once,119 distinct named passes/0fail/0skip, each exit0/vet0. Processor is mocked; exact historical RED source was not frozen. No adoption or completion credit follows preparation alone.

Primary independently verified602 SHA256/length checks with0 mismatches across PDF immutable owned copies, actual/frozen evidence, historical baseline inventories and both preparation actual/copied identities. `document-preparations-root-identity-summary.json` also recounts actual GoJSON events. Root full integration/scans and supported real-parser/processor/Agent tests remain pending. Main application files remain unchanged at this preparation boundary.

Prior private core3be86ef nativeCI38140923924/all12stepsSUCCESS and root1497/vet0 are separate accepted evidence. No new native PDF/application result, paid/CDN/WeChat call, private DB migration/startup, actual arm64 execution,512MB measurement or final acceptance is claimed. Real WeChat remains deferred; paid allowance is spent; actual private schema2 remains unchanged.


2026-10-12 local PDF gate: engineering **72%** (36/50), package38 accepted locally. Fresh permitted Medium FIX1 review I1ADDRESSED/SpecApproved/QualityApproved0newCritical/Important/Minor; primary complete source/diff/test/report review and460 source/evidence identity checks0mismatch. Complete frozen321-public-file normal-owner native WindowsGo1.27.2 full1596 named pass/0fail/4existing platformskip,17packagepass/2no-test, actual test/vet exits0/empty vet. Original failed/initial evidence retained.

Complete staticWindows86files18511lines74warnings(13HIGH11MED50LOW), Linuxamd64STATIC86files18329lines77warnings(15HIGH13MED49LOW),0Goerrors/0nosec/actualwarningexit1each. One new G115 CMap conversion and one removed octal conversion fingerprint per target; all other warning dispositions retained. New conversion is bounded by per-font insertion ceiling8192 before `uint64(8192-len(mappings))`, and shared remaining room is checked before range expansion. Original octal now deliberately masks low8bits; no warning suppression/clean-scan claim. Full frozen-public Gitleaks~3052765bytes0/exit0. Same complete1479-byte pinned BSD license/source headers remain in source and binary notices; both retained256-entry encoding tables and all8 supplied source inventory identities verified locally. Final license/artifact gate remains separate.

New PDF development publication/nativeCI and real processor-Agent/Core integration are next. Prior private core3be86ef/native38140923924/all12SUCCESS is separate. LiveWeChat deferred, paid allowance spent, privateDBschema2 unchanged; no actualarm64/512MB/livefile/final acceptance credit. See [PDF root verification](docs/research/pdf-extraction-root-verification.md).
