# Serial service independent task review

2026-10-09. Fresh GPT-6.1 Medium reviewer: /root/serial_service_review. Base51ea0ed; frozen task .tools/reviews/service.diff and scoped fix1 .tools/reviews/service-fix1.diff. Root persists this synthetic review.

Spec compliant; quality Approved. No Critical or Important defect established. The service has one poller, one serial worker,32 wake hints and a durable scoped inbox. It saves raw bytes before decode, including an empty BLOB; retains the same frame and cursor under capacity pressure; quarantines malformed authorized data; cancels both account tasks on expiry while retaining management lifetime; and retains inbox work on storage/capacity/unknown handler outcomes (service.go:1-258).

Authorization selectors precede typed ID/text semantics, while original object/array shape, cumulative duplicate/Unicode counts, status and cursor limits remain checked (normalize.go:39-195). Missing/null state follows the accepted encoding/json state0 policy. The first account pause cause survives late workers; status codes are allowlisted and nonnegative counters/usage saturate (service.go:79-107; status.go:23-72). Actual-agent fake model/sender restart tests cover successful, ambiguous and every prior claimed state without repeated effects.

Focused unchanged-code checks evaluated four named risks: filtered decoding still uses bounded channel preflight (weixin/types.go:85-226); handler storage results and synchronous cancellation cleanup match the service contract (agent.go:81-188); capacity rolls back and inbox completion is scoped/context-bound (inbox.go:31-164/198-245); ordinary business errors are classified by RawUpdates before raw return and therefore use polling backoff (client.go:261-277).

Minor fix1: text was joined before checking the aggregate16KiB budget. The original implementer now subtracts each item and newline from a nonnegative budget before Join. Exact-boundary/separator-overflow tests and focused allocation RED1.0→GREEN0 coverage passed. The same reviewer approved the scoped fix with no remaining or new issues (normalize.go:228-262; service_test.go:718-758).

Root independently read the code and ran the full repository:258 passed,0 failed,1 Unix permission check skipped on Windows; full vet passed. Service7.885s, storage64.056s. These are synthetic Windows results, not RSS or live acceptance.

Cannot verify in this review: CLI/Core store ownership and shutdown, native Linux/race, real provider/WeChat, or2C/512MB. Reviewer ran no tests, Git commands, network requests, private-data reads, delegation or mutations. Initial truncated output was recovered by reading only missing diff segments.
