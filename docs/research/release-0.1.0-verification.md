# MiskoAI 0.1.0 final delivery verification

2026-10-21. Engineering **98% =49/50**: delivery49 is accepted; only owner-approved public binary release50 remains. This record does not assert publication, deployment or unrestricted real-data acceptance.

## Source, patch and actual checks

Release candidate source: `dce3c1b480fbe8acb2223a498edbe9baeac7533b`. Native verified Go1.27.3 Windowsamd64 compiler; CGO0, trimpath, buildvcs=false, pinned offline modules. The sole compiled-input change from084c8bb is go.mod's1.27.2→1.27.3 patch baseline. Product Go source, dependency versions/sums, embeds, schema/settings and security limits are unchanged. The earlier084 CI failure on GO-2026-6809 and all Go1.27.2 candidates remain preserved and are excluded from release. [Patch investigation](go-1.27.3-release-gate.md).

- Local frozen full Windows **3266 named PASS/0 FAIL/4 platform SKIP**, test exit0, full vet0. These fresh full results include the earlier separately checked CLI admission delta; the older3253 Store.Close full run is not restated as this run.
- Local fixed-Go scanner built from cached pinned golang.org/x/vuln@v1.8.0 source under -mod=readonly; actual analyzed Go1.27.3, symbol/source mode, official DB timestamp2026-10-21T00:27:29Z, scan exit0/no findings. An initial `go run ...@v1.8.0` failed before scanning because GOPROXY=off refused deprecation metadata; retained, no full-test rerun or security exemption.
- Exact [native CI39604537169](https://github.com/hidxt/miskoai/actions/runs/39604537169), job119237235022: all12 configured steps SUCCESS, including full fixtures/vet, nativeLinux race, official pinned govulncheck, history policy1672contents/0findings and Linuxamd64/arm64 builds. Primary checked exact head/run/job/steps and actual logs, Go1.27.3 and no-vulnerabilities output.
- Three fresh native Go build exits0, actual Windows version0/exact `MiskoAI 0.1.0` and licenses0/exact128707bytes. Go version/GOOS/GOARCH/CGO0/trimpath, every linked h1 sum and PE32+/ELF64 machines verified; Linux files have no PT_INTERP/PT_DYNAMIC.
- 526 canonical tracked regular files match exact Git blob IDs before/after builds;254 compiled Go/module/embed/notices inputs also match the local full-test snapshot exactly. Source manifest SHA256 `aa7daba6b6bf3f5425f71516fcb578a0ca588dfcdbef099e09a5248ab0356b0e`; compiled manifest `e3868e66439b305fc24993a8f6d4b79be990e2fba84bf083272e4981af50a3fa`. Raw bytes retained; no LF normalization or reproducible-build claim.

The permitted gpt-6.1-sol/medium child built and read back the candidate. Primary read the full original helpers and exact fixed-toolchain delta, logs/results, freshly ran its readback, then made a separate final metadata package after CI acceptance. Binaries are byte-identical to those builds. Final archives incorporate the actual CI/local receipt, LICENSE, complete notices and operator guidance; every regular member was read back against expected bytes. Prior candidate folders are preserved.

## Final seven assets

|File|Bytes|SHA256|
|---|---:|---|
|miskoai-0.1.0-windows-amd64.zip|11678354|`8ce8a0c0ff667fe0156868782e9add8fbdf747548420c3fb0c16b2d5bade468e`|
|miskoai-0.1.0-linux-amd64.tar.gz|11531998|`8f402ecf9f5987a4ac5b2e3591dc28d790f4228219b36a4fe30c1ca0f9c53387`|
|miskoai-0.1.0-linux-arm64.tar.gz|10727025|`81a02483daab0dffb7da5674aee68d7d060f7e83ac10403b605af287e83b821c`|
|miskoai-0.1.0-source.tar.gz|1477543|`31ac07a5ee05f4c129a2dae39d6d5b932441230393214f6e5f373b02237391f8`|
|PROVENANCE.json|3482|`4113835221963f0fe04bffaba7368f732c1f7d1cf4deb09fbaf0c62e1a7f2bc0`|
|BINARY_SHA256SUMS|264|`2c877e7d04941da4011cab00c86a30f0db4115b9fed08e0b2371c5f2c209cdb0`|
|SHA256SUMS|556|`9143b9e8e216cefa43be9611fa1c1a713b562900a371e86111fa2154c35db504`|

Platform archives have5 regular root members: executable, LICENSE, THIRD_PARTY_NOTICES.txt, PROVENANCE.json, README.txt. Source has529 regular members:526 exact tracked files plus source/compiled manifests and build provenance. No extraction/install/startup hooks or credentials. Checksum file covers the other6 assets; its own digest is recorded here.

|Unpacked binary|Bytes|SHA256|
|---|---:|---|
|miskoai-windows-amd64.exe|20570112|`5d14a2839bf46cb0739d98b757d80ed9475b57e31edd944b7002d9c9a1917abe`|
|miskoai-linux-amd64|20440720|`6e925066398497df6eb17b760fdf4fbff69a415c8423a31aea63959057abbee0`|
|miskoai-linux-arm64|19271626|`4d9f4f1d2a98ec63804587529f7f9304584c11094ea215248d4990b13e2bf785`|

Complete notices SHA256 `c908e37e4a47647b7c6a40bbbf586e7681ae4163f5b0f191586d7b646da81463`; source LICENSE SHA256 `c71d239df91726fc519c6eb72d318ec65820627232b2f796219e87dcf35d0ab4`. Neither was replaced or shortened.

## Acceptance boundaries

Implementation/local/native checks above are distinct from earlier finite [live recovery](weixin-authorization-recovery-live-verification.md), [static PNG](weixin-static-image-live-verification.md), [resource47/48](server-resource-acceptance.md) and [Linux35-image browser recovery45](native-browser-recovery-verification.md). Those retain their original Go1.27.2 binary/toolchain and test scope; this patch did not replay consumed live/paid/server allowances. No new fixed-binary RSS measurement is asserted. The complete Go/src comparison found only X.509 trust filtering and version metadata changes, with media/HTTP/compiler/runtime source otherwise unchanged; X.509 hashing work is not a zero-overhead proof.

Known limits: Windows larger image library restore can exceed the unchanged30s validation deadline; existing401/cancel wording Minor; nativeGIF/stickers unsupported, staticPNG/JPEG fallback; actualarm64 runtime, installedsystemd, naturalauthorizationTTL, powercrash, indefinite/hostile/real-provider worst-case RAM unverified. Resource observations used406.57MiB-visible RAM/configured1GiBSwap and sampled0Swap; no universal512MB/no-swap guarantee. Earlier Core reopen/race-test failure remains without a proven causal explanation. Retained83/86 static warnings and dispositions are not a clean-warning claim.

Original private schema2 is unmigrated; later separate tests did not open it, while an earlier text claim changed its lifecycle state. Old/new authorization copies and old scope remain private and retained. No ordinary private Core.Run/service was started by this delivery phase. All owned acceptance processes/tunnel have ended.

The [release plan](../operations/release-0.1.0-plan.md), [notes](../operations/release-0.1.0-notes.md), [operator checklist](../operations/delivery-checklist.md) and [native operations](../operations/native-linux.md) form the delivery49 record. Tagv0.1.0/Release/uploads require the owner's final concrete approval; no public write for50 has occurred.
