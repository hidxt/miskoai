# Shared bounded vision input Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans. Root dispatch follows codec/image-validation gates; steps use synthetic data only.

**Goal:** Select the configured vision model on the existing DeepSeek transport and construct one bounded inline image without another model pool or whole plaintext copy.

**Architecture:** A DeepSeek model clone shares the existing netx.Client pointer, private credential and two-slot HTTP admission. A pure ReaderAt-to-base64 helper consumes a completed, previously format-validated image with fixed scratch. The later claimed attachment agent owns the one outer media lifetime; these provider primitives do not activate attachment reception or make any request by construction.

**Tech Stack:** Go1.27.2 stdlib encoding/base64, io, strings and existing provider/netx; no new provider/dependency/runtime.

**Spec:** docs/superpowers/specs/2026-10-09-attachments-design.md. Root inspected current DeepSeek.request/Chat/Stream, netx.request admission-before-json.Marshal and agent.contextCost. Existing inline prefix6MiB/request8MiB/response2MiB/two-slot constraints remain. Provider research in docs/research/providers.md is historical source evidence; the four live cloud probes are spent and cannot be rerun.

## Global Constraints

- All children ONLY GPT-6.1 Sol Medium (gpt-6.1-sol, reasoning_effort medium), no delegation/private data/external requests/Git/global configuration.
- Official configured DeepSeek/deepseek-flash remains the default text/vision supplier/model; model clone does not discover or switch suppliers.
- Input1..4MiB immutable completed image, only png/jpeg/gif MIME from separately reviewed full validation. Binary/base64 bytes are not genuine USER text or model instructions.
- One admitted media pipeline holds admission across validation, base64 construction, request/retry, actual response and artifact close. Clone shares existing two-slot transport with text/summary. No detached timeout worker or measured512MB/RSS claim.
- Encode with <=32KiB scratch and one prebounded builder; no io.ReadAll of whole plaintext or a second standalone base64 buffer. Preserve cooperative context errors and safe fixed errors without raw reader/key/image data.

## Review Focus

- Cloning a model cannot accidentally construct a fresh two-slot transport or change the source client's model/key/endpoint.
- Size/MIME/context refusal occurs before large allocation, input read or request; dishonest/truncated ReaderAt cannot publish partial content.
- Two simultaneously held original/clone requests prevent a third request from reaching the transport until actual release; canceled wait does not create an extra request.
- Worst-case JSON escaped text plus maximum inline image stays below the existing8MiB request cap; base64/JSON copies and admission order are documented honestly.
- Cancellation/read failure never exposes partial inline data or leaks secret canaries in errors and does not claim forced interruption of arbitrary ReaderAt.

## Task1: Shared model clone and bounded inline construction — Medium

**Files:** modify internal/provider/deepseek.go narrowly for shared immutable model validation/clone; create internal/provider/{vision.go,vision_test.go}; report docs/superpowers/reports/2026-10-10-shared-vision-input.md. No netx/Core/agent/media/config/Web edits; no changes to existing retry, stream or credential policy. Narrow existing request-preflight amendment below enforces its already stated one-image-per-request contract globally before JSON allocation.

**Interfaces:**
- `(*DeepSeek).WithModel(model string)(*DeepSeek,error)` validates bounded nonempty model using the same constructor model contract and returns a new immutable model/key wrapper sharing the EXACT original transport pointer. Nil receiver/unconfigured transport refuses safely. No NewDeepSeek/netx.New/new semaphore, copying Client or mutable receiver change. Configure-before-use synthetic transport injection remains confined to current package tests.
- `InlineImage(ctx context.Context, input io.ReaderAt, size int64, format string)(ContentPart,error)` accepts nonnil ctx/ReaderAt, size1..4194304, format exact png/jpeg/gif only; this helper is encoding, not full image validation. Caller must have passed immutable completed Artifact through ValidateImage. Select fixed MIME prefix, check integer arithmetic and standardbase64.EncodedLen(size) before allocation. Builder.Grow exact prefix+encoded bound, write prefix then base64.NewEncoder over a sectionReader with <=32KiB scratch and context checks between actual reads. Check encoder.Close and exact byte count; truncated/read-failing input returns no partial ContentPart. Size max+1/MIME/context refusal reads zero bytes.
- Return one `ContentPart{Type:"image_url",ImageURL:&ImageURL{URL:...,Detail:"low"}}`, no prompt/model/request by construction. At4MiB encoded length is5592408, plus fixed prefix<32bytes, under existing6MiB inline guard. The original input is not copied wholesale; builder.String retains its allocated encoding bytes. JSON serialization inside already-admitted netx.request still has its own bounded allocation/copies; do not claim zero-copy total or exact peak RSS from this helper.
- Root concrete source finding before dispatch: existing DeepSeek.request counts images separately per user ContentPart array, so multiple image-bearing user messages could pass preflight and trigger a large json.Marshal before netx checks8MiB. Enforce ONE image across the whole request and <=6MiB total inline bytes before serialization, not one per message; bound text with subtraction before addition. Runtime RED TestOneImageAcrossMessagesRefusesBeforeMarshal must show multiple arrays rejected/no RoundTripper call; real agent/CLI input remains unchanged and there is no public arbitrary structured-message endpoint. This enforces the existing claimed limit, not an expanded supplier/concurrency/request cap.
- Existing text aggregate<=64KiB and messages<=40/output<=4096 remain. Validate a maximum image plus worst-case escaped allowed text produces JSON<=8MiB; do not expand request/response/context/concurrency caps to make a mock pass. The future attachment context builder must count text portions of ContentPart arrays in its ordinary text budget and bound binary separately, rather than applying existing string-only contextCost blindly.

- [x] Runtime RED TestVisionCloneSharesTransportAndModelIsolation, TestVisionSharedAdmissionIncludesOriginalAndClone, TestInlineImageMaximumAndRefusalBeforeRead, TestInlineImageCanonicalEncodingAndFailures, TestMaximumVisionJSONBound. Use an offline held RoundTripper with actual concurrent original/clone Chat requests, third canceled waiter and exact attempt counters; compare decoded synthetic base64 independently, exact max/max+1, zero input/unknown format, short/error ReaderAt, canceled context, safe canary errors, maximum JSON escaped text. No network/provider calls.
- [x] Implement minimal clone/encoding helper; retain existing provider assertions and constructors. Focused runtimeRED then GREEN; no schema/media/agent wiring or live probes.
- [x] Run offline verified `go test -json ./internal/provider ./internal/netx -count=1` and scoped vet, report actual command/exit/count/skips and allocation/copy assumptions. Freeze for fresh Medium transport/resource review and root integrated/native/scanner gates.

## Root downstream obligations

The completed attachment agent must authorize and persist claim before work, validate descriptor/key/actual digest/length and full image before InlineImage, supply current genuine caption plus lower-role bounded context, use the configured WithModel clone from the original Core DeepSeek object, track logical vision attempts/actual successful usage, and make at most one strict-ACK text reply. Documents use separately budgeted labeled UTF8 excerpts. Unsupported/multiple items must receive honest bounded handling. Until that gate, this helper is not a working image chat pipeline. Actual extra paid vision/WeChat media tests and512MB measurement need their own bounded authorization/evidence; none is inferred here.


2026-10-11 rootlocalgate accepted: full1145pass/vet0/freshFIX1Approved0findings/static0new/publicGitleaks0; exactnativepublicationpending, no35creditforprimitives. D055 canonicalfield/URLguard closes the independent Importantfinding; fullpipeline taskplan remains separate. Engineering **64%**.
