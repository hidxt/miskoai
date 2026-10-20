# Joined Store.Close: primary integration verification

2026-10-20. Engineering **82%** =41/50 remains unchanged. This accepts the described local correction and exact adoption; new native CI, final security/functional/resource/artifact and release gates remain separate. Base public source is `5a94038c9b2b1321b2834719cdb364530b2ed974`.

## Confirmed defect and bounded correction

A controlled test wrapping the pinned real modernc SQLite driver proves a database/sql lifetime gap: cancellation's asynchronous rollback marks a transaction done before driver rollback and connection release finish. A caller's Rollback can return ErrTxDone, and old Store.Close could return nil while that physical cleanup was still held. The retained original-source RED log has two named failures: early successful Close and the uncached/raw idle-driver Close error. The final four-test file was expanded after RED; no byte-identical RED-file claim is made.

Under the existing owner-quiesced single-connection contract, Close now reserves the connection with a30s drain context, returns it idle, then synchronously DB.Close. sync.Once caches success/refusal for repeated and concurrent callers. The real-driver test holds rollback behind a channel, observes database/sql's actual capacity waiter, and proves both Close callers wait for physical cleanup. Further tests cover observable idle-driver Close errors and already-canceled/expired drain refusal. No production connector, new SQL, schema/configuration change, retry, sleep, Raw close or detached cleanup goroutine was introduced.

Primary and a fresh permitted GPT-6.1 Sol Medium reviewer traced current owners: Core rejects leases and joins generation/service/summary/Manager/readers before Store.Close; restore/bundle own exclusive leases through replacement; stopped maintenance and CLI owners finish synchronous work first. No new call may enter the reservation-to-DB.Close interval. The30s limit bounds drain acquisition, not synchronous physical I/O. A failed drain stops admission and returns ErrStorage without claiming joined shutdown. Core's existing release of its maintenance lock after storage errors remains an explicit reconciliation limitation. Errors database/sql internally discards are not newly observable.

## Independent and primary evidence

Author final narrow storage/Core run:7 named PASS/0FAIL/0SKIP; vet0. One consolidated fresh review independently read all506 frozen source identities, all11 retained evidence identities, actual RED/GREEN events and owner lifetimes: SpecApproved/QualityApproved,0Critical/Important/Minor. Primary read the complete changed files, patch and report and independently verified those frozen identities.

Primary resumed a complete native Windows Go1.27.2 offline/CGO0 run after the earlier owner pause. The original interrupted primary test/runner was joined and its partial logs retained; it is neither a successful gate nor a product failure. New outputs use a distinct resume1 prefix. Actual completed commands:

```text
go test -p=1 -json -count=1 -timeout=15m ./...
go vet -p=1 ./...
```

Actual full test/vet/post-source-identity exits are0/0/0, with **3253 named PASS, 0 FAIL, 4 existing platform SKIP**, 20 package PASS and 2 no-test package SKIP. Named parent and subtest events are counted separately; distinct named passes=3253. Test stderr and vet diagnostics are empty. All506 source bytes remained frozen before and after execution. Local environments were cleared of application/provider credential variables by name; no live service, provider, CDN or WeChat request was made.

Primary then verified 252 other compiled Go/test/module/embed inputs equal the current main working source after canonical newlines and adopted exactly two files:

| File | SHA256 |
|---|---|
|internal/storage/store.go|`621594a640631731d4a3f130b48775bb69c2f56235938d4f237cc24ad1811a8a`|
|internal/storage/close_test.go|`357de5a14ab5158811d630911d5041eed11703a666c97d33e15f348d41d22539`|

## Static evidence and limits

Affected storage-only gosec ran separately for Windows and Linux amd64 against the frozen corrected source. Windows retains41 warnings and Linux41 warnings, actual exits1/1;0Go errors,0new/0removed normalized findings per target. Code/semantic fingerprints ignore source line shifts. Other production sources/dependencies are unchanged from the existing125-file full scan (Windows83/Linux86 retained warnings); its explicit dispositions remain, with no suppression or clean-scan claim.

Latest pre-correction native run39287643209 passed unit/integration and vet but failed TestCoreImageDatabaseReopenDoesNotReplay/ambiguous in race testing, without a DATA RACE warning; downstream gates were skipped. This correction fixes a separately reproduced real lifecycle gap. Its causal link to that immutable native failure remains unproven. A successful local reopen does not diagnose that run, and new exact-source native Linux/race evidence is still required. Actual arm64 runtime, full security/functional review, real reconnect/expiry and sustained/resource acceptance remain open.

Ignored development evidence retains the frozen manifests, semantic RED/GREEN results, consolidated review, original interrupted logs, resumed full JSONL/exits, adoption identities and both scoped static reports/comparison. Public records include no real credentials, chats, private media, server identity or database contents. No consumed live/server attempt was rerun.
