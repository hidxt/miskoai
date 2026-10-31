# Model failure diagnostics: local verification

Engineering **98%**. Owner approved local diagnostic implementation and synthetic verification only. **Local implementation, concentrated review and integrated diagnostic verification are complete at the stated scope.** Full regression completed **3434PASS/10FAIL/8SKIP**, with no timeout/build failure; full vet and three platform builds pass. Root diagnostic recheck **75PASS/0FAIL/0SKIP**. No new source publication, native CI, agent-initiated live test or deployment has occurred. This does not identify or repair the historical live failure's underlying cause.

## Implementation and invariants

Baseline source eec21cd. Fifteen owned source/test/asset files add finite typed categories at exact netx/DeepSeek error origins, one Handle-local diagnostic forwarded through Agent/Service, normalized authenticated status/diagnostics output and one escaped dashboard row. Ordinary, JSON image-advice, vision and document model-call errors are covered. Success/duplicates/nonmodel results clear the latest category; nonmodel media/cleanup failures do not independently set it. No SQLite/schema, durable history, profile, image-selection/cooldown or send/ACK/dedup change.

Categories: request_invalid, authentication, balance, rate_limit, unavailable, timeout, canceled, transport, payload_limit, response_invalid, response_empty, response_finish, unknown. Empty means no model failure in the latest result; unknown/injected strings are normalized. This remains a latest process-local observation, not a durable per-message trace or guarantee that any network request reached the supplier.

Model remains deepseek-flash, including the previously approved JSON parameter. Endpoint, request construction, prompts, budgets, deadlines, admission, one logical call and explicit received429/503-only max3 attempts remain unchanged. Error() renders a fixed category; no raw URL/error/body/model text/credential/chat enters diagnostics. For transport/header/body errors, preserve only known cancellation/deadline sentinels, not arbitrary nested HTTP errors that could broaden retries. HTTP status classification comes from received responses. Other generic errors remain safely transport/unknown as appropriate.

## Actual test cycle and concentrated review

Native project Go1.27.3 executable SHA25683b7ba66215e696414fa49462d0db2da4f4cf15c7bebd42f0b189fa04866999d. Offline/local settings: GOTOOLCHAINlocal, GOPROXYoff, GOSUMDBoff, GOENVoff, GOWORKoff, GOTELEMETRYoff, CGO0, GOMAXPROCS2, project-only module/cache/temp paths; provider key variables removed. No downloads or external runtime invocation.

- Baseline `go test -p=1 -json -count=1 -timeout=12m` provider/agent/core `-run JSONMode`: **32PASS/0FAIL/0SKIP**, exit0,119.21s.
- Initial new diagnostic RED: exit1; missing classifier APIs cause netx/provider/agent build failures, plus service/web missing-field assertions fail. This is not a compiled full red suite.
- Initial targeted GREEN across netx/provider/agent/service/web: **66PASS/0FAIL/0SKIP**, exit0,46.37s.
- One concentrated GPT-6.1 Sol Medium review: **0Critical/1Important/0Minor**. I1 correctly identified response-body cancellation/deadline identity being lost by sanitization. Primary independently traced the same gap; retain the original review and candidate diff.
- Full1 was interrupted by the primary upon that confirmed defect; session exit1 and both output handles closed were observed. The interrupted run is not a passing or completed gate. No service/tunnel process was stopped.
- Correction RED: **66PASS/5FAIL/0SKIP**, exit1,19.23s. Four header/body wrapped-cancellation/deadline children and their parent fail for the expected lost identity.
- Correction GREEN: **71PASS/0FAIL/0SKIP**, exit0,40.32s.
- Final targeted diagnostics: **75PASS/0FAIL/0SKIP**, exit0,43.81s, including generic read failure during request cancellation and injected HTTP503 transport/body errors that must remain one attempt.
- Same reviewer performed a bounded I1 correction inspection: **I1 addressed;0 unresolved Critical/Important/Minor**. Primary read both reports and the production diff; reviewer did not run tests or assert the final-target outcome.

Ruling: adopt the reviewed candidate into the root while the immutable identical-source full regression continues. Before adoption all270 original inputs still matched their baseline. After copying only15 owned files, **all277 candidate inputs match root byte-for-byte**, including module/embed/LICENSE inputs. No public Git operation occurs, and adoption is not acceptance of the still-pending full run.

Private local receipts are retained under .tools/reviews/oct31-model-failure. Prior JSON-mode Windows mixed evidence, native CI and server observations remain separate historical gates. No clean full Windows suite or fresh native CI is implied by targeted checks.

## Final broad local verification

Final immutable-source `go test -p=1 -json -count=1 -timeout=12m ./...`: exit1, **3434PASS/10FAIL/8SKIP**,1203.88s. The timeout is the Go test per-package bound, not a changed product deadline. No package timeout or build-failure event occurred. All failures are6 authentic hardlink fixture creations denied before their intended product assertions, plus4 parent failures. This is not a clean Windows full-suite pass. No new unsandboxed/privileged fixture check was performed and no prior one-time permission was reused.

| Package | Failed test | Observed setup location/reason |
|---|---|---|
| cli | TestLibraryBundleCLISourceAdmissionBeforeCore/hardlink | library_bundle_test.go:221, native link Access is denied |
| cli | TestLibraryBundleCLISourceAdmissionBeforeCore | parent of the above failure |
| library | TestBundleInstallLinkedObjects/destination | bundle_install_test.go:796, native Link setup Access is denied |
| library | TestBundleInstallLinkedObjects/stage | bundle_install_test.go:804, native Link setup Access is denied |
| library | TestBundleInstallLinkedObjects | parent of the above failures |
| library | TestContentPhysicalBounds/hardlink | content_test.go:358, authentic hardlink setup Access is denied |
| library | TestContentPhysicalBounds | parent of the above failure |
| library | TestRetainedRecoveryRefusals/hardlink | retained_test.go:436, authentic hardlink Access is denied |
| library | TestRetainedRecoveryRefusals | parent of the above failure |
| media | TestStageLocalHardlinkRefusal | local_test.go:88, native link Access is denied |

| Package | Skipped test | Actual reason |
|---|---|---|
| config | TestConfigurationRejectsSubstitutionAndPreservesTarget | host denies synthetic junction |
| library | TestBundleStageLinkedOriginalPreserved | host denies synthetic hardlink |
| maintenance | TestLockRejectsMovedDirectory | Windows refuses moving directory containing live lock; Unix identity needs native CI |
| privatefs | TestPrivateJunctionTraversalWindows | host denies synthetic junction |
| privatefs | TestPrivateDirNeverRepairsExisting | explicit broad DACL is covered by the Windows-specific case |
| privatefs | TestPrivateSymlinkTraversal | host lacks symlink privilege |
| privatefs | TestPrivateHardlinkAndEmptyProtection | host denies synthetic hardlink |
| storage | TestExistingParentPermissionsArePreserved | Unix permission-bit case requires native Unix execution |

Full `go vet -p=1 ./...`: exit0,45.49s. Three `go build -p=1 -trimpath -buildvcs=false -ldflags='-s -w'` targets: all exit0,157.67s total; PE/ELF signatures,64-bit target architectures and hashes checked. Vet/build ran alongside the final remaining full-test packages; no causal claim about earlier timing failures is made.

| Target | Bytes | SHA256 |
|---|---:|---|
| Windows amd64 | 14,036,992 | cb9296db76c261fbed36da306b98cdd7d527b46a648d3e66d3aaa3d40a517d71 |
| Linux amd64 | 13,906,080 | 4980936a8c4a274fae3fb06947765b831b8a167b7cccdbfb50abfd2fea089fe6 |
| Linux arm64 | 13,107,360 | 3e43501630041884a41cee536f8b686cec25f9aea3615b6e8135234f7fa335ae |

These are local cross-compiled native binaries, not actual Linux/arm64 execution, new resource acceptance, CI artifact equality or approved deployment assets.

## Integrated gate, scanning and boundaries

Primary reran the diagnostic command from the integrated project root: **75PASS/0FAIL/0SKIP**,exit0,125.59s. Rechecked all277 root and isolated source inputs against the final frozen manifest after tests/builds:0 mismatches. All3 binary hashes and actual PE32+/amd64 or ELF64/amd64/arm64 headers match. No local reproducibility or CI-byte equality claim.

Existing project Gitleaks scanned the frozen public-source copy, approximately2,359,788bytes, exit0/no leaks; report is empty. Final project six-pattern/public-filename policy scanned570 public files,0 findings. Diff check0; private evidence/binaries/source-copy ignore coverage verified, staged files0. Final receipts are retained locally. No public Git action occurred. A future commit/push requires fresh approval and its own status/diff/history/secret gates.

No new browser rendering/Node check has been performed for the single added escaped diagnostic row. Authenticated DTO checks cover both status and diagnostics endpoints; UI uses existing pairs→node→textContent rendering. No native race/Unix runtime/new-candidate CI, actual Linux/arm64 execution, fresh resource or live diagnostic acceptance is claimed.

The currently deployed test service still runs eec21cd without these new diagnostics. Historical peer-confirmed successful natural image delivery and subsequent model-unavailable notice are both retained. Observing a new classified live failure requires a separately approved fixed update and follow-up scope; no replay/new paid probe is authorized by this local work. Final acceptance and public Release remain deferred.
