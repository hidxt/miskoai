# JSON-mode staged binary: prepared suffix recovery

Engineering **98%**. **Explicitly approved and completed**: one-time suffix upload/hash, retained-prefix assembly, complete5-file verification, one stop/private full backup/fixed install/one start,16SSH/23HTTP all0. Product source eec21cd and exact nativeCI41174113939/all12SUCCESS are unchanged. Original failure and first reconciliation records remain preserved; no automatic replay. See [actual server evidence](../research/image-json-mode-server-update-verification.md). Live natural-image and final acceptance remain open.

## Actual observed state

The owner approved2 read-only checks with conditional continuation only if the existing binary was complete. Both checks exited0. Old unit was active and old binary hash matched; prior3 helpers matched and start.sh was absent. The staged binary was only **11,188,224 bytes**, SHA256 `3de21042a52d8968da013cd15a7193f6926f40621443d1d44de2cdcc9d477f57`; complete was false. No writer was observed. Primary stopped without any remote write, retry, stop, backup, replacement or start.

Local-only verification proves this fragment is exactly the first11,188,224 bytes of the approved13,897,888-byte binary. The missing suffix is **2,709,664 bytes**, SHA256 `38676e27d0473101bf24d5974abd3c83e9057f2d6d7fe04b15272d2794b91775`. Local prefix-plus-suffix reconstruction hashes exactly to approved full SHA256 `869ce5c2714c52acfdaee6dacaa503d008759da534e1be24cab490b147d40488`. This does not establish the original transport failure's cause.

## Proposed one-time scope

After a new explicit owner decision, the same pinned SSH client/key may perform one bounded suffix recovery in the existing private directory `/home/miskoai-test/.local/share/miskoai-update-20261031-image-json`:

1. Syntax-check the prepared fixed Bash fragments and freshly inspect the old active binary, original fragment identity/hash/no writer, prior3 helpers, absent start.sh and unused recovery paths. Any mismatch stops before transfer.
2. Upload **only the2,709,664 missing bytes once**, to new `miskoai-linux-amd64.suffix-11188224`, with a60s total transfer bound and no retry. Verify exact length/hash/regular-file identity. Do not retransmit the complete binary or overwrite the existing fragment.
3. Recheck both immutable inputs and no open writer, assemble them into new `miskoai-linux-amd64.assembled-once`, verify full length/hash and sync. Preserve the original fragment by moving it once to `miskoai-linux-amd64.retained-prefix-11188224` without clobbering any target, then publish the verified assembled staging file at the original staged basename. Both retained prefix and suffix remain private. No live binary is touched here; no file deletion or partial cleanup.
4. Freshly reconcile full staging identity. Only after exact full hash admission, complete ownership/mode admission, upload unchanged start.sh for the first time and verify all5 original packet files. Then perform fresh management preflight and the previously approved, never-started one graceful stop/private consistent full backup and old copies/fixed install/one start/post-check sequence.

New network uploads are at most2,709,664bytes plus3,260bytes for the previously unstarted start.sh, **2,712,924bytes total**. Reconciliation/verification/syntax are bounded metadata or hash operations. At most16 SSH operations and23 authenticated/static management HTTP checks cover the full proposed sequence; existing original maintenance240s and start60s ceilings remain. All extra artifacts stay within the existing400MiB packet budget;64MiB data admission, unit/resource/firewall/Swap and provider limits remain.

Any missing/incomplete/different/uncertain result stops immediately, preserving original and new artifacts. No automatic replay, further transfer attempt, timeout extension, rollback, old sending-history restore or service restart. Current password/Keys/WeChat/data remain; normal owner chat/model billing resumes after successful start under the existing trial authorization. No new automated paid/provider/WeChat test, installation, tag or Release. A live natural-image owner check is still required after successful deployment.

The earlier2-request read-only approval is consumed; it does not authorize this write/assembly sequence. The new local helper uses a distinct owner-decision guard; original product/packet hashes and exact native CI are required. Local reconstruction proof is complete; Linux syntax/runtime recovery has not run.

One concentrated independent review used **GPT-6.1 Sol Medium** (`gpt-6.1-sol`/`medium`),0 actionable C/I/M. Primary read the report, reconfirmed local reconstruction, all266 unchanged product inputs and0 PowerShell AST errors. Frozen helper SHA256 **5434b559488fc4fbe52cdc23c5b5315a74121f2efb60eb492c92145a4f1da24f**. Original5-file packet lengths/hashes and manifest remain exact. This is local static/byte evidence; it does not authorize or claim a Linux recovery execution.
