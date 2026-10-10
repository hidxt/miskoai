# Embedded management view: root verification

2026-10-10 checkpoint. Engineering **56%** = 28 of 50 equal local/phase work packages. Package29 passes its local engineering gate; this is not final UI, live integration, resource or release acceptance. Mandatory constraints were reread after context compression. The implementation and review children used only GPT-6.1 Sol Medium (`gpt-6.1-sol`, reasoning effort `medium`).

## Implementation and review

The six-file view adds embedded HTML/CSS/vanilla JavaScript for Dashboard, AI/personality, scoped Memory/history and System. Root read source/diffs and accepted backend contracts. It requires no frontend server or additional product runtime. Provider, channel, storage, authentication and resource policy remain unchanged by the view.

Fresh reviewer `/root/web_view_review` initially reported I1: database restore retained stale editor IDs and cached lists. Original author fixed confirmed-restore invalidation before dispatch, retaining the chosen File locally. Successful restore reloads authoritative views; failure/abort never restores old edit targets; pre-confirmation cancellation preserves state. Root's browser then found I2: profile control named `length` collided with the native collection's numeric length, and I3: a legal long profile name overflowed mobile layout. The original author used `namedItem` for profile fields and wrapping/min-width corrections. The same reviewer approved the scoped fixes: I1/I2/I3 addressed, spec and quality approved, zero new unresolved findings. No backend change was needed.

Known minor retained for final regression: an active built-in profile can show a blank friendly name, e.g. `当前档案：（warm）`.

## Frozen source identity

| Path | SHA256 |
|---|---|
| internal/web/assets.go | 78e0e184cdd9b45f79bfdeb12da2305eb8c086fda8dc33d82a44a6dded2bde35 |
| internal/web/assets_test.go | e08d4e66f53c274228c86b2f44216074dbaf19cd5ba1ab4b7fe5b258102133d4 |
| internal/web/assets/index.html | 4dca50c31957b7ede57ac993006e13a5fd17eeb61052be5ebf557c67ae09750d |
| internal/web/assets/app.js | 36bd0f7066c80ffcb542405ad08bc15d6194135b3ee56337ec512fc1b42fdc87 |
| internal/web/assets/style.css | 72d6d929b6497ba46811fcd629e29b867145c7b0a796d066f7ffe534eec3ca80 |
| docs/superpowers/reports/2026-10-09-web-view.md | fb75cffe6f4b0631b12d96e85c931d108bac18c5f5d9cd41b743f8c84c996168 |

Root's complete immutable export `.tools/reviews/web-view-reviewed-src2` contains223 public files. All six owned hashes matched before/after corrected checks. The scoped app/CSS/report fix patch is40749 bytes, SHA256 `822a69dc582678d40aa8b4e1453b81f112cb3540282651b9365489f94ae3ba3d`. Original patch/evidence remains retained; separate runs are not added together.

Root adopted the meaningful author regression fixture as optional development script `scripts/test_web_view.js`, SHA256 `6da96fb496a25f07dff86b32b91d4fc5312f284ec98067508e28e35067e50f79`. It executes real app.js in Node VM with modeled DOM/fetch, including native collection length collision and namedItem, real event handlers and captured mutation bodies. It adds no Node product dependency. Current native Go CI does not run this optional script.

## Actual local commands and results

Verified native Windows Go1.27.2 with offline module/cache/toolchain and dedicated synthetic private fixtures; no actual database or credentials. Root ran against the immutable corrected export:

```text
go test -json ./... -count=1
go vet ./...
node --check internal/web/assets/app.js
node <corrected real-app runtime fixture>
node scripts/test_web_view.js
```

Complete Go run: exit0, **833 named test/subtest passes,0 failures,4 existing platform skips**;14 package passes and2 no-test package skips, counted separately. The four existing named skips concern moved-directory locking, existing private-directory repair refusal, symlink traversal and existing parent permissions. Full vet exit0/no diagnostics. JavaScript syntax exit0. Corrected real-app runtime and public script each exit0, **4 passes/0 failures**, no browser/network; not additive counts.

Evidence: `.tools/reviews/web-view-root2-tests.jsonl`, `web-view-root2-vet.txt`, `web-view-root2-runtime.jsonl`, `web-view-public-runtime.jsonl`. Initial runtime I1 RED1pass/3fail captured an actual stale PATCH for decimal string ID9007199254740993; accurate I2 collision fixture RED0pass/4fail, corrected GREEN4pass. Author report distinguishes setup errors from runtime RED and mock limitations.

Complete production gosec: Windows59files/11040lines/57warnings (9MED,48LOW); Linuxamd64 static59files/10858lines/60warnings (2HIGH,11MED,47LOW),0Go errors/0nosec. Both warning exits1. Root compared normalized full finding fingerprints with accepted API baseline:0new/0removed; no assets.go warning. Existing Linux HIGH UID warnings retain their prior64bitABI disposition. These are not clean scans, native execution or measured RSS.

## Actual synthetic browser evidence

Root controlled the real browser and newly created isolated synthetic fixtures. Passed: wrong-password refusal/input clearing; login; honest Go-heap/unavailable-RSS and logical-attempt labels; desired/effective/ENV settings; profile create/select/delete confirmations; corrected full eight-field edit/save/reopen; fact paging/search/exact decimal string ID above2^53/edit/delete; null expiry and invalid-expiry refusal; candidate quotation/confirmation/rejection; scoped history; inert hostile HTML-like text and absence of other-scope canary; local diagnostics; clear confirmation; cross-tab CSRF rotation refusal with no automatic retry/manual refresh recovery; logout private-data clearing. Resume confirmation was canceled, so no actual channel resume is claimed.

Original real browser reproduced the profile length error and mobile overflow. Corrected profile fields all populated; user changes saved/reopened. At390x844 viewport, effective client/document width375px, zero overflowing visible elements; long legal hostile-looking heading remained visible and wrapped. Final console error/warning list empty; viewport reset and tabs closed. Actual screenshots retained privately under `.tools/reviews/web-view-fixed-desktop.jpg` and `web-view-fixed-mobile.jpg`.

All four synthetic fixture server lifetimes actually ended with `run_ok=true close_ok=true external_delegate_calls=0`, exit0. Fixtures never called Core.Run, used only their own new schema/database and offline delegates, and never accessed actual authorization, paid providers, WeChat or CDN. Two fixtures expired their30minute contexts; two were explicitly stopped and joined.

**Native file-save and file-picker restore remain unverified.** Browser download-event tooling unexpectedly blocked approximately7232s and3882s. Direct backup click later produced the prepared-download message, which does not prove native file creation. Focused expected-filename inventory found no artifact. No cause or product failure is inferred from the tool anomaly. Actual file chooser restore was not attempted. Node VM restore outcomes and backend synthetic Go tests remain their respective evidence classes. These native browser paths stay in final UI regression package45; D043 records the local-panel gate boundary.

## Native, live and final boundaries

Prerequisite public commit `7a8ead407ad724bb3adcb2e5603e1d08c359ff31`, [native run38028793488](https://github.com/hidxt/miskoai/actions/runs/38028793488), job114145196338: root read every configured step, all success (unit/integration,vet,native Linux race,govulncheck,history policy,amd64/arm64 builds). It excludes this new view. Failed earlier005e34a/run38027908824 remains historical; later steps were skipped there. View development publication/exact native verification is pending at this checkpoint.

Authorized four finite cloud probes already passed once; allowance spent. Owner confirmed earlier controlled text delivery, retained ambiguous/no replay. Fresh strictACK marker remains unanswered; real media/reconnect/expiry unverified. Actual private DB remains schema2; no operational service or migration started. No512MB VPS supplied, no RSS/arm64 execution/final security-license-artifact-release acceptance inferred.

TASKS.md, MEMORY.md and DELIVERY.md were compressed into current constraints/evidence. Their complete prior byte content is preserved in `docs/history/2026-10-10-{tasks,memory,delivery}.md`; historical pending/model statements are superseded by current rules. No requirement, denominator or product scope was changed. Next serial implementation is CDN Task2 with only new owned transfer files and offline tests.

## Development publication follow-up

Normal development commit `dab6c53ecccbfaeddfc1d7c4fa0f2388dec24ae8` contains exactly18 inspected paths (view sixfiles, optional client regression script, root evidence/state/archives). CDN working files were excluded. Index export228publicfiles matched six frozen hashes. Staged policy18contents, working policy232contents and full-history policy723contents:0findings/exit0. Gitleaks index export~1846607bytes and history21commits/~2016692bytes:0findings/exit0. Fullhistory scans preceded push.

Initial automatic approval rejected the public push for insufficient recognized payload/destination authorization. Root performed read-only verification of the user's original master task's explicit Public-maintenance/Push instructions and existing origin `hidxt/miskoai`, then resubmitted the same normal push with this evidence; approval allowed it. No indirect execution or bypass. Normal push7a8ead4..dab6c53 succeeded. No main merge or release. Exact [native CI38042004429](https://github.com/hidxt/miskoai/actions/runs/38042004429) is in progress at this follow-up; no result is claimed yet. Current Go workflow does not execute the optional Node fixture.


Exact native follow-up: run38042004429/job114183942231 on dab6c53ecccbfaeddfc1d7c4fa0f2388dec24ae8 completedSUCCESS. Root read every configured unit/integration,vet,native Linux race,govulncheck,full-history policy and Linuxamd64/arm64build step and confirmed overallworkflow/SHA. This includes the view, excludes current CDN working files, and does not execute optional Node regressions or grant final UI/live/512MB acceptance. Engineering56%.
