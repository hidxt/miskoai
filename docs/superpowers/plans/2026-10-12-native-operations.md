# Native operations implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement task-by-task. Owner authorized continuous routine execution; root owns acceptance and Git.

**Goal:** Deliver a usable native binary/systemd operator package with honest local and native evidence boundaries.

**Architecture:** Existing single Go executable, SQLite and user systemd manager. Manual private configuration and reviewed commands; no installation script or runtime changes.

**Tech Stack:** Go1.27.2, native Linux amd64/arm64, systemd.

**Spec:** docs/superpowers/specs/2026-10-12-native-operations-design.md

## Global constraints

- All children ONLYgpt-6.1-sol with medium effort; root reviews/integrates.
- No actual credentials/private DB/channel/server access, network probes, permanent runtime or new resource caps.
- Preserve current CLI/config/storage/maintenance behavior; settings precedence and schema rollback restrictions remain.
- Exactly owned artifacts below; no README/root ledger/Git/workflow edits.

## Review focus

1. Existing installations/symlinks: inspect and refuse unexpected targets; no clobber or recursive deletion.
2. Secrets: local private editor and owner-only paths, no shell history/arguments or raw environment diagnostics.
3. Login lifetime/systemd compatibility: distinguish enable from lingering and syntax from actual startup.
4. Upgrade schema changes: consistent backup and explicit restored-channel reconciliation; binary rollback is insufficient.
5. Shutdown timeout: joins versus forced termination are distinct; no false graceful/resource/live claim.

### Task1: Native operator package

**Files:** create `deploy/systemd/miskoai.service`, `docs/operations/native-linux.md`, `docs/superpowers/reports/2026-10-12-native-operations.md`. Own ignored `.tools/reviews/native-operations-*` evidence only.

**Interfaces:** consumes existing `miskoai init|config validate|version|licenses|doctor|serve|status|backup PATH|restore PATH`, env config and privatefs/maintenance contracts. Produces installable user-service template plus guide; no new runtime interface.

- [ ] Read spec and relevant actual source; check all guide commands and private path assumptions.
- [ ] Create exact service template and Chinese operator guide. Include architecture/checksum/license/setup/tunnel/lifecycle/upgrade/rollback/uninstall and all five review-focus cases.
- [ ] Build existing native Windows executable into ignored task path and run `version`, `licenses`, `config validate`, and synthetic `doctor` under cleared MISKOAI_* names and fixed synthetic private config. Record actual args/exits; never inspect original environment values or invoke serve/probes.
- [ ] Read template against upstream primary docs; record Linux/systemd syntax/runtime as unverified unless actually run natively. No container/WSL substitutes, no new Git workflow.
- [ ] Self-review complete template/guide, preserve full evidence, freeze three owned SHA256/byte identities and copied evidence; return actual results/quiescence/limitations. Root handles fresh review, integration, progress and publication.

Plan self-review: all spec clauses map to Task1 and its review-focus checks; no implementation-mirroring tests; no hidden installation effects. No unknown signatures/values or routine approval required under owner autonomy.
