# Finite WeChat authorization-state observation

2026-10-20. Engineering **82% = 41/50** unchanged; package24 reconnect/authorization-expiry recovery remains open.

After explicit owner approval, the primary executed the reviewed finite current-state checker exactly once on the local machine. Its binary SHA256 was `3034fde888edb8dc1c194dfb5a40294d3f6a8db760c4d49db375e59b54d78cbc`. It reads existing private authorization, reserves a retained exclusive private attempt marker and performs at most one55s GetUpdates query without retries or replies. Default invocation refuses before I/O. No chat database, model, upload, server or QR flow exists in this tool.

Actual result: `authorization_accepted`, `poll_attempted=true`, process exit0 and empty stderr. This means the one bounded current GetUpdates call succeeded. Received ordinary payloads/cursor were discarded, not printed or retained. The private once marker is preserved and will not be reset/reused. No reply was sent. Current accepted authorization needs no replacement QR on this evidence.

Preparation retained an initial Windows latch-creation failure, corrected by letting privatefs create the missing protected directory. Corrected synthetic tests3 PASS, vet/build0, default refusal1 and independent FIX1 review0 new Critical/Important/Minor bind the executed binary. Credentials, remote IDs, account metadata, payloads and authorization links are absent from this report.

This observation does not prove disconnected HTTP recovery, full Core cursor/restart behavior, future authorization validity or actual token-expiry/QR restoration. A separately prepared finite reconnect checker requires its own concrete authorization before any new request. No previous text/image or server attempt is repeated.


## Controlled real disconnect and reconnect

After separate concrete owner approval, the reviewed finite checker (SHA256 `f41b1741cecc67845a44f30aa4b2de90bd502587147520a1d7d2bb329c820f19`) executed its three separate phases exactly once. Primary read complete source/tests, verified frozen506 product inputs and tool/binary hashes, and independently reran scoped22 named PASS/0 FAIL/test0/vet0. Fresh independent source review reported0 Critical/Important/Minor. The tool has retained exclusive private once files and no automatic mode sequence/retry path.

First phase: one real authenticated query was written, then actively canceled through its own HTTP trace/context. Actual result disconnect_observed, request_wrote=true, cancel_observed=true, send_attempted=false, cleanup=true, process0/empty stderr. This proves observed local HTTP cancellation; it does not certify remote receipt of that canceled request or a spontaneous network outage.

The owner then sent the agreed new synthetic marker. A fresh process/client made one new query and at most one fixed text send: actual reconnect_ack_persisted, send_attempted=true, acknowledged=true, persisted=true, cleanup=true, process0/empty stderr. The owner explicitly confirmed receiving the new reconnect reply. Ordinary messages/cursor were discarded rather than stored or replied to. Only fixed synthetic identities/content were persisted in a new private test database; actual IDs/context remained in memory.

Third phase reopened only that synthetic database without authorization/network/original-database access. Exact sent history and existing-ID refusal passed: local_history_dedup_verified, persisted=true, poll_attempted=false, send_attempted=false, cleanup=true, process0/empty stderr. Storage closed before synthetic ownership and original ownership; all finite processes exited. Original data-directory ownership was held only by the live phases, after strict restore-pause admission; the original chat database was never opened or migrated.

The bounded authorization permitted at most three transmitted HTTP operations across the two live phases, no retries/model/CDN/server activity. This is controlled adapter connection recovery plus synthetic durable-ledger reopen/dedup, not ordinary Core cursor-loop recovery. No old text/image marker was resent and all once latches remain retained.

Actual authorization was still valid. The owner subsequently requested real invalid-authorization recovery and accepted manually disconnecting through the official WeChat interface followed by server rejection of the old authorization and re-scan recovery. That separate workflow is being prepared, not executed or accepted here. It will be described as deliberately invalidated authorization, not natural token TTL expiry. Package24 remains open until its remaining accepted scope is verified; engineering82% is unchanged.
