# Image-advice JSON mode: local verification

Engineering progress: **98%**, historical49/50 phase gates. Final acceptance and the owner-deferred first public release remain incomplete.

## Scope and evidence

On2026-10-31 the owner approved a local correction to the existing single image-advice reply request: DeepSeek `response_format={"type":"json_object"}` plus a trusted JSON example. The completed diagnostic deployment bf521038 remains the running trial version. This authorization does not cover a new real model request, public push, CI-triggering push or test-server replacement.

The preceding fresh owner trial returned text only. Fixed authenticated status `plain_text` proved the captured-catalogue advice branch accepted legacy plaintext without an image ID. No model response or chat body was logged. This correction enforces syntax, not a requirement to attach an image to each message. Official sources: [JSON Output](https://api-docs.deepseek.com/guides/json_mode/) and [Chat Completions](https://api-docs.deepseek.com/api/create-chat-completion/).

## Implementation contract

- Explicit optional `ChatJSON` shares the existing DeepSeek request construction, transport/admission,60s deadline, response validation and explicit429/503-only retry ceiling. Ordinary Chat, Stream and vision retain default format; neither prompt text nor catalogue data can select request mode.
- Agent activates this capability only after its existing trusted image-advice/captured-catalogue admission. The Core counted wrapper forwards exactly once, with one logical attempt and successful usage accounting. A legacy delegate still receives one Chat call; JSON failures never trigger another plain request.
- The system policy contains a complete three-string JSON example with empty image_id. Its bytes/tokens are reserved inside the existing context limits. Strict parsing, same-scope immutable catalogue selection, cooldown, once-only preparation and independent text/image ACKs remain unchanged.

## Verification ledger

All work uses the ignored project-owned public-source snapshot `.tools/reviews/oct31-json-mode/src`, existing project Go1.27.3 and synthetic boundary transports. Network credentials are removed from the test environment; dependencies are offline. No new tool installation or external user-file access.

| Evidence | Actual result |
| --- | --- |
| RED, `go test -p=3 -json -count=1 -timeout=8m ./internal/provider ./internal/agent ./internal/core -run JSONMode` | exit1;10 namedPASS/22 namedFAIL/0SKIP. Expected missing JSON capability, wrapper forwarding and Agent selection failures; no compile failure. Parent/subtests are counted separately. |
| GREEN, same command after minimal implementation | exit0;32 namedPASS/0FAIL/0SKIP. Actual mocked HTTP body, normal/stream/vision default paths, blank/error/oversize/cancel and max3 explicit429/503 attempts; actual Agent/Core two-ACK borrow and duplicate path. |
| Full affected modules, `go test -p=3 -json -count=1 -timeout=8m ./internal/provider ./internal/agent ./internal/core` | exit0;652 namedPASS/0FAIL/0SKIP. |
| Full vet | exit0; no diagnostics. |
| Concentrated fresh independent review | GPT-6.1 Sol Medium, `gpt-6.1-sol`/`medium`;0 actionable C/I/M findings. Primary read the report and four production diffs; all7 frozen files integrated byte-identically after baseline guards. |
| Full Windows sandbox suite, `go test -p=3 -json -count=1 -timeout=8m ./...` | exit1;3169 namedPASS/14 namedFAIL/8SKIP; CLI/Core/Library package8m timeouts. Not a clean full run; timeouts leave later cases unexecuted in that run. |
| One concentrated serial follow-up, `-p=1 -timeout=12m` with actual failed/timeout roots | exit1;31 namedPASS/2 namedFAIL/0SKIP in57.66s. All new nonlink failure/timeout cases passed; the remaining child/parent failure is authentic hardlink fixture creation denied before the product assertion. Product/test-internal deadlines unchanged; load contention is a hypothesis, not a proven cause. |
| Three native platform builds | exit0 each; ELF64 amd64/arm64 and PE amd64 headers/hash checked. Build artifacts are cross-compilation evidence, not execution on Linux. |
| Gitleaks first public-tree snapshot | exit0;556 git-visible files/0 reported leaks. Final documentation snapshot is checked separately before sealing. No publication performed. |
| New exact-source native Linux CI/race | Not run; prior diagnostic CI is separate. |
| New real provider/WeChat natural-image trial | Not run; live image acceptance remains open. |

Known Windows sandbox hardlink/junction fixture limitations must be retained if reproduced. The earlier owner-approved native proof belongs to the diagnostic candidate and is not reusable authorization for a new unsandboxed run. Do not claim a clean whole-suite result by dropping tests or combining unrelated candidates.

### Full-run failures and follow-up disposition

Named counts include parents and subtests; they are not independent assertions. All14 first-run failure events are retained:

| Test | First-run result / disposition |
| --- | --- |
| `TestLibraryBundleCLISourceAdmissionBeforeCore/hardlink` and parent `TestLibraryBundleCLISourceAdmissionBeforeCore` | Existing sandbox hardlink creation denied. |
| `TestStageLocalHardlinkRefusal` | Existing sandbox hardlink creation denied. |
| `TestBundleInstallLinkedObjects/destination`, `/stage` and parent `TestBundleInstallLinkedObjects` | Existing sandbox hardlink fixture creation denied. |
| `TestContentPhysicalBounds/hardlink` and parent `TestContentPhysicalBounds` | Existing sandbox hardlink fixture creation denied. |
| `TestLibraryBundleCLIBackupFailuresAndCompletedRetention/short-write`, `/cancel` and parent `TestLibraryBundleCLIBackupFailuresAndCompletedRetention` | Lifecycle/cancellation error in broad run; all11 child cases and parent passed in serial follow-up. |
| `TestLibraryBundleCLIRestoreFailuresAndEffectRetention/source-close` | Error in broad run; all7 child cases and parent passed in serial follow-up. |
| `TestCoreImageReplyCancelAndCloseJoin/close` and parent `TestCoreImageReplyCancelAndCloseJoin` | Job-boundary timeout in broad run; both children and parent passed in serial follow-up. |

The3 package8m timeouts occurred while running `TestLibraryBundleCLIRestoreFailuresAndEffectRetention/replacement`, `TestCoreLibraryBundleLegacyEmptyImport`, and `TestRetainedRecoveryRefusals/corrupt`. All3 cases passed in serial follow-up. The complete retained-recovery root additionally exposed `TestRetainedRecoveryRefusals/hardlink` and its parent as2 failed events: `retained_test.go:436` fails at actual `os.Link` with Access denied before the recovery assertion. All other7 children of that root passed. No assertion was weakened, deleted or skipped to obtain this result; no product source changed after the frozen review.

The8 full-run skips are `TestConfigurationRejectsSubstitutionAndPreservesTarget`, `TestLockRejectsMovedDirectory`, `TestPrivateJunctionTraversalWindows`, `TestPrivateDirNeverRepairsExisting`, `TestPrivateSymlinkTraversal`, `TestPrivateHardlinkAndEmptyProtection`, `TestBundleStageLinkedOriginalPreserved`, and `TestExistingParentPermissionsArePreserved`. Existing platform/privilege/fixture limitations remain. These mixed runs do not establish a clean full Windows suite or new native Linux acceptance.

Primary verified266 source/module/embed/LICENSE inputs identical between tested snapshot and integrated root, and all7 reviewed candidate hashes remain exact. The affected652-pass run is a complete same-candidate provider/Agent/Core run, recorded independently from the broad run and serial supplement. It does not erase either set of failures. New exact-source native Linux CI must pass before any separately approved deployment; actual natural-image behavior still needs a new owner-sent live message after such an update.

Private command logs/hash receipts remain under the project-owned ignored review directory. No credentials, private paths outside the project or chat content are included in this public report.

## Candidate binary identities

| Target | Bytes | SHA256 |
| --- | ---: | --- |
| Linux amd64 | 13,897,888 | `869ce5c2714c52acfdaee6dacaa503d008759da534e1be24cab490b147d40488` |
| Linux arm64 | 13,107,360 | `3423ea51f696e5ee4fcfd494e0f2b6fa5fe68e79ff9f4cff510ac52ffe14b6eb` |
| Windows amd64 | 14,027,776 | `4b4e2b919faea264aa5d016990cdc4db17f245dce5cc26688392c8b11dbe7c7f` |

These binaries remain local. The running diagnostic trial binary `bbdb154e…` is unchanged.
