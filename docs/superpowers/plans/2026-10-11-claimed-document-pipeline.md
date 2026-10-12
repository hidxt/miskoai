# Claimed document attachment pipeline implementation plan

> **For agentic workers:** Use superpowers:subagent-driven-development or superpowers:executing-plans for the serial tasks below. Steps are independently checked; primary owns integration/publication.

**Goal:** Route an authorized, durably claimed supported file through private download, complete bounded extraction, a visibly labeled context excerpt and the existing one-send text reply boundary.

**Architecture:** Extend the existing Controller-lifetime attachment processor with document processing under its existing capacity-one slot. Reuse one document.Parser. Agent quotes a bounded excerpt as external user-role data alongside genuine caption, preserves mandatory safety/profile/current input, and calls the configured text model synchronously before artifact cleanup.

**Tech stack:** Existing native Go1.27.2 modules/SQLite/stdlib/embedded UI, no new module or resident runtime.

**Spec:** docs/superpowers/specs/2026-10-09-attachments-design.md, Document contract/Provenance/Shared admission, plus REQUIREMENTS.md and SECURITY.md. This is the planned application integration after pure parser gates, not a provider/schema/channel/deployment/resource change.

## Global constraints

2026-10-12 primary execution refinement: Task1 lifetime and Task2 Agent groundwork may be prepared in separate isolated copies of the accepted313-file PDF-core checkpoint while PDF extraction is the sole writer in the main checkout. Each preparation owns only its explicit processor or Agent paths and uses the fixed existing document.Result/Parser and planned DocumentProcessor interface; neither can activate file chat, modify the main checkout or receive completion credit. Supported PDF fixture, real processor-Agent combination, root acceptance and final adoption wait for the guarded extraction and independent task gates. Core Task3 remains serial after those gates. This changes preparation scheduling only, preserving one product writer in the main checkout, all scopes/caps/providers and the final dependency requirements below.

- Children ONLY GPT-6.1 Sol Medium (`gpt-6.1-sol`, `reasoning_effort medium`, isolated fork); no children from workers. One product writer; primary frozen checks and readonly reviews may overlap it.
- Input4MiB, decoded8MiB, complete extraction128KiB UTF8, pages64, one media operation through actual download/parse/model/Close; document parser cooperative2s includes its own waiting/reading. Existing Agent90s total context and CDN30s remain. No detached worker, hard deadline or RSS guarantee.
- Preserve DeepSeek official deepseek-flash text/vision/shared transport2, Ollama hosted search, scope/claim-before-effects, quotas, one send and ambiguity/no replay. Real WeChat deferred, finite paid allowance spent, actual private schema2 unchanged.
- No source adoption before guarded PDF Task1/Task2 fresh reviews/root gates and BSD notice integration. TXT/Markdown and restricted DOCX are already accepted. This plan remains PREPARED until those dependencies pass.
- Filename is only an allowlisted format hint, never a path or model field. Descriptor/key/URL/name/digest stay outside model context/errors/events. Only genuine caption is stored as USER evidence; document content never creates facts/candidates.
- Existing unsupported/multiple descriptors produce one honest response without fetch/model. No ordinary service startup/private DB or new paid/CDN/WeChat request is authorized by synthetic implementation.

## Concrete preflight and alternatives

Root read accepted storage.Attachment fields/Validate, CDN.Download, ImageProcessor slot/lifetime, document.Parser.Extract, Agent Handle/answerImage/context and Core production construction. CDN returns completed bounded plaintext but does not compare optional file length/MD5; Task1 must do that before parsing. Current Agent treats files as unsupported, and current document parser supports TXT/MD/DOCX only. This plan cannot run before PDF acceptance.

Choose the existing outer processor slot, instead of an independent file semaphore (which would permit two simultaneous media operations), and synchronous Parser/model callbacks, instead of an abandoned timeout worker. A pure parser-only hook would leave the requested chat feature incomplete. Retain existing constructors and add explicit document dependencies for compatible offline injection.

## Review focus

1. File name/metadata tricks cannot become filesystem access or an extra download; recognized extension is a hint, actual parser validation determines support.
2. A small final decompressed result cannot hide intermediate decode growth; accepted parser budgets are not reset or weakened in application integration.
3. Quoted text with control characters, Chinese boundary bytes or hostile instructions fits the actual serialized text context and cannot displace safety/profile/current caption.
4. Partial context visibility is shown deterministically in the channel reply; model wording alone is not proof of whole-document visibility.
5. Cancellation, bad metadata, successful model followed by failed Close, and recovered duplicates preserve actual cleanup, usage and single-send semantics.

## Task1: shared private document processing lifetime

**Own:** new internal/attachment/document.go, document_test.go; narrow image.go fields/construction; report docs/superpowers/reports/2026-10-11-document-lifetime.md. No Agent/Core/provider/storage/media/parser edits.

**Interfaces:** existing Artifact and downloader remain. Add `NewProcessor(dataDir string, cdn *media.CDN, parser *document.Parser) (*ImageProcessor,error)`, with nonnil parser, using the existing image constructor/downloader and adding one borrowed parser field. Preserve NewImageProcessor and its image-only semantics. Add `(*ImageProcessor).WithDocument(ctx context.Context, descriptor storage.Attachment, consume func(document.Result) error) error`; it uses the SAME slot as WithImage and synchronous ownership rules. Injected parser/downloader seams are private test helpers, no raw file path export.

- [ ] Write compiling finite semantic RED for supported tiny TXT/MD/DOCX/PDF, unsupported filename/no fetch, optional length/MD5 mismatch/no parser or consumer, invalid/error-with-artifact cleanup, held model/Close preventing both next image and document operations, actual cancellation join and safe error canaries. Use only generated accepted parser fixtures.
- [ ] Validate descriptor and kind=file; select final extension case-insensitively from txt/md/docx/pdf, require a nonempty basename and refuse slash/backslash names. Missing/unknown extension is honest unsupported without fetch; never inspect a filename as a local path. Do not broaden URL policy or add retries.
- [ ] Own download artifact even on download error, validate actual Size1..4194304 and optional canonical Length against actual plaintext. If MD5 exists, compare it against a context-aware fixed4096-byte ReaderAt scan; tolerate legitimate final full read plus EOF, refuse dishonest/short/error reads safely. MD5 is a compatibility checksum, never authentication. Preserve errors without raw metadata; retained static MD5 warnings must receive explicit primary disposition, not suppression.
- [ ] Invoke the existing reused Parser.Extract synchronously with the selected format and artifact; unsupported, empty, limit and cancellation return zero successful result and never call consume. Check complete nonempty valid UTF8 result<=131072, supported format and page bounds before passing it to consume. Keep slot through actual callback return and artifact Close; preserve safe cancellation and classify cleanup failure.
- [ ] Run attachment/document scoped test-json count1 and vet with a bounded watchdog, retain actual exits and RED/GREEN evidence, freeze owned/report/evidence hashes and copies for fresh Medium review and primary integration. No package38/real file-chat/RSS completion credit from this groundwork.

## Task2: bounded visible excerpt and claimed document Agent

**Own:** new internal/agent/document.go, document_test.go; narrow agent.go, attachment.go, context.go only for dependency/dispatch/mandatory structured context; report docs/superpowers/reports/2026-10-11-agent-document.md. No Core/service/processor/parser/storage edits.

**Interfaces:** `DocumentProcessor` has `WithDocument(context.Context,storage.Attachment,func(document.Result)error)error`. Add `NewWithAttachments(s *storage.Store,m Model,q Searcher,send Sender,o Options,vision Model,images ImageProcessor,documents DocumentProcessor)(*Agent,error)`; require documents nonnil and vision/images both present or both absent. Preserve New and NewWithImages exactly. Add private answerDocument following answerImage's return `(string,provider.Usage,string)` and existing claimed-send state machine.

- [ ] RED authorization-before-keys, claim-before-fetch/parser/model, real empty USER/history exclusion, document commands/search/facts/candidates inert, unknown file no effects, acknowledged/ambiguous restart no repeated effect, hostile source canaries, mandatory custom profile and caption retention, UTF8/JSON escape boundary, exact visible range label and usage-on-cleanup failure. Register every held release/cancel/join cleanup before assertions.
- [ ] Handle file attachments after the existing durable claim and generation caption<=8192 check; bypass caption command/search/fact extraction just as image input. Missing document dependency retains honest unsupported behavior. Successful extraction invokes text model exactly once in the processor's synchronous callback; preserve actual usage before finish/output/cleanup/send checks.
- [ ] Build mandatory safety/custom profile/current genuine caption plus external document data. Select a prefix at a valid UTF8 boundary, max16384 source bytes, with deterministic start=0/end-exclusive/total-extracted-byte metadata and format/pages. JSON-quote the excerpt as user-role noninstruction data; no system/assistant data impersonation, name/key/URL or persisted generated USER helper. Use a model-only fixed description request if caption is empty.
- [ ] Fit the ACTUAL quoted serialized text using existing contextCost/withinContext and forty-message/64KiB bounds. At most a bounded binary search over prefix size; each probe remains<=16384 source bytes and valid UTF8. Preserve safety/profile/current caption/excerpt as mandatory, then prune optional whole history pairs/data. If no nonempty excerpt fits, return honest context limit without model. Do not assume128KiB extraction fits the model.
- [ ] Prefix every successful document reply with `文档资料：使用提取文本 UTF-8 字节 [0,E) / T。` using actual numeric E/T, even when all extracted text fits. Reserve label bytes when applying existing maxReply=16384 validation; never truncate the label or falsely report whole document/page-layout understanding. Plain extracted-total means the declared parser subset, not every visible story/OCR.
- [ ] Retain successful usage if cleanup/output/send later fails; one existing durable sending/ACK/CompleteMessage path only. Record exactly genuine in.Text in USER messages and the resulting bounded assistant response; no excerpt into facts/candidates/history USER. Scoped tests/vet, frozen identities, fresh Medium review/root gate before Task3.

## Task3: Controller lifetime and receiver integration

**Own:** narrow internal/core/core.go,lifetime.go; new document integration tests in internal/core/document_test.go and internal/service/document_test.go; report docs/superpowers/reports/2026-10-11-core-document.md. Service production delivery/storage/normalizer already carry file descriptors and need no protocol/schema change.

**Interfaces:** append optional borrowed Documents agent.DocumentProcessor dependency and private Controller.documents; preserve existing text-only injection and Vision/Images pairing before filesystem effects. Production constructs one document.Parser and one NewProcessor on its existing owned CDN, assigns that same outer processor to image/document interfaces, and reuses it across generations/restore. Generation construction chooses NewWithAttachments only when documents are configured; otherwise existing constructors.

- [ ] RED production identity/shared outer-slot and parser across generation/restore, paired injection compatibility/no unintended production fetch, empty-caption supported file through durable synthetic inbox/claim, unsupported file single honest reply, actual cancel/Close/replace/restore waiting for held document cleanup, DB usable until cleanup returns, borrowed resources untouched, scope/high-ID/provenance and acknowledged/ambiguous reopen no replay.
- [ ] Integrate accepted dependencies only; no per-message/per-generation parser/pool, changed metrics category, second reply, ordinary private receiver startup or actual migration. Document model attempts use existing ChatAttempts/shared text delegate; successful usage accumulates once. Close joins generation/leases before owned CDN/database/lock, preserving source-proven ordering.
- [ ] Primary reads full owned source/diff/contracts, verifies every source/evidence identity, builds an accepted complete frozen snapshot excluding unrelated WIP, runs full test/vet/static/public-secret/history/native CI gates and fresh task review. Root updates ledgers and public development branch via explicit accepted paths only. Actual media/file/live/512MB/final acceptance remains separately open.

## Root self-review and execution ruling

Coverage maps the attachment spec's claim/provenance/private admission/document bounds/excerpt/one-send requirements to Tasks1/2/3. Interfaces agree: one WithDocument callback/result, one persistent outer slot, one explicit document Agent constructor, one borrowed Core dependency. Owned overlap is serial: Task1 image.go remains frozen before Task3 construction; Task2 Agent freeze precedes Task3 calls. Every Review Focus has named RED assertions. No placeholder feature/UI or supported-format fabrication is introduced. This plan does not cover sticker/library/output or deployment; their existing requirements remain separate.

Ruling: prepare and execute this application-integration plan under the owner's master request and repeated routine-autonomy instructions, with permitted Medium implementation/review gates. It changes no provider/SQLite/channel/deployment/safety/512MB requirement and needs no routine confirmation. Dependency failure keeps this plan prepared, while independent authorized work continues; bypassing PDF guards or adding concurrency is forbidden. Current engineering70%; no completion credit for preparation.
