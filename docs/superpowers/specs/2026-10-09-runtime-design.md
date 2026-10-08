# Serial runtime design

## Intent and scope
Continue authorized local development of the single Go companion. Integrate the text-core agent and durable inbox into one serial poller and one serial worker; later CLI/Web owns process lifetime. Real WeChat validation remains deferred, and no actual service with owner credentials may be started by the development agent. No new live cloud batch, VPS connection, main merge or release.

## Receive boundary
Extend the pinned Weixin adapter with a bounded raw poll API. A successful HTTP response is returned as opaque bytes before typed message normalization; retain malformed JSON as private evidence. Transport/auth/status errors remain safe sentinels. Existing GetUpdates uses the shared transport/decode boundary without changing its public contract. Decoder limits messages to256 and items to256 per message before typed-array allocation, cursor/opaque fields to16KiB and text to16KiB; invalid authorized entries quarantine the whole frame without advancing cursor.

Runtime first resolves a pending frame from the database before any next remote poll. Quarantined evidence stops receiving for that account. Normalization accepts only direct user messages in NEW/FINISH states (reject GENERATING) from the fixed QR-authorized user, with provider ID and reply context (top-level ID first, then the first nonempty item ID, following the pinned inbound helper), and ignores groups/unknown senders/bot messages before model/tools. Phase2 is text-only; unsupported media remains explicitly unimplemented until the media plan. Cursor advances only with committed inbox entries. Capacity failure retains the frame, waits for the worker to drain, and retries resolving locally; it must not repoll or repeat remote requests for that frame.

One wake channel holds at most32 references; it is only a hint, never the source of work. Worker reads batches of up to32 from SQLite and handles them serially with the fixed-scope agent. Sent/duplicate/rejected/failed/ambiguous terminal results remove the inbox reference. Storage failures retain work and stop safely; no claimed message is automatically replayed after restart. Unknown possible sends retain their durable claim. Shutdown cancels poll/model activity and joins both workers before the caller closes SQLite.

## Errors and observations
Authorization expiry/401 cancels both account poller and worker, including current model/send context, until explicit owner reauthorization; management status remains available under the outer lifetime. Protocol quarantine stops receiving until owner inspection. Retry polling transport/rate/service failures with bounded exponential delays1–30s under lifetime cancellation; sending remains one attempt. A30s cap is a delay bound, not permission to increase per-request deadlines. Poll responses with no work wait at least1s so synthetic/abnormal fast responses cannot spin. An absent/empty next cursor retains the prior cursor, matching the pinned monitor's nonempty cursor update.

Status exposes safe counters/enums only: receive state, counts received/processed/sent/duplicate/failed/ambiguous, provider token totals, last safe error code and uptime. It never exposes auth metadata, input/reply, raw frames, tokens, URLs or remote errors. Queue and goroutine counts stay fixed; no goroutine per message. Tests inject transport and agent interfaces with synthetic data and control cancellation/backoff without production network.

## Alternatives and decision
The selected serial worker keeps ordering, memory bounds and recovery understandable on512MB. Parallel per-message goroutines would complicate ordering and pressure the store; an in-memory-only queue would lose work on restart. Both are unnecessary for the first requested companion. Media/summary workers will have separate bounded admission and cannot alter text send policy.

## Acceptance
Synthetic tests must cover raw malformed response retention, valid frame/inbox/cursor restart, group/unknown sender rejection, capacity retaining the same frame, duplicate/ambiguous non-replay, auth expiry stopping polls and cancellation joining worker/poller. Native CI/race supplements Windows host tests; real WeChat and512MB resource measurements remain distinct gates.

Pinned evidence: [protocol enums/types](https://github.com/Tencent/openclaw-weixin/blob/24de5c9eb0dd5e595d7e2d090ed8a3f82870d42c/src/api/types.ts) defines USER/NEW/GENERATING/FINISH; [inbound identity helper](https://github.com/Tencent/openclaw-weixin/blob/24de5c9eb0dd5e595d7e2d090ed8a3f82870d42c/src/messaging/inbound.ts) selects top-level then item ID and echoes per-message context. The conservative state admission is MiskoAI policy, not a claim about real backend delivery.
