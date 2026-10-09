# Runtime Configuration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans. Execute each task with its own implementation, synthetic test and fresh review gate.

**Goal:** Establish a shared private filesystem boundary and load actual bounded non-secret runtime settings without exposing credentials.

**Architecture:** A small privatefs package owns file/directory permission verification and private file creation. Configuration owns complete strict settings and explicit credential-file loading; Core and CLI consume those same boundaries. This is the configuration portion of the Core/Web design; controller, authenticated HTTP and management UI have their own later plan.

**Tech Stack:** Go1.27.2, stdlib and existing golang.org/x/sys only; native Linux and development Windows.

**Spec:** docs/superpowers/specs/2026-10-09-core-web-design.md.

## Global Constraints

- One native Go process; SQLite; embedded HTML/CSS/vanilla JS; Linux amd64/arm64 and2C/512MB target. No new dependency or resident runtime.
- Children use only GPT-6.1 Medium/Low; root owns architecture and actual private-data operations.
- Credentials stay environment or explicitly configured verified owner-only files; no guessed file paths. No credentials in settings, status, errors, command arguments or browser responses.
- File/directory privacy must be established before payload read/write: Unix0600/0700 or Windows owner-only DACL, no symlink/reparse or unsafe inherited-reader access.
- Environment overrides stored values; explicitly present invalid/empty environment values remain errors, never silently bypassed.
- DeepSeek official provider remains fixed, configurable text/vision models default to deepseek-flash. No real API request in startup/config/doctor tests.

## Review Focus

- Existing broad-read Windows directory or credential ACL: reject before payload access; never infer privacy from Unix mode bits on Windows.
- Symlink/reparse file or dedicated-directory substitution: refuse and leave the target unchanged; no silent permission repair of an existing unrelated directory.
- Valid first JSON value followed by another value, duplicate known fields, wrong types or huge tiny-field inputs: reject completely within the input cap.
- Explicit empty environment override: report invalid configuration; do not silently use saved defaults or private-file secrets instead.
- Missing authorization: Core can run management-only later, but malformed existing authorization must fail safely rather than be treated as missing.

## Task1: Shared private filesystem boundary — Medium

**Files:** create internal/privatefs/{privatefs.go,permissions_unix.go,permissions_windows.go,privatefs_test.go,permissions_windows_test.go}; modify internal/cli/{authorization_permissions_windows.go,authorization_permissions_other.go,operations.go,probes.go} only to reuse this boundary; owned report docs/superpowers/reports/2026-10-09-privatefs.md.

**Interfaces:**

- `EnsureDir(path string) error`: dedicated directory, reject root/current directory, create missing final directory with private permissions before use; existing directory must already be private. Verify path components against symlink/reparse traversal, without requiring unrelated ancestors to have0700.
- `CheckDir(path string) error`: verify existing dedicated private directory, no creation/permission mutation.
- `CheckFile(path string,maxBytes int64) error`: verify regular private file and nonnegative bounded size before payload access. Validate dedicated parent, current owner, Unix group/other permission absence or Windows owner-only DACL; refuse symlinks/reparse. On Windows the dedicated parent must have a protected owner-only inheritable DACL; child files may inherit owner-only entries from it, but every effective allow entry must target only the current owner. Newly application-created files use a protected DACL. This permits SQLite-created sidecars while still rejecting any other reader before payload access.
- `Create(path string) (*os.File,error)`: exclusive new regular file with owner-only access established atomically before first payload byte. Parent already private; no overwrite. Return safe sentinel errors, never attacker-controlled path/OS error text.
- `Read(path string,maxBytes int64) ([]byte,error)`: use the verified opened handle, compare identity/metadata around open, cap read at maxBytes+1, refuse unsafe/raced file. Same-user malicious arbitrary process is outside this boundary; do not claim cross-process immunity beyond actual checks.
- `ProtectEmpty(file *os.File) error`: shared compatibility boundary for the existing empty authorization-file helper. Refuse nil/closed/nonregular/nonempty/multiple-link/reparse files; establish and verify owner-only protection on the opened empty object before payload writes. Windows must preserve the reviewed already-open-reader refusal. This helper never repairs an existing nonempty credential or database. Ordinary creation uses Create with protection supplied at creation; keep the old CLI protectAuthorizationFile wrapper/tests delegating to this shared helper rather than duplicate its ACL algorithm.

- [ ] Write `TestPrivateFileRejectedBeforeRead`, `TestPrivateDirNeverRepairsExisting`, `TestPrivateCreateExclusive` and OS-specific ACL/symlink tests using only private synthetic fixtures. Assert broad ACL/mode refusal, unchanged target bytes/permissions, max+1 refusal, no overwrite and safe errors.
- [ ] Run the focused new tests and record the missing boundary as RED.
- [ ] Implement narrow helpers using existing token protection as reference. Unix uses ordinary0700/0600 ownership checks; Windows creation supplies protected owner SID DACL at creation through existing x/sys Windows facilities. Existing CLI save/init/restore/doctor paths use helpers without broad operation redesign. Synthetic fixture parents remain0700.
- [ ] Run privatefs and affected CLI tests/vet once, report commands, platform skips and actual ACL evidence; freeze for fresh Medium review and root integrated checks. No actual private data, staging or commit by child.

## Task2: Strict persistent settings and explicit private credential input — Medium

**Files:** modify internal/config/{config.go,config_test.go}; add internal/config/{settings.go,credentials.go,settings_test.go,credentials_test.go}; adapt internal/cli/operations.go initialization to produce real non-secret settings and weixin_probe.go to reuse strict private authorization loading; modify cli_test.go/weixin_probe_test.go only for synthetic configuration/auth fixture coverage. Owned report docs/superpowers/reports/2026-10-09-runtime-config.md. Task1 reviewed first; no concurrent CLI ownership.

**Interfaces:**

- Preserve `Load() (Config,error)`. Add `Settings` with listen, deepseek_url, model, vision_model, weixin_url, max_output_tokens, context_bytes and context_tokens JSON fields. Legacy six non-secret template fields remain accepted; context defaults match agent24576bytes/32768estimated tokens, caps16384..49152bytes/4096..65536tokens, output1..4096. DataDir is selected from MISKOAI_DATA_DIR and is never a setting in the JSON body.
- `LoadSettings(dataDir string) (Settings,error)`: missing settings returns defaults; existing settings must pass privatefs and strict complete UTF8 JSON object validation within16KiB, known fields only, no duplicate/case-fold aliases, wrong/null types or trailing value. Recognized partial legacy templates merge defaults only for absent fields.
- `SaveSettings(dataDir string,settings Settings) error`: validate non-secret values, create private temporary file, sync/close and atomic replace, retaining old settings on failure. Serial controller ownership handles concurrent writes later. No credential fields can be marshaled.
- `InitSettings(dataDir string,settings Settings) error`: same validation/serialization but exclusive private creation of settings.json, never replacing an existing file. CLI init uses this API so the existing no-overwrite guarantee survives adding persisted settings; share serialization helpers instead of copying SaveSettings logic.
- Add effective context budgets to Config plus non-secret override-presence metadata. `DesiredSettings() Settings` and `EffectiveSettings() Settings` expose only values, never credential data. Environment remains authoritative and endpoint validators are reused without external calls.
- Explicit `MISKOAI_CREDENTIALS_FILE` may load only the existing helper's exact `{DEEPSEEK_API_KEY,OLLAMA_API_KEY}` format within8KiB through privatefs. No automatic secrets/api-keys.json discovery. Environment presence takes precedence including empty/error handling. Admin password remains MISKOAI_ADMIN_PASSWORD; serve later requires16..256bytes and validUTF8, ordinary CLI diagnostics may operate without it.
- Credential precedence refinement: absence of all key sources means unconfigured and permits diagnostics; an explicitly present empty/invalid key environment value is an error and cannot fall back to file. Each present key must be validUTF8, nonblank, at most1024bytes and contain no ASCII controls. Load an explicitly configured credential file only for keys absent from environment; if both environment keys are present/valid no file payload read is needed. An explicitly empty credential-file path remains an error. File fields use exact canonical uppercase names, mandatory string types and the same key validation. Missing admin password is allowed for diagnostics; present invalid/empty password is rejected, while serve's minimum16-byte readiness requirement remains separate.
- `LoadAuthorization(dataDir string) (Authorization,error)` uses fixed weixin-auth.json; `Authorization{Token,Account,AllowedUser,BaseURL}` is private-only and never included in Config JSON/status. Missing returns a safe ErrAuthorizationMissing; existing file complete strict JSON within64KiB, exact four existing field names, bounded token16KiB/scope256 and official Weixin origin validation. Do not contact server or silently replace malformed authorization.
- Exact authorization keys are bot_token/account/allowed_user/base_url, preserving the existing writer. CLI live probe delegates only loading/conversion to this API; its fixed-marker/one-send/state behavior is unchanged. Strict object keys are canonical, rejecting case/Unicode-fold aliases even when supplied alone. Add TestInitSettingsNeverOverwrites and TestProbeStrictPrivateAuthorization; no test reads the actual workspace data directory.

- [ ] Write `TestSettingsCompleteStrictObject`, `TestSettingsLegacyDefaultsAndExplicitOverrides`, `TestCredentialFileExplicitOnly`, `TestAuthorizationMissingVersusInvalid` and `TestPublicSettingsNeverContainSecrets`. Include duplicate escaped/Unicode aliases, trailing JSON, invalid UTF8, nulls, over-cap files, broad ACLs, empty explicit overrides and synthetic secret-canary absence in every public serialization/error.
- [ ] Run focused tests and record RED behavior against old environment-only loading.
- [ ] Implement strict bounded typed loading and validation, factor environment override logic without duplicating provider/endpoint policy. Initialize persists only Settings through InitSettings; it must not call providers or read unknown files.
- [ ] Run config/CLI/privatefs tests/vet, report and freeze. Root independent review, full integration, native race, secret/license checks and explicit development commit follow. No actual credential/database read by child.

## Root gate and self-review

- [ ] Verify CLI→config→privatefs imports introduce no cycles, no secret JSON fields leak, and startup/maintenance can reuse the exact APIs.
- [ ] Verify all five Review Focus conditions have named synthetic tests in their owning task. Check unchanged init/doctor/QR/backup/restore behavior with the new privacy boundary, including Windows fixtures needing private ACLs.
- [ ] Record environment/desired/effective distinctions and OS evidence honestly. Root-only bounded real WeChat probes remain separate; no automatic general service start.

Plan self-review: configuration/credentials sections and shared privacy requirement are covered by the two tasks; HTTP, controller lifetime, metrics, store leases, maintenance and UI are deliberately separate subsequent subsystem work. Interfaces above use existing CLI credential format and approved endpoints. User's standing autonomous execution instruction supplies the execution method; no routine plan-approval pause is added.
