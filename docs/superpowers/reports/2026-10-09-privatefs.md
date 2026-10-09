# Runtime configuration Task1: shared private filesystem boundary

Date: 2026-10-09. Implementer: `gpt-6.1-sol`, reasoning effort `medium`; no delegation. Base: committed `c17c6ea`. Authoritative scope: `.tools/reviews/privatefs-brief.md`. Root approved `internal/cli/cli_test.go` synthetic fixture ownership because the brief's `operations_test.go` does not exist, then explicitly approved the narrow unsafe-sidecar backup-refusal regression and operations preflight.

## Project checkpoint

The reviewed Memory Task3/shared encoder checkpoint is the base and remains untouched. Task1 privatefs is local implementation pending fresh independent Medium review and root integrated verification. Runtime configuration, scoped administration, Core/Web, media, real WeChat follow-up, actual 512MB/VPS and final release acceptance remain separate later gates. Existing native Linux CI at c17c6ea excludes this working source. No actual database, credential, chat, live provider/channel request, network, staging or commit was used by this task.

## Implementation

`internal/privatefs` exports EnsureDir, CheckDir, CheckFile, Create, Read and ProtectEmpty with safe ErrUnsafe/ErrCreate/ErrRead sentinels. Existing dedicated directories are verified without chmod or ACL repair. Only a missing final directory is created; existing ancestors are checked for symlink/reparse traversal without requiring unrelated ancestors to be private. Root/current-directory paths are refused.

Unix uses owner identity, group/other permission absence, regular-file and single-link checks, 0700/0600 creation and O_NOFOLLOW opened handles. Windows uses protected owner-only inheritable directory DACLs and protected owner-only file DACLs supplied through security attributes at creation. Windows checks ownership, real object attributes and every allow ACE's principal on the actual opened handle. Owner-only inherited file ACEs under the verified protected parent are accepted, including SQLite-created sidecars. Windows Unix mode bits are never privacy evidence.

Read compares file identity, size, mode and modification metadata around the opened handle, bounds the read to maxBytes+1, refuses an unrepresentable read cap, requires the returned byte count to match the verified file size, and rechecks privacy before returning bytes. CheckFile accepts math.MaxInt64 for metadata-only validation. ProtectEmpty refuses nil/closed/nonregular/populated/multiple-link/link-or-reparse references and uses the reviewed Windows ReOpenFile algorithm to refuse an already-open reader before securing the empty object. CLI's compatibility wrapper delegates to this helper; no second ACL algorithm remains in CLI. Ordinary authorization/settings/lock/restore-candidate creation uses Create.

CLI privateDir/init/save/doctor/restore use the shared boundary. Doctor creates a dedicated private synthetic child. Restore retains its bounded streaming copy rather than materializing a possible 256MiB snapshot; shared CheckFile is followed by reopened-handle identity/size/modification checks before payload copy and source rechecks afterward. Candidate and lock privacy is established atomically before payload writes. Backup destination directories are verified before SQLite snapshot creation; existing database and any existing WAL/SHM privacy are checked before storage.Open or destination directory creation. These metadata-only checks use math.MaxInt64, preserving existing storage size admission rather than adding a new DB/journal cap. Restore keeps its existing 256MiB source cap and uses math.MaxInt64 for existing-target metadata privacy. Storage files/policy were not edited.

## Meaningful RED

With only unavailable API stubs and the new real-filesystem tests present, the following command exited 1:

```powershell
& .tools/toolchains/go1.27.2-verified/go/bin/go.exe test -count=1 -v ./internal/privatefs
```

TestPrivateFileRejectedBeforeRead, TestPrivateCreateExclusive, TestPrivateSymlinkTraversal and TestPrivateSafeErrorsAndDedicatedDirectory failed with `private filesystem boundary unavailable`; the Unix-only existing-mode test skipped on Windows. This was runtime RED from the missing boundary rather than a syntax/import failure. The subsequent Windows ACL test command also exited 1 with both creation-dependent tests failing against the unavailable boundary:

```powershell
& .tools/toolchains/go1.27.2-verified/go/bin/go.exe test -count=1 -run 'TestPrivateBroadACL|TestPrivateCreationACL' -v ./internal/privatefs
```

## Additional regression RED/GREEN

Root approved the narrow preflight needed to prevent SQLite opening an existing broad-reader sidecar. `go test -count=1 -run TestBackupRejectsUnsafeSidecarBeforeMutation -v ./internal/cli` exited 1 against the missing preflight: `unsafe sidecar accepted`. The implemented preflight now refuses both synthetic WAL and SHM variants; the tests assert unchanged main/WAL/SHM bytes, identity, mode, size and modification metadata, continued unsafe-sidecar refusal (no repair), and no destination directory creation. The Windows broad fixture is constructed in an inherited-read temporary directory and moved into the protected directory; its unsafe status is independently asserted before invoking backup. The approved correction does not alter SQLite admission/manifests/engine handling.

`go test -count=1 -run TestPrivateMetadataMaximumLimit -v ./internal/privatefs` exited 1 because the initial shared helper rejected a valid metadata-only maximum; metadata validation now accepts math.MaxInt64 without allocation. Read separately refuses a max+1 overflow. The explicitly named `go test -count=1 -run TestPrivateReadLimitOverflow -v ./internal/privatefs` exited 1 against an unguarded read with `overflowing limit returned a successful or partial payload <nil>`, reproducing empty success for a nonempty private fixture. The temporary unguarded baseline initially failed compilation due to a leftover unused math import; that setup error was corrected and is not behavioral RED evidence. The arithmetic guard and exact expected-length validation then passed the final affected suite/vet.

## Verification environment and commands

All Go commands used the verified workspace Go 1.27.2 executable, `GOTOOLCHAIN=local`, `GOPROXY=off`, `GOSUMDB=off`; GOPATH/GOMODCACHE/GOCACHE were `.tools/go`, `.tools/gomod`, `.tools/gocache` resolved beneath the workspace. GOTMPDIR/TEMP/TMP were `.tools/tmp` resolved beneath the workspace. No downloads occurred. `go version` reported `go1.27.2 windows/amd64`.

Commands run after implementation:

```powershell
& .tools/toolchains/go1.27.2-verified/go/bin/go.exe fmt ./internal/privatefs ./internal/cli
& .tools/toolchains/go1.27.2-verified/go/bin/go.exe test -count=1 -v ./internal/privatefs ./internal/cli
& .tools/toolchains/go1.27.2-verified/go/bin/go.exe vet ./internal/privatefs ./internal/cli
git diff --check -- internal/cli internal/privatefs
```

Latest completed native Windows run: privatefs 7 passed / 0 failed / 4 skipped; CLI 16 top-level tests plus 2 sidecar subtests passed / 0 failed / 0 skipped (25 passing test entries total across both packages); scoped Windows vet exited 0 with no diagnostics; diff check exited 0. Runs were repeated only after new changes or failing fixtures warranted it. An earlier affected run failed TestBackupRestoreRoundTrip because its synthetic directory had been created by storage with inherited host ACLs; the fixture now establishes the dedicated directory with EnsureDir before storage creation, without altering expectations or weakening checks.

Actual executed Windows ACL evidence:

- Broad explicit Everyone read ACL on a synthetic file is rejected without returning payload, changing its DACL or changing marker bytes.
- Broad explicit directory ACL is rejected by EnsureDir without permission repair. Read of a separate protected private child is refused because the parent is broad.
- A newly-created private directory's read-back DACL is protected with one current-owner allow ACE carrying both object/container inheritance.
- A synthetic child made through ordinary os.WriteFile inherits the owner-only ACL and is accepted as a sidecar fixture.
- A Create-returned empty file's read-back DACL is protected before any payload byte is written.
- Existing TestAuthorizationFileProtectedOwnerOnlyWindows, TestAuthorizationFileProtectionFailsClosedWindows, TestAuthorizationProtectionRejectsPreexistingReaderWindows and TestAuthorizationProtectionRejectsExistingContentWindows all pass through the delegating wrapper.

Explicit Windows skips: TestPrivateDirNeverRepairsExisting is Unix mode evidence replaced by the executed Windows ACL test; TestPrivateSymlinkTraversal cannot create a symlink because the execution environment reports missing privilege; TestPrivateJunctionTraversalWindows and TestPrivateHardlinkAndEmptyProtection cannot construct their synthetic fixtures because the environment reports access denied. Initial setup-failure runs were not counted as passing tests. No reparse/hardlink runtime acceptance is claimed for this host.

With GOOS=linux, GOARCH=amd64 and CGO_ENABLED=0:

```powershell
& .tools/toolchains/go1.27.2-verified/go/bin/go.exe test -c -o .tools/tmp/privatefs-linux-amd64.test ./internal/privatefs
& .tools/toolchains/go1.27.2-verified/go/bin/go.exe vet ./internal/privatefs ./internal/cli
```

Both exited 0 with no diagnostics. The final resulting amd64 test binary exists (4,869,192 bytes). These are cross-platform compilation/static checks only, not native Linux execution. Native Unix ownership/mode/symlink/hardlink tests remain for root's native Linux gate. With GOOS=linux, GOARCH=arm64 and CGO_ENABLED=0, `go test -c -o .tools/tmp/privatefs-linux-arm64.test ./internal/privatefs` also exited 0 with no diagnostics (4,634,759-byte test binary). Final Linux amd64 scoped cross-vet was rerun after the sidecar test expansion and exited 0. Neither binary was executed on this Windows host.

## Owned changes and self-review

Created: internal/privatefs/privatefs.go, permissions_unix.go, permissions_windows.go, privatefs_test.go, permissions_windows_test.go. Modified: internal/cli/authorization_permissions_windows.go, authorization_permissions_other.go, operations.go, probes.go and approved cli_test.go synthetic fixtures and sidecar regression. Created this report. No other root/project documents, git state, storage/config/module/dependency or live Weixin behavior were changed.

Self-review checked safe errors, no overwrite, no existing-directory permission repair, exact max+1 refusal and target retention, payload access ordering, protected directory inheritance, opened-handle ACL verification and compatibility reader refusal. Same-user malicious processes are outside the boundary; ordinary checks do not revoke already-acquired handles. Windows administrators retain normal OS ownership privileges. Ancestor traversal is checked, but no immunity beyond the actual metadata/handle checks is claimed. Native Linux runtime, Windows race, live providers/WeChat, actual database migration and VPS/RSS were not tested.

Implementation and report are frozen for independent Medium review and root integrated verification; no staging/commit/push by child.

## Follow-up: raw Windows aliases before normalization

Checkpoint: follow-up after base `85d60c8`. Implementer remains `gpt-6.1-sol`, reasoning effort `medium`; no delegation. Follow-up base `85d60c8`; root supplied the independently reproduced alias issue and approved the narrow exported Resolve interface. This follow-up supersedes the earlier Task1 freeze only for four privatefs files and this appended report. Runtime-config implementation is separately frozen pending reuse of this interface; root owns full integration. Core/Web/media, live channel/provider acceptance and actual 512MB/VPS/release gates remain separate project work.

Ownership followed: modified only internal/privatefs/privatefs.go, permissions_windows.go, permissions_windows_test.go, privatefs_test.go and appended this report. No CLI/config/root-doc edits, actual private data, credentials, networking, staging or commits. Unix permissions implementation is unchanged.

Root cause: Windows filepath.Abs accepts/normalizes raw trailing-dot and trailing-space component aliases. Validation applied only after Abs cannot see the original spelling, so an alias can silently address the canonical object. Raw ProtectEmpty names also reached the ACL mutator without lexical refusal.

Meaningful RED used existing APIs before introducing Resolve:

```powershell
& .tools/toolchains/go1.27.2-verified/go/bin/go.exe test -count=1 -run 'TestPrivateRawAliasesRejectedWindows|TestPrivateOrdinaryRelativePathsWindows' -v ./internal/privatefs
```

Exit 1 on committed behavior. All four dot/space × backslash/forward-slash alias subtests failed: EnsureDir and CheckFile accepted aliases, Read returned the known synthetic marker, Create touched the canonical new target, and ProtectEmpty accepted the raw alias and changed the synthetic empty target's ACL. The assertions also snapshot known target identity, bytes and owner/DACL before invoking these APIs. Ordinary relative dot-operator paths already passed. This was runtime behavior evidence, not a missing-API compilation failure.

Correction: exported Resolve(path) returns a safe ErrUnsafe for an empty or lexically invalid raw spelling, validates platform spelling before filepath.Abs and again afterward, and returns an absolute lexical path. Its documentation states that it proves neither existence, privacy nor absence of symlink/reparse traversal. Windows lexical inspection normalizes both separators, checks components (including UNC volume components), preserves only exact . and .. dot operators, and refuses other trailing-dot/space components, ADS/colon spellings and reserved names before Abs can erase them. Dedicated-directory checking, checkedOpen and Create reuse Resolve. ProtectEmpty resolves and rejects its raw f.Name before any permission mutation. Read's final identity/parent rechecks use the canonical absolute f.Name already associated with the verified opened handle.

The post-fix Resolve tests were added only after the API existed. They show no directory creation for a valid nonexistent path, permit lexical resolution of the current directory without falsely claiming dedicated-directory privacy, reject raw aliases/reserved/ADS spellings even in components followed by .., and independently check exact ./synthetic-data, .\synthetic-data, ..\synthetic-data and ../synthetic-data results. Existing APIs also accept ordinary relative paths and return the complete marker through Read.

GREEN commands, using the same verified Go1.27.2/offline workspace-cache/temp environment recorded above:

```powershell
& .tools/toolchains/go1.27.2-verified/go/bin/go.exe fmt ./internal/privatefs
& .tools/toolchains/go1.27.2-verified/go/bin/go.exe test -count=1 -run 'TestPrivateRawAliasesRejectedWindows|TestPrivateOrdinaryRelativePathsWindows' -v ./internal/privatefs
& .tools/toolchains/go1.27.2-verified/go/bin/go.exe test -count=1 -v ./internal/privatefs
& .tools/toolchains/go1.27.2-verified/go/bin/go.exe vet ./internal/privatefs
git diff --check -- internal/privatefs/privatefs.go internal/privatefs/permissions_windows.go internal/privatefs/permissions_windows_test.go internal/privatefs/privatefs_test.go
```

All GREEN commands exited 0. Final focused native Windows package run: 11 top-level tests plus four alias subtests passed (15 passing entries), zero failures, four unchanged explicit skips. Skips remain the Unix-mode test, denied Windows junction/hardlink setup, and unavailable Windows symlink privilege. The raw alias regression and ordinary relative-path tests executed without skips; known target ACLs/identity/bytes remain unchanged after refusal. Scoped vet and diff check produced no diagnostics. This follow-up did not rerun CLI/config/full-project tests, live probes or cross-platform execution; root will perform integrated checks after independent review.

Self-review inspected the exact production diff for raw-before-Abs ordering, safe sentinels, exact dot operators, both separator styles, canonical handle-name rechecks and no expansion into permission repair or config policy. Follow-up source and report are frozen for the same independent Medium reviewer and root integration.
