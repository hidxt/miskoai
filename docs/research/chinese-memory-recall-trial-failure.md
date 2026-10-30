# Chinese memory recall: owner trial failure — 2026-10-22

2026-10-30 resolution update: revised source a66af6c deployed after all12 native CI checks; owner confirms the normal WeChat reply now recalls the saved synthetic preference for the original natural-question regression. The earlier failure and investigation below remain historical evidence. This is bounded literal-fragment recall, not universal synonym/semantic search. [Update evidence](trial-revision-server-update-verification.md).

Engineering remains **98%=49/50** at recorded local/phase gates. This new real-trial finding blocks final functional acceptance/publication; existing source-bound local/native passes do not override it. No product code, SQLite policy, deployment binary or provider configuration was changed during this investigation.

## Actual browser and peer evidence

At the owner's explicit request, primary used direct visible browser controls on the deployed management panel, rather than management API calls. Created one clearly labelled synthetic preference fact in the new server test scope (category functional test, importance60, no expiry). The page visibly reported successful save. A short Chinese keyword search found that fact. Existing history showed the stored received messages and corresponding replies; a keyword search selected the relevant test exchanges.

Owner then asked the new bot a natural Chinese question whose answer was present only in the manually saved fact, without including the answer in the question. Owner reported that the reply did not recall the fact. The management history visibly retained two such questions/replies denying a record. No automatic new paid/model/message test or replay was issued by primary.

Using the same full question in the management fact search returned zero matching facts; restoring a short keyword returned the original fact. This isolates an actual full-sentence retrieval failure from save/persistence/history success. The fact remains saved, unedited, explicitly synthetic; no facts/history were deleted or cleared. Browser remains on the relevant test history for owner inspection. Screenshots remain in ignored private review artifacts, not this repository.

## Source trace

`internal/agent/agent.go` answerWithProfile passes factQuery(current) to ChatContextWithProfile. `internal/agent/context.go` factQuery splits only at characters that are neither Unicode letters nor numbers, so an unspaced Chinese question stays one whole term. `internal/storage/derived.go` retrieves Facts via searchFacts. `internal/storage/memory.go` factPredicate produces quoted whole-term FTS OR escaped literal substring matches; management FactsPage uses that same predicate. The question's whole Chinese phrase is not contained in the saved declarative fact, while the shorter Chinese keyword is. No payload logging or private-database opening was needed for this trace.

This establishes the deterministic retrieval gap for this tested pair. It does not claim every Chinese recall fails, reveal an actual provider payload, or prove all unrelated context/pruning/model behavior. Existing term32/query1024/context/resource/scope/provenance limits remain unchanged. Any correction needs bounded synthetic regression coverage, integration and a separately verified server candidate before a new real recall acceptance claim. Do not hide the failure by inserting the answer into the question or changing the fact to repeat that question.

## Trial disposition

- Confirmed-fact save and short-keyword search: passed through the UI.
- History persistence and keyword search: passed through the UI.
- Natural Chinese full-question fact retrieval and real conversational recall: failed.
- Correction/new binary/retest: pending owner trial feedback and the normal implementation/verification path.
- Public release: still deferred; no tag/Release/upload.
