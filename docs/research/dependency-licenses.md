# Dependency license inventory

Verified 2026-10-09 from downloaded local module license files. The current go.mod declares 11 external modules (12 including MiskoAI itself). MiskoAI retains its existing Apache-2.0 LICENSE.

## Declared modules

Paths below are relative to the repository; SHA-256 hashes identify the actual license bytes read. Executable classification combines the documentation worker's Windows amd64 dependency listing with the primary agent's successful `GOOS=linux go list -deps ./cmd/miskoai` output. This is build dependency analysis on the Windows host, not native Linux execution or a binary symbol audit.

| Module/version | License | Purpose | Executable classification | Local license path | SHA-256 |
| --- | --- | --- | --- | --- | --- |
| modernc.org/sqlite v1.60.1 | BSD-3-Clause | SQLite database driver | linked | .tools/gomod/modernc.org/sqlite@v1.60.1/LICENSE | c6fe05491a60ae13bcd223088d2705e36dede24e5587226231d2459ada5c4822 |
| github.com/dustin/go-humanize v1.0.1 | MIT | Formatting helpers through libc | linked | .tools/gomod/github.com/dustin/go-humanize@v1.0.1/LICENSE | a973b4498c13eb74baa2a8e5c351426a6826f2fcdd909916dbe53ee2e755fd71 |
| github.com/google/uuid v1.6.0 | BSD-3-Clause | UUID helper through libc | linked on Linux (primary verified); absent from earlier Windows listing | .tools/gomod/github.com/google/uuid@v1.6.0/LICENSE | 0a8d61ed3cbfd5312326e8126c31ce9c627a283adc99131b56896d29ada04b2d |
| github.com/mattn/go-isatty v0.0.24 | MIT | Terminal detection through libc | linked | .tools/gomod/github.com/mattn/go-isatty@v0.0.24/LICENSE | 08eab1118c80885fa1fa6a6dd7303f65a379fcb3733e063d20d1bbc2c76e6fa1 |
| github.com/ncruces/go-strftime v1.0.0 | MIT | SQLite date formatting | linked | .tools/gomod/github.com/ncruces/go-strftime@v1.0.0/LICENSE | 38ae43959daf953a393a585b2988672cb65a5a541aca0d0be5e72595a0a16883 |
| github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec | BSD-3-Clause | Large integer arithmetic | linked | .tools/gomod/github.com/remyoudompheng/bigfft@v0.0.0-20230129092748-24d4a6f8daec/LICENSE | dd26a7abddd02e2d0aba97805b31f248ef7835d9e10da289b22e3b8ab78b324d |
| golang.org/x/sys v0.48.0 | BSD-3-Clause | Platform system calls, Linux Unix calls and Windows ACL support | linked: x/sys/unix on Linux; x/sys/windows for Windows ACL | .tools/gomod/golang.org/x/sys@v0.48.0/LICENSE | 911f8f5782931320f5b8d1160a76365b83aea6447ee6c04fa6d5591467db9dad |
| modernc.org/libc v1.77.1 | BSD-3-Clause | Translated C runtime support | linked | .tools/gomod/modernc.org/libc@v1.77.1/LICENSE | 95ff867eb55a56935fa7492406cfa953fb7c13ca73f4c0a86ae05756b4605600 |
| modernc.org/mathutil v1.7.1 | BSD-3-Clause | Arithmetic support | linked | .tools/gomod/modernc.org/mathutil@v1.7.1/LICENSE | bfa9bf72a72ca009fd62a8f84fca3dca67e51d93af96352723646599898b6cf5 |
| modernc.org/memory v1.12.1 | BSD-3-Clause | Memory allocation support | linked | .tools/gomod/modernc.org/memory@v1.12.1/LICENSE | 59895e669f48f168b6b858358f6005779cdf40a265f7828813061b56af67b496 |
| golang.org/x/term v0.46.0 | BSD-3-Clause | Hidden terminal verification-code input | direct runtime dependency; linked on Linux (primary verified) | .tools/gomod/golang.org/x/term@v0.46.0/LICENSE | 911f8f5782931320f5b8d1160a76365b83aea6447ee6c04fa6d5591467db9dad |

## Inherited material and distribution notices

The SQLite module also ships LICENSE-SQLITE (public-domain dedication), LICENSE-SQLITE_VEC (MIT) and LICENSE-3RD-PARTY.md. The latter inventories inherited Go, musl libc, go-netdb and NixOS notices, graph-only dependencies and test-only material. modernc.org/libc also ships LICENSE-3RD-PARTY.md. modernc.org/memory carries LICENSE-GO, LICENSE-MMAP-GO and LICENSE-LOGO; the logo attribution is repository material, not executable code. Preserve applicable complete notices in binary distribution packaging. This inventory is not a replacement for those full texts.

No sqlite/vec or sqlite/vfs import was observed in the executable dependency listing. Those optional packages carry separate inherited material. The earlier Windows listing omitted UUID; the primary's subsequent Linux dependency listing includes it. Its notice is required for Linux product distribution.

The adapted Tencent protocol source is pinned to commit 24de5c9eb0dd5e595d7e2d090ed8a3f82870d42c, package 2.4.9. Its MIT notice is retained in docs/research/tencent-LICENSE.txt. Source: https://github.com/Tencent/openclaw-weixin/blob/24de5c9eb0dd5e595d7e2d090ed8a3f82870d42c/LICENSE.

## Development tools

These separately installed tools are not product go.mod dependencies: gitleaks v8.30.1 (MIT); govulncheck from golang.org/x/vuln v1.8.0 (BSD-3-Clause); gosec v2.29.0 (Apache-2.0). Their local license files were read. Their numerous SDK dependencies in the shared module cache do not establish product linkage and must not be reported as app dependencies.

## Actual verification and limits

Read current go.mod; ran `go list -m -json all` and `go list -deps -f` for ./cmd/miskoai with local GOPATH=.tools/go, GOMODCACHE=.tools/gomod and GOCACHE=.tools/gocache. The full graph contains additional upstream development/test/transpiler modules beyond the declared modules. Read LICENSE files and inherited notice inventories; computed SHA-256 below. The primary subsequently verified Linux dependency output includes github.com/google/uuid, golang.org/x/term and golang.org/x/sys/unix with exit 0. Windows ACL implementation uses golang.org/x/sys/windows. No binary distribution license audit is claimed; source notice packaging is recorded below.

| Additional notice | SHA-256 |
| --- | --- |
| .tools/gomod/modernc.org/sqlite@v1.60.1/LICENSE-SQLITE | 8438c9c89b849131ead81d5435cb97fcf052df5b0b286dda8a2d4c29e6cb3fd0 |
| .tools/gomod/modernc.org/sqlite@v1.60.1/LICENSE-SQLITE_VEC | 6ce72bbe12d975bd5286e5ab0a064c069693300c47bccbc57bec18485f1621ea |
| .tools/gomod/modernc.org/sqlite@v1.60.1/LICENSE-3RD-PARTY.md | b4366b9c27a9364633e015011a231913ed19bf1228f335bf617e50a6f7b168a6 |
| .tools/gomod/modernc.org/libc@v1.77.1/LICENSE-3RD-PARTY.md | f597097efe3d97021f89170746bd3a0fb9a8b6fb26b82043ed68a4e0283bee6c |
| .tools/gomod/modernc.org/memory@v1.12.1/LICENSE-GO | 2d36597f7117c38b006835ae7f537487207d8ec407aa9d9980794b2030cbc067 |
| .tools/gomod/modernc.org/memory@v1.12.1/LICENSE-MMAP-GO | c2eba69f20d05414538c3a5df7694dde392e065ff70882e1625e90f5d6659fff |
| .tools/gomod/modernc.org/memory@v1.12.1/LICENSE-LOGO | 5ae5bee3072a841376451b48d8cfcec7188e10543926d5870828d36c8a750dc5 |
| docs/research/tencent-LICENSE.txt | 8d7ee65fb779f15d455ca76ec0ee24e4473a1c45d0ca8ea96e6a39a82872d774 |
| .tools/gomod/github.com/zricethezav/gitleaks/v8@v8.30.1/LICENSE | e3884b252b3bfc045e55be43a34d1e80da070bc6f804ac95bf4660e97d62ebc6 |
| .tools/gomod/golang.org/x/vuln@v1.8.0/LICENSE | 911f8f5782931320f5b8d1160a76365b83aea6447ee6c04fa6d5591467db9dad |
| .tools/gomod/github.com/securego/gosec/v2@v2.29.0/LICENSE.txt | 2be0a81f30d2b73cb3b5182b969175245042158735fcc8af133929212c772e78 |

## Notice packaging checkpoint (2026-10-09)

Created internal/notices/notices.txt with complete source license texts from all 11 declared external modules, MiskoAI Apache-2.0, pinned Tencent MIT, Go standard library BSD license, root-level inherited LICENSE files and the mathutil/mersenne license. Additional module-root NOTICE/COPYING files and recursive NOTICE files were sought; none were present. SQLite/libc flattened third-party inventories and their full inherited texts are included, covering graph-only material conservatively. Memory logo attribution is included conservatively.

The source bundle contains22 distinct files; after Git LF normalization and removal of an extra final blank line it has126940 UTF-8 bytes, SHA256 5e0fe6de01539a170e7367e0a2ff8757a02c726eaec63c02b771b03b26877b24. Full license text remains preserved. Primary verifies binary embedding byte-for-byte through scripts/verify_binaries.py; release archive packaging is still pending.

Update: golang.org/x/term v0.46.0 LICENSE was read in full and appended without truncation. Its module contains one LICENSE and no additional NOTICE. It is now a direct dependency used by CLI hidden terminal verification. All 11 external module paths/versions are covered; no development scanner SDK dependency was added to the product list. The primary verified Linux build dependency linkage as described above; native Linux execution remains separate acceptance evidence.
