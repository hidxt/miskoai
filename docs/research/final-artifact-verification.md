# Final artifact verification

2026-10-21. Package46 is accepted for the source-bound `0.1.0-dev` development candidate after independent GPT-6.1 Sol Medium verification and primary review/read-only hash/header checks. Engineering **90% =45/50**; release50 is separate.

| Artifact | Bytes | SHA256 |
|---|---:|---|
|Windows amd64|20568064|`a1e0b14b484b89f8a44ca236195a1792e372b3b4485b41e908e5053a894dc82c`|
|Linux amd64|20440305|`b68024731542fea5ceec2b527ca36932c2ade1bd54ed8f80b12953488a8a54d8`|
|Linux arm64|19271443|`32fe6208b888f5eaf471529bdda8f1515bad7453a7ab7f244e628fa6da056a16`|

Actual retained native Go1.27.2 builds exited0 for all three targets, using CGO0, trimpath, buildvcs=false and offline pinned modules. Fresh independent inspection verified correct PE32+/ELF64 architectures, linked module versions/sums, exact complete embedded notices and safe Windows version/licenses output; all263 bounded verification subprocesses exited0 with empty stderr. Primary read the verifier and evidence and freshly checked the three actual hashes/lengths/headers. No rebuild or unchanged test-suite/security-scan rerun was needed.

Production revision is `d9bc7320ba6f78350fe9d622e35e5e49d28b8f2c`; later `616ba3b8a011897ae6eaa0f79675237faa11a5fa` changes ten documentation paths only. All515 frozen public files match preserved manifest `1917fed7e0373541488d6c1c04eef947ad2012010908da24d3efe6b98e8097a7`. All254 compiled inputs plus LICENSE match production Git/current text after CRLF-to-LF normalization;247/255 also match raw frozen/current bytes. Eight line-ending-only differences are explicitly recorded: library bundle_io.go; storage attachments.go, image_replies.go, library_backup_test.go, schema4_test.go, schema5_test.go; management style.css; LICENSE. The frozen CSS bytes are the actual embedded input; this is canonical source correspondence, not an assertion that every working file is byte-identical to the build input.

Because buildvcs=false omits an embedded commit, attribution uses preserved build commands, frozen manifests and artifact hashes. No reproducible-rebuild claim is made. Both Linux files have no PT_INTERP/PT_DYNAMIC segments; this establishes static-link structure, not actual arm64 runtime or systemd installation. Linux amd64 server measurements and full resource acceptance remain separately scoped. No tag, release, deployment or final acceptance was performed by this gate.

The later exact [native CI39410051711](https://github.com/hidxt/miskoai/actions/runs/39410051711), job118585309788, succeeded on616ba3b8 with all12 configured steps; primary checked exact head/run/job/step results. Existing retained static warnings and earlier failing/partial evidence remain in their original reports.
