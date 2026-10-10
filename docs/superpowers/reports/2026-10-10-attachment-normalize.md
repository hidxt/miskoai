# Pure authorized attachment normalization

Task2 author completion, 2026-10-11. Engineering progress remains **62%**;
package34 credit, independent review, root integration and native CI are owned by
the primary agent. Author model: **gpt-6.1-sol**, reasoning effort **medium**.

## Scope and provenance

Created only `internal/service/attachment_normalize.go`, its new test file and
this report. The private `normalizeAttachments(raw, scope, receipt)` is a pure
alternative; `normalize.go`, `service.go`, the operational receiver and all
storage/agent/media/Core/Web/config/CLI files were not modified. No attachment
is processed operationally at this gate. The primary published Task1 separately
while this task was running; no Git operation was performed by this author.

Read mandatory repository instructions/documents, the Task2 contract in
`.tools/reviews/attachment-normalize-brief.md`, the attachment design and pinned
Weixin evidence. Wire provenance remains Tencent/openclaw-weixin commit
`24de5c9eb0dd5e595d7e2d090ed8a3f82870d42c` and
`docs/research/weixin-media-design-evidence.md`: inbound image2/file4, preferred
image hex key, canonical media key encodings, optional file length/MD5 and full
URL metadata. No new online research or live protocol evidence was generated.
The frozen Task1 `storage.Attachment.Validate` contract is reused. D050/D051
storage rulings remain unchanged.

## Implemented behavior

- Original frame admission is 2MiB/valid UTF8. Existing raw traversal bounds
  original message/item arrays to256 before typed arrays are materialized,
  including arrays from ignored senders. Status expiry takes precedence over
  visited message/cursor shape errors and other service status codes.
- Sender, recipient, group, message type and state authorize the message before
  consumed media/key parsing. Unauthorized descriptors with invalid preferred
  keys are ignored with the original cursor preserved.
- Every consumed field in authorized attachment messages is checked against
  its original key spelling. Duplicate, EqualFold and escaped aliases refuse
  before typed decoding collapses them. Original item type traversal remembers
  an earlier media type even when a later duplicate type would hide it as text.
- Nested metadata objects, strings and inert unknown fields are bounded at the
  raw boundary before consuming typed metadata. Image/file/ref/voice/video
  opaque values are bounded; text permits bounded escaped representations while
  its decoded joined caption remains16384bytes. Image metadata consumes only
  original media/preferred hex; file metadata consumes media/name/MD5/length.
- Exactly one image/file plus genuine text produces the typed DTO. Empty genuine
  caption is accepted. Multiple attachments or unsupported nontext kinds
  produce only `{Kind:"unsupported"}` with original genuine caption. Their
  unused media keys are never decoded, including invalid image/file key values.
- `encrypt_type` may be absent or the exact JSON integer1. Null, decimals,
  strings, booleans and other values refuse. `media.ParseKey` supplies existing
  strict key compatibility; invalid preferred hex cannot fall back. A valid
  preferred image hex makes fallback media key inert and absent from the DTO,
  even when the bounded fallback has a nonstring shape.
- Both full URL and query are retained as bounded supplied metadata, with no URL
  rewrite or fetch authorization. A future CDN consumer must independently
  enforce its existing host/public-IP policy and URL precedence. Attachment
  validation and the whole canonical16KiB DTO bound are delegated to Task1
  `Attachment.Validate`; storage validation is not independently duplicated.
- Lossless uint64 top-level IDs, opaque item-ID fallback, context token,
  advisory timestamp/receipt fallback and checked newline joining follow the
  original decoder and normalizer contracts. Complete text-only frames delegate
  to unchanged `normalize`, preserving existing null/duplicate/escaped-field
  behavior and errors. The default normalizer still skips media.
- USER text contains genuine caption/text only: no generated placeholder,
  extracted content, media URL/query/key or filename. Returned errors are fixed
  protocol/auth/service sentinels, never raw metadata or remote error text.

## Runtime RED and GREEN evidence

All commands used verified native Windows Go1.27.2 offline. The initial
compiling baseline delegated the new private entry point to the existing
normalizer; failures were behavioral assertion failures, not missing-symbol or
compile failures.

| Actual command after environment setup | Exit | Named pass/fail/skip | Complete log under `.tools/reviews/` |
|---|---:|---|---|
| `go test -json ./internal/service -run 'TestAuthorizedAttachmentNormalization\|TestAttachmentConsumedAliasesRefuse\|TestAttachmentCaptionOnlyProvenance\|TestMultipleUnsupportedItemsAreExplicit\|TestAttachmentMetadataAndKeysBounded' -count=1` (RED baseline) |1|4/22/0|`attachment-normalize-red.jsonl`|
| Same focused command after implementation (GREEN1) |0|26/0/0|`attachment-normalize-green1.jsonl`|
| `go test -json ./internal/service -run 'TestAttachmentOriginalTypeCannotHideMedia' -count=1` (targeted RED) |1|0/1/0|`attachment-normalize-type-red.jsonl`|
| `go test -json ./internal/service -run 'Test.*Attachment' -count=1` (GREEN2) |0|44/0/0|`attachment-normalize-green2.jsonl`|
| `go test -json ./internal/service ./internal/channel/weixin ./internal/storage ./internal/media -count=1` |0|641/0/1|`attachment-normalize-scoped.jsonl`|
| `go test -json ./internal/service -count=1` (final service source/tests) |0|99/0/0|`attachment-normalize-service-final.jsonl`|
| `go vet ./internal/service ./internal/channel/weixin ./internal/storage ./internal/media` (final source/tests) |0|0 output bytes|`attachment-normalize-vet.txt`|

The targeted RED caught a real intermediate bug: typed item inspection collapsed
an original `type:2,type:1` before strict alias checks. Original raw type
traversal corrected that cause; the regression is included in GREEN2 and final
service verification. Final new tests comprise11 top-level tests/48 named passes.
GREEN2's regex excludes `TestMultipleUnsupportedItemsAreExplicit`; that test
passed GREEN1 and the complete scoped/final service runs.

Freeze accounting: production source was unchanged throughout the four-package
run and final service rerun (SHA256
`5bd7110a7961ad79226a845e176b2be5b395ebccd8b3985daa4b35d9b8c29432`).
The four-package run compiled the earlier service test version. During its
storage phase, final test-only additions strengthened invalid preferred versus
valid fallback and unsupported invalid-key coverage, and added canonical DTO
escaping overflow, ignored voice bounds and16384byte escaped caption coverage.
Those additions were compiled and executed by the separate final complete
service run, not retroactively attributed to the initial scoped run. No storage
repeat was needed: production and other packages remained unchanged. Scoped vet
ran against final files. Root will run the complete frozen repository suite.

The single scoped skip is existing
`storage.TestExistingParentPermissionsArePreserved`:
`Unix permission bits require native Unix execution`. All four scoped packages
passed. No failure or skip was omitted; the logs are complete JSON event streams.

Environment setup used the exact executable
`.tools/toolchains/go1.27.2-verified/go/bin/go.exe` and matching gofmt. PATH
prepended that verified toolchain; `GOTOOLCHAIN=local`,
`GOPATH=<root>/.tools/go`, `GOMODCACHE=<root>/.tools/gomod`,
`GOCACHE=<root>/.tools/gocache`, `GOTMPDIR=<root>/.tools/tmp`,
`TEMP=TMP=GOTMPDIR`, `GOPROXY=off`, `GOSUMDB=off`.
All `MISKOAI_*` variables were removed by name, including
`MISKOAI_CREDENTIALS_FILE`, without printing their values. Synthetic replacements
were `MISKOAI_DATA_DIR=<root>/.tools/tmp/attachment-normalize-synthetic`,
`MISKOAI_ADMIN_PASSWORD=synthetic-password-123`,
`DEEPSEEK_API_KEY=synthetic-key` and `OLLAMA_API_KEY=synthetic-key`.
New tests are pure in-memory JSON fixtures and create no worker, private spool,
database or network lifetime. Existing scoped tests manage their own synthetic
fixtures. No actual data directory or credential file was opened.

## Frozen handoff and limits

Owned-file copies and complete SHA256/byte manifest are retained under
`.tools/reviews/attachment-normalize-owned/` and
`.tools/reviews/attachment-normalize-owned-hashes.json`. Complete logs plus their
hashes are in `.tools/reviews/attachment-normalize-log-hashes.json`. The report
hash is in the manifest, avoiding a self-referential in-report hash.

Author implementation and local scoped verification are complete with no known
unresolved implementation issue. Fresh independent review, root source/diff
inspection, frozen full tests/scans and native CI are pending primary gates.
This task did not run full repository checks/scanners, Git, external network,
actual SQLite migration, actual service, provider/CDN calls or real WeChat.
No download/decrypt/format/parser/vision/send integration, arm64 execution,
512MB measurement, live attachment acceptance or final release acceptance is
claimed. Existing paid allowance and owner-approved scope remain unchanged.

## Reviewer I1 correction — FIX1, 2026-10-11

The initial completion and hashes above describe the initial frozen artifact.
Fresh independent review returned ChangesRequired with0Critical/1Important/
0Minor. I1 found that the authorized message traversal retained only the last
matching `item_list` before inspecting item types. An earlier original media
list followed by a later text list could therefore erase all media detection
and delegate to the original text normalizer before strict consumed-field
admission. Root independently confirmed the trace. Initial root full test/vet
passes do not establish corrected-artifact acceptance.

This author reproduced I1 with compiling semantic tests before fixing it.
Fixtures cover image/file/unsupported original lists followed by text/empty
lists through duplicate, case-folded and escaped `item_list` spellings. Nine
later-text cases plus their parent test failed; the nine later-empty cases
already returned a protocol error through the old empty-text path. Separate
controls prove genuine text-only duplicate-list behavior still equals unchanged
`normalize`, unauthorized sender/recipient/group descriptors never consume
invalid keys, and original lists share the cumulative256-item limit.

The narrow fix inspects item types after authorization across EVERY original
matching `item_list`. Nontext detection remains monotonic across fields, while
only last-list items are retained for later materialization. As soon as any
original authorized list contains nontext, strict original message-key checks
reject duplicate/case-folded/escaped consumed lists before nested media/key
decoding. A wholly genuine text-only envelope continues to delegate unchanged.
No wider refactor, shared production edit or operational activation occurred.

| Actual FIX1 command after the same explicit offline/synthetic environment setup | Exit | Named pass/fail/skip | Complete new log under `.tools/reviews/` |
|---|---:|---|---|
| `go test -json ./internal/service -run 'TestAttachmentOriginalItemLists' -count=1` before fix |1|10/10/0|`attachment-normalize-fix1-red.jsonl`|
| Same focused command after fix |0|20/0/0|`attachment-normalize-fix1-green.jsonl`|
| `go test -json ./internal/service -count=1` on corrected final source/tests |0|119/0/0|`attachment-normalize-fix1-service.jsonl`|
| `go vet ./internal/service ./internal/channel/weixin ./internal/storage ./internal/media` |0|0 output bytes|`attachment-normalize-fix1-vet.txt`|

Default-source SHA256 values were identical before and after FIX1:
`normalize.go` =
`0ad4e93bfbb32621ae39ff940b7c560fd6c18f6febedde491ca3dc32aa817333`;
`service.go` =
`3a541f3d85455132bbca86a28115386f25ffa164626b65f07419efe2a430fa85`.
Other production dependencies were unchanged. No storage/full-root suite,
scanner, Git, external network, private database or live action was performed
in this correction pass.

Corrected owned copies are under
`.tools/reviews/attachment-normalize-fix1-owned/`, with
`attachment-normalize-fix1-owned-hashes.json` and
`attachment-normalize-fix1-log-hashes.json`. Initial copies/manifests and ALL
initial RED/GREEN/scoped logs remain preserved without overwrite. I1 is fixed
at the author gate with semantic RED/GREEN and complete service verification;
the same independent reviewer must rereview the corrected immutable artifact.
Root integration/full tests/native CI and all live/resource/final gates remain
pending. Engineering progress stays **62%**, without package34 credit.
