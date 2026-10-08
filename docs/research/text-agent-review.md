# Task1 text agent independent review

2026-10-09. GPT-6.1 Medium task-scoped review. Base e988028; head frozen uncommitted Task1; reviewed `.tools/reviews/text-agent.diff` against `.tools/reviews/text-agent-brief.md`, the text-core design and implementer report.

## Spec compliance

**Verdict: Spec compliant for Task1.** The patch supplies the requested six agent files, injected interfaces, fixed scope, option defaults/caps, deterministic memory commands, explicit search routing, bounded context/output, safe result codes, cancelable serialization and durable send handling. Authorization and envelope validation precede claim and subsequent effects (`internal/agent/agent.go:81`, `internal/agent/agent.go:105`).

Duplicate suppression covers every existing claim state, with restart tests for ambiguous sends and acknowledgment-persistence failure (`internal/agent/agent_test.go:120`, `internal/agent/agent_test.go:311`, `internal/agent/agent_test.go:339`). Root refinements are present: honest AI identity across profiles, nil-context rejection, incremental per-fact JSON export, exact empty arrays and an honest over-cap notice without deletion advice (`internal/agent/profiles.go:5`, `internal/agent/context_test.go:64`, `internal/agent/agent_test.go:475`, `internal/agent/agent_test.go:517`, `internal/agent/commands.go:29`).

**Cannot verify from this task diff:** durable raw receive, inbox/cursor atomicity, schema admission/migration and global fact quotas belong to storage/runtime work. The primary retains their separate review and integrated evidence. Task1 does not implement those subsystems.

**Cannot verify from this task diff:** service-wide ordering/concurrency when agents are instantiated, native Linux execution of this new package, real provider/WeChat interoperability or 2C/512MB performance. The admission gate serializes one Agent instance; runtime construction and workload measurements remain separate acceptance gates (`internal/agent/agent.go:81`).

## Strengths

- The send path establishes durable sending before its single sender call. Send uncertainty and acknowledgment-persistence failure return ambiguous; completed replies are persisted only after successful acknowledgment. Restart tests exercise the storage boundary (`internal/agent/agent.go:129`, `internal/agent/agent.go:139`, `internal/agent/agent.go:148`, `internal/agent/agent_test.go:311`).
- Admission waits respect cancellation before claiming work. The waiting-deadline test confirms that an expired queued request leaves its ID claimable (`internal/agent/agent_test.go:402`).
- Context construction preserves current input and fixed policy, removes whole history pairs, places quoted retrieval in user-role data and enforces byte and conservative token budgets (`internal/agent/context.go:15`, `internal/agent/context.go:78`, `internal/agent/context_test.go:44`).
- Tool authorization remains deterministic. Only explicit user commands invoke search or memory mutations; model output and retrieved instructions do not enter a tool-dispatch loop. Search tests include malicious retrieval and unsafe source URLs (`internal/agent/commands.go:13`, `internal/agent/commands.go:56`, `internal/agent/agent_test.go:242`).
- Export encoding stops at the channel cap while retaining a complete JSON array or returning a limit notice. Escaping-allocation and exact-boundary tests address meaningful resource and correctness regressions (`internal/agent/commands.go:29`, `internal/agent/agent_test.go:475`, `internal/agent/agent_test.go:517`).
- Scope tests verify independent same-ID handling, distinct client IDs and preservation of another scope's memory during clear (`internal/agent/agent_test.go:366`).

## Findings

- Critical: none found in the frozen Task1 patch.
- Important: none found in the frozen Task1 patch.
- Minor: none identified that warrant a change before task integration.

## Quality assessment and focused checks

**Task quality: Approved.** The implementation matches task boundaries and supplies concrete recovery, authorization, context-budget and export tests. Its claims remain limited to a synthetic text-agent subsystem; runtime and deployment acceptance need their own evidence.

For the concrete risk of a storage-contract mismatch, inspected unchanged `internal/storage/messages.go:21`, `:62`, `:84` and `:97`: claims reserve 16KiB before effects, transitions match agent calls, completion persists reply/sent atomically and History(16) returns up to 32 chronological turns.

For the concrete risk of context limits disagreeing with the provider, inspected unchanged `internal/provider/deepseek.go:60` and its request-validation block: the agent's 40-message/64KiB limits and maximum output of 4096 match the provider contract.

Actual commands: Get-Content read the frozen diff, brief, report, design and required repository documents; focused rg/Get-Content checked the two named contract risks. One rg invocation included nonexistent internal/provider/types.go and exited 1; the subsequent direct deepseek.go read resolved the provider check. No test suite was rerun. No network requests, private data access, database mutation, product edits, index/HEAD mutation, commit or delegation occurred.

Reported implementer tests are evidence supplied by the implementer, not independently rerun reviewer evidence. The primary's concurrent integrated checks, subsequent scans, native CI and acceptance decisions belong to the primary and are not asserted by this review.

The review was initially read-only. A primary follow-up explicitly authorized persisting this completed verdict to this documentation file only. No other file was changed by the reviewer. Review verdict frozen after persistence.
