# Memory and Profiles Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans. Execute serial tasks with fresh implementation and independent review gates.

**Goal:** Add bounded derived summaries, explicitly reviewed statement candidates and persistent expression profiles to the existing scoped chat core.

**Architecture:** Compatible SQLite schema3, one coalescing summary worker and quoted lower-role agent integration. Service observer is a wake hint, never a chat dependency.

**Tech Stack:** Go1.27 stdlib, existing pure-Go SQLite/DeepSeek; no new dependency.

**Spec:** docs/superpowers/specs/2026-10-09-memory-profiles-design.md.

## Global Constraints
- Go modular monolith; SQLite; one native resident process; Linux amd64/arm64,2C/512MB target.
- Only GPT-6.1 Medium child configurations; no delegation by children.
- No real provider/WeChat calls/private-data service start while owner defers validation.
- Fixed account/user scope; generated summaries/candidates never automatically become confirmed facts; no private payload in status/logs.
- Facts retain existing10000global/8MiB and4MiB per-scope bounds; messages/dedup survive memory clearing.
- One summary worker/one coalesced wake, total model concurrency remains2 through shared provider instance.

## Review Focus
- Restored/effective-WAL schema3 rows must be type/byte/count/UTF-8 checked before payload loading; schemas1/2 remain recognizable without mutation on rejection.
- Quoted evidence must come from the cited scoped USER message; assistant/external/model guesses cannot be silently confirmed.
- Clearing memory during an in-flight summary must prevent old generated data from returning; dedup IDs must remain claimed.
- Hot profile/summary reads capture one consistent snapshot and cannot override fixed system policy/current input.
- Cancellation joins the actual summary worker; repeated wakes and failed attempts cannot cause unbounded jobs/API calls.

## Task1: Storage3 — Medium
**Files:** modify internal/storage/{migrations.go,validate.go,store.go,memory.go,messages.go}; add derived.go/profiles.go/derived_test.go/profiles_test.go and schema3_test.go. Own only internal/storage plus report docs/superpowers/reports/2026-10-09-storage3.md.

**Interfaces:**
- `Summary {Text string; Watermark,Revision int64; UpdatedAt time.Time}`.
- `Candidate {ID int64; Content,MessageID,Quote string; CreatedAt time.Time}`.
- `Profile {ID,Name,Description,Style,Address,Length,Sticker string; Humor int}`.
- `Derived(ctx context.Context,scope Scope)(Summary,error)`; absent returns zero summary,nil.
- `CompletedCount(ctx context.Context,scope Scope)(int64,error)` counts state sent records including erased-content dedup tombstones.
- `DerivedHistory(ctx context.Context,scope Scope,pairs int)(Summary,int64,[]Turn,error)` limit1..16; one read transaction captures summary/revision, completed count and chronological completed nonempty history.
- `ContextSnapshot {History []Turn; Facts []Fact; Summary Summary; Profile Profile}`; `ChatContext(ctx context.Context,scope Scope,query string,pairs,factLimit int)(ContextSnapshot,error)` pairs1..16/factLimit1..8, bounded query as SearchFacts; one read snapshot for selected profile/derived/history/facts. Factor scoped private query helpers accepting existing rowQuery rather than duplicating History/SearchFacts SQL or calling public Store methods while a transaction holds the sole connection.
- `SaveDerived(ctx context.Context,scope Scope,expectedRevision int64,summary Summary,candidates []Candidate)error`; at most8 new candidates per atomic write; compare revision, validate watermark≤current sent count and≥prior watermark, increment revision. Candidate quote exact substring of cited scoped state-sent message's content; IDs/quotes valid, nonempty. New candidates deduplicate scope/messageID/quote/content. Candidate IDs use AUTOINCREMENT and remain non-reused after deletion/clear; stale confirmation references must never target a newly generated candidate. Revision increments use checked arithmetic, refusing overflow rather than wrapping. Return safe `ErrStale` on revision mismatch; quotas rollback summary+candidates together.
- `Candidates(ctx context.Context,scope Scope,limit int)([]Candidate,error)` limit1..128.
- `ConfirmCandidate(ctx context.Context,scope Scope,id int64)(int64,error)` atomic explicit_user fact(confidence1/importance50)+scoped candidate delete; quota failure preserves candidate. Factor transactional fact-insert helper, never call Store.AddFact while holding its sole connection transaction.
- `DeleteCandidate(ctx context.Context,scope Scope,id int64)error`.
- `ClearDerived(ctx context.Context,scope Scope)error`: empty summary, revision increment, watermark current count, delete scoped candidates.
- `ClearMemory(ctx context.Context,scope Scope)error`: transaction clears scoped facts/candidates/derived summary and erases messages content/reply, retaining every scope/ID/state claim; advance watermark/current count and revision. Caller quiesces scoped handling before external administration; deterministic agent clear is already serialized. Do not delete inbox/cursor/raw quarantine/auth metadata. History excludes rows whose content or reply is empty after this privacy operation.
- `Profiles(ctx context.Context,scope Scope)([]Profile,error)` custom rows only,≤16scope/64global.
- `PutProfile(ctx context.Context,scope Scope,p Profile)error`; canonical strict JSON body≤8KiB, global bodies128KiB; field caps/enum/ASCII ID per spec, reserved builtin IDs refused.
- `DeleteProfile(ctx context.Context,scope Scope,id string)error`; scoped active deletion selects warm transactionally.
- `SelectProfile(ctx context.Context,scope Scope,id string)error`; builtin or existing scoped custom;≤16global selection scopes.
- `ActiveProfile(ctx context.Context,scope Scope)(Profile,error)`; absent selection returns Profile{ID:"warm"}; builtin returns ID only; custom returns validated fields.

- [ ] Write meaningful synthetic RED tests: exact manifests1/2/3 migration/restore/WAL/admission bounds, no mutation on refusal; scoped derived/candidates, false quote/assistant-source refusal, atomic quota rollback; confirmation/isolation; clear CAS rejects stale job and all message states remain duplicate; profile quotas/hot selection/delete fallback/foreign scope.0700 child fixtures, no actual data.
- [ ] Implement exact separate schema1/2/3 manifests and version migration, read-only effective-WAL admission for recognized versions≥2 before writable open; checkpoint upgraded manifest before exposing store. Global summary16scopes/128KiB (8KiB each), candidates256/256KiB content+quote,≤128scope, profiles64/128KiB JSON body and≤16scope; metadata bounds before scans. No automatic data pruning.
- [ ] Run storage tests/vet, update owned report with RED/GREEN/refinements/limitations, freeze. Root independent review and integrated regression before Task2.

## Task2: Coalesced summary worker — Medium
**Files:** create internal/summary/{worker.go,prompt.go,worker_test.go,prompt_test.go}; own report docs/superpowers/reports/2026-10-09-summary.md only.

**Interfaces:**
- `Model interface { Chat(context.Context,[]provider.Message,int)(provider.Reply,error) }`.
- `Options {Account,User string}`; `New(*storage.Store,Model,Options)(*Worker,error)`.
- `Wake()` nonblocking channel1, no payload; `Run(context.Context)error` single start and cancel/join; `Snapshot()Status` safe Completed/Failed counters, LastCode only.
- Store interfaces above; fixed scope only. Use exact DerivedHistory snapshot helper from Task1; do not modify storage in this task.

- [ ] Write RED synthetic tests for16-message watermark gating, coalescing, invalid structured replies/unknown fields/more than8candidates, exact visible-user quote requirement, assistant/external rejection, expected-revision clear race, provider failure not affecting chat,60s failed-attempt eligibility, cancellation/join and no leaked running job.
- [ ] Implement one worker using the SAME shared DeepSeek Model object as text. Last16 completed pairs, original scoped IDs and user/assistant roles; generated request≤24KiB/40messages, quoted user-role data, previous summary prefix≤2KiB and each visible turn prefix≤1KiB, remove oldest whole pairs to fit. Fixed instruction preserves honest provenance, outputs strict JSON summary/candidates without tools. Every accepted quote must appear in the model-visible USER snippet, then storage checks actual USER source. Commands are excluded from candidate source. Reject invalid/truncated finish (only stop accepted), total output≤16KiB, summary8KiB, candidates8 with1KiB fields; output budget1024tokens and total deadline60s. No partial malformed JSON or automatic facts. Save watermark captured before call and expected revision.
- [ ] Eligibility≥16 completed messages beyond watermark. Failure waits≥60s before another wake is eligible; no autonomous endless API retry. A wake may be coalesced while working. Inject private time/wait test seam for bounded deterministic tests. Cancellation holds admission until actual model returns, respecting its context; no abandoned worker.
- [ ] Run summary tests/vet and report actual evidence/limits; freeze for root independent review.

## Task3: Agent/service derived data integration — Medium
**Files:** modify internal/agent/{agent.go,context.go,commands.go,profiles.go,agent_test.go,context_test.go}; internal/service/{service.go,service_test.go} observer hook only; internal/storage/{derived.go,profiles.go,derived_test.go,profiles_test.go} narrow captured explicit-profile override only; owned report docs/superpowers/reports/2026-10-09-memory-integration.md.

**Interfaces:**
- Add `Observer interface {Wake()}` and `(*Service).SetObserver(Observer)error` accepted only before Run; worker wakes after successful handled sent results. Do not introduce per-message goroutines.
- Add `(*Store).ChatContextWithProfile(ctx context.Context,scope Scope,query string,pairs,factLimit int,profileID string)(ContextSnapshot,error)`. Empty profileID captures stored active selection; explicit builtin or existing scoped custom ID is resolved in the SAME read transaction as summary/history/facts. Existing ChatContext delegates to the same private implementation with empty override. Factor profile-by-ID helper for active and explicit selection; never mutate stored selection merely to honor an Agent option. No schema/admission/quota change. This root preflight refinement closes a cross-task gap: Task1's original snapshot only captured the active profile, so an explicit custom override otherwise required a second inconsistent read.
- Agent uses reviewed ChatContext to build one captured derived/profile/history/facts snapshot per ordinary answer; persistent active selection overrides default warm unless Options.Profile explicitly selects a builtin/custom ID. Default empty Options.Profile uses stored selection; existing explicit builtin values keep behavior. Custom IDs validate by storage/ASCII rules. Missing configured custom profile returns configuration/input error safely, never bypasses safety.
- Derived summary prefix≤2KiB and custom profile JSON are quoted lower-user-role data; core safety remains the same system instruction, with builtin expression instruction when relevant. Remove oldest history pairs then optional facts/sources/summary; custom expression is data and can be omitted if mandatory budget cannot accommodate it. Never displace current input/safety.
- `/memory clear` invokes ClearMemory and replies honestly that explicit memory, derived data and prior context are cleared while IDs remain for duplicate prevention. Existing commands/search/one-send behavior preserved. Add `/memory candidates` listing≤8bounded candidates and `/memory confirm ID`/`/memory reject ID` deterministic explicit review controls (no model/search). Confirmation is caller's explicit authorization; invalid ID scoped refusal.

- [ ] Write RED tests for persistent custom profile hot changes/quoted injection/cross-scope refusal, captured explicit custom override without changing active selection, missing/deleted explicit profile refusal before model work, summary budget and no privileged retrieved data, candidate listing/confirmation/rejection isolation, clear blocks stale summary and preserves duplicate suppression, observer wake after ack only and no blocking chat.
- [ ] Implement minimal integration against reviewed concrete storage snapshot/worker interfaces. No broad architecture change.
- [ ] Run agent/service/storage/summary tests and vet; report/freeze. Root fresh Medium review and integrated full suite/native race/scan/development commit gates. Authenticated Web/CLI/lifecycle follows separately.

## Root gate
- [ ] Resolve actual cross-task interfaces, scope/budget/snapshot claims and reviewer cannot-verify items; update ledger/docs/decisions.
- [ ] Preserve all live/deployment limitations. No actual private-data migration during development.
