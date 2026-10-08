# Runtime Task1 independent review

2026-10-09. Frozen task patch: `.tools/reviews/raw-poll.diff`, base `ebcfa5c`, uncommitted task changes. Reviewer: GPT-6.1 Medium. Read-only product review; this report was persisted under the root's explicit follow-up authorization.

## Spec compliance

**Verdict: Spec compliant.**

- Both requested interfaces are implemented. `GetUpdates` delegates to `RawUpdates` and `DecodeUpdates`; the shared bounded exchange performs one HTTP request. `internal/channel/weixin/client.go:134`, `:154`, `:252`, `:261`.
- Raw polling preserves malformed successful bodies for caller-owned quarantine; transport, HTTP, size and explicit business failures return safe sentinels. `internal/channel/weixin/client.go:261`; `internal/channel/weixin/raw_test.go:34`, `:182`.
- Typed admission checks the response cap, UTF-8, complete object JSON and status before message decoding. Message/item budgets precede typed-array allocation. Duplicate arrays share cumulative budgets; Unicode folding mirrors relevant encoding/json key matching. `internal/channel/weixin/types.go:85`, `:103`, `:132`, `:200`; `internal/channel/weixin/raw_test.go:16`, `:122`.
- Known cursor, identifiers, context, text and opaque item fields have 16 KiB bounds; unknown fields remain accepted within the bounded raw response. `internal/channel/weixin/types.go:132`; `internal/channel/weixin/raw_test.go:210`.

## Strengths

- The transport extraction retains poll/outbound slot separation, authenticated headers, cancellation, bounded response reading and send ambiguity mapping. Raw/typed serialization, send coexistence and one-request behavior have direct synthetic tests. `internal/channel/weixin/client.go:154`; `internal/channel/weixin/raw_test.go:48`, `:148`, `:182`.
- Expiry in either ret or errcode takes precedence over generic failure in the other field, with raw and typed regression cases. `internal/channel/weixin/types.go:103`; `internal/channel/weixin/raw_test.go:79`, `:182`.
- Allocation regression tests distinguish preflight admission from a count check after typed decoding, including overwritten duplicate arrays. `internal/channel/weixin/raw_test.go:122`.
- Errors expose sentinels rather than remote text. Raw evidence remains in the explicitly returned byte slice. `internal/channel/weixin/client.go:134`, `:261`; `internal/channel/weixin/types.go:103`.

## Issues

- Critical: none found.
- Important: none found.
- Minor: none identified that warrants a task change.

## Quality assessment

**Verdict: Approved.** The implementation meets the raw boundary and bounded decoding requirements, with targeted coverage for retention, allocation, duplicate/Unicode keys and status precedence. No blocking shared transport/send/login regression was found.

### Named focused checks outside the patch hunks

- Shared send/login contract risk: inspected unchanged SendText, StartQR and QRStatusAt because call now uses the extracted exchange. Send remains 15 seconds, missing send acknowledgement remains ambiguous, and QR validation remains in the callers. `internal/channel/weixin/client.go:282`, `:306`, `:326`.
- Lossless ID/preflight compatibility risk: inspected unchanged Message/Item declarations and item ID unmarshaler. Wire tags match preflight guards; integer/string item IDs retain existing lossless handling. `internal/channel/weixin/types.go:14`, `:27`, `:45`.

## Cannot verify from this task diff

- Durable raw persistence before decoding, quarantine/cursor transactions, fixed QR sender authorization before model/tools, account-wide cancellation on expiry and the bounded 32-reference wake channel belong to subsequent runtime integration and require controller verification there.
- New native Linux/race behavior, real WeChat/provider behavior and actual 2C/512MB memory/soak acceptance remain separate unverified gates. Synthetic allocation ceilings are not process RSS evidence.
- Reported Windows suite/vet results are implementer evidence and do not replace the root integration gate.

## Review actions

Read the frozen patch and implementation report, required repository documents, runtime design and reviewer template. No test rerun, network request, private data read, product mutation, Git change or delegation was performed. The sole subsequent write was this authorized review document.
