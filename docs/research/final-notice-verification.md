# Source and candidate binary notice verification

2026-10-20. Package44 is accepted for the recorded source/candidate artifacts. Engineering progress becomes **80%** =40/50, with10 packages remaining. This accepts dependency/source/binary notices; native CI, live/resource tests, final artifacts and release remain separate gates.

Primary verified all11 declared pinned external modules and23 complete notice texts, including the project license, upstream protocol attribution, Go runtime, SQLite and its supplied third-party notices, transitive modules and the retained PDF source license. Each notice header's SHA256 matches the actual pinned source file, and its full text matches after newline and outer-blank normalization. No license text was truncated. The verified Go1.27.2 toolchain LICENSE independently matches the embedded runtime license source: SHA256 `911f8f5782931320f5b8d1160a76365b83aea6447ee6c04fa6d5591467db9dad`. LICENSE is unchanged.

Native verified Windows Go1.27.2 produced candidates with `CGO_ENABLED=0`, `-trimpath`, `-buildvcs=false`, offline pinned dependencies and no private application/provider environment. All257 canonical code/module/embed inputs match product commit `5a94038c9b2b1321b2834719cdb364530b2ed974`; the frozen-source manifest and build metadata are retained privately as development evidence. The Windows candidate's actual `licenses` output matches every embedded notice byte, and its actual `version` output is `MiskoAI 0.1.0-dev`. Both Linux ELF64 candidates contain the identical complete notice bytes; ELF machine62/183 and absence of dynamic-loader/dynamic segments were verified. This is not Linux or arm64 execution.

| Candidate | Bytes | SHA256 |
|---|---:|---|
| Windows amd64 |20562944|`76106a0a34aeb90af2b91faddf7c761fe45045ac1c40f02db83186e742ecc831`|
| Linux amd64 |20432108|`16a955e918a38cd3dfa5f7a99d598a782242689eae34c5e8749069f25e3ba0ec`|
| Linux arm64 |19270918|`e431861c7323e6c102ac37df90e3a61b01d4377b4784ca17faf882caa2b4bf25`|

Primary independently parsed each actual `go version -m` result: Windows links10 external modules and Linux targets9 each. Every linked module/version/h1 sum matches go.mod/go.sum and has its complete pinned license in the23-text inventory. The11-module source inventory intentionally also covers declared dependencies that are not linked into a given platform binary. There is no unknown linked dependency, new dependency or missing linked notice.

The first cold Linux build reached its300s local watchdog after the Windows build succeeded; that failed attempt is preserved. After the separate synthetic-tool build populated dependency caches, a bounded continuation reused the already successful Windows file, revalidated it and built only the missing Linux targets with900s limits. The continuation actually exited0 and produced the manifest above. No product test was repeated for this artifact check, and no forced GC, container, WSL, server upload or release occurred.

Evidence: ignored `source-notices-root-oct20-verification.json`, `candidate-artifacts-oct20-resume/manifest.json`, individual build logs/buildinfo and `final-notices-root-oct20-acceptance.json`. Primary's verifier exited0 after re-reading all artifact bytes, notice inventory, module sums and current toolchain license. If later work changes dependencies, embedded notices or final artifacts, repeat the affected comparisons before release; this record does not waive that gate.

The exact native run39287643209 on this product source passed unit/integration and vet but FAILED the previously observed Core image-reopen ambiguous case during race tests; later vulnerability/history/build steps were skipped and no DATA RACE warning was present. That failure remains open. This notice acceptance does not accept package42, final artifact46, resource47/48 or release50.
