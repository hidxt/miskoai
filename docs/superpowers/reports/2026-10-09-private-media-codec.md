# Isolated Task1: private bounded media codec

Implemented 2026-10-10 by GPT-6.1 Sol Medium (`gpt-6.1-sol`, reasoning effort
`medium`), with no children. Engineering progress remains **48%** pending the
primary's independent review/integrated acceptance. This report supplies local
Windows implementation/test evidence only. Codec native CI, native race,
actual media/backend tests and final acceptance remain pending.

## Scope and interfaces

New files owned by this task:

- `internal/media/codec.go`
- `internal/media/spool.go`
- `internal/media/codec_test.go`
- `internal/media/spool_test.go`
- this report

No existing source, license, root ledger, dependency or global setting was
modified. No Git mutation, external network, credentials/private payload,
provider, CDN, model, service, parser or actual WeChat operation occurred.
The implementation follows the isolated brief and pinned source research in
`docs/research/weixin-media-design-evidence.md`; no Tencent source was copied.

`ParseKey` gives present image hex strict precedence, rejects invalid preferred
hex without fallback, and bounds base64 spelling before decoding. Standard
strict base64 is re-encoded and compared exactly; only raw16 or ASCII hex32
decoded forms are accepted. Errors expose constant sentinels only.

`Encrypt` streams up to 4MiB actual plaintext, always includes PKCS7 padding,
and permits empty plaintext as a codec operation. `Decrypt` bounds actual
ciphertext to 4MiB+16, retains the final16-byte block, verifies every PKCS7
padding byte and bounds the resulting plaintext to 4MiB. Invalid/truncated
ciphertext never returns an Artifact. ECB is compatibility only and supplies
no authenticated encryption; downstream content validation remains mandatory.

The input scratch and buffered output are each16KiB, plus two16-byte block
arrays. Reads are bounded further to the first excess byte; neither codec
allocates a whole input/output buffer. One pending block spans partial reads;
decrypt also retains one final block. One hundred consecutive zero-progress
Reader calls end with safe IO failure. No codec goroutine is started.

The spool uses a random128-bit name and privatefs exclusive owner-only
creation in a dedicated private directory. Immediately after creation, the
handle's FileInfo identity is forced through `os.SameFile(original, original)`
while its original path exists, before payload. This preserves Windows lazy
FileInfo identities across closure/replacement. Privacy and original identity
are checked before payload and again before publication. Sync and write-close
must succeed; re-opened read handle, original identity, actual size and privacy
must agree. The Artifact exports only `Read`, `ReadAt`, `Size`, `Close`.

Artifact Close serializes with reads, is idempotent, closes its handle and
removes only the matching original main file. Failure cleanup makes the same
original-identity comparison before removal. Narrow private per-call seams
exercise write/short-write/sync/close failures and exact replacement points;
there are no exported or mutable global hooks.

## Runtime TDD and exact local checks

Every command ran from `C:\Users\auzasr\Documents\Projects\miskoai` using
`.tools/toolchains/go1.27.2-verified/go/bin/go.exe`. Version output was exactly
`go version go1.27.2 windows/amd64`. Environment for each Go invocation:

```powershell
$mediaRoot = (Get-Location).Path
$env:GOTOOLCHAIN = 'local'
$env:GOPATH = "$mediaRoot/.tools/go"
$env:GOMODCACHE = "$mediaRoot/.tools/gomod"
$env:GOCACHE = "$mediaRoot/.tools/gocache"
$env:GOTMPDIR = "$mediaRoot/.tools/tmp"
$env:TEMP = $env:GOTMPDIR
$env:TMP = $env:GOTMPDIR
$env:GOPROXY = 'off'
$env:GOSUMDB = 'off'
```

Tests and the allocation probe used the authorized normal-user execution
token (`require_escalated`) with synthetic private fixtures; vet ran under the
workspace token. No ACL check or product guard was relaxed.

| Command after the environment setup | Result | Retained ignored evidence |
| --- | --- | --- |
| `go.exe test -count=1 -json ./internal/media` against explicit finite not-implemented stubs | exit1; 0pass/44fail/0skip test entries, seven top-level groups. Runtime assertions reported missing behavior; compilation succeeded. | `.tools/reviews/media-codec-red.json` |
| same focused command after implementation | exit1; only artifact replacement Close fixture and its parent failed because Windows refused renaming the open read handle; other fixture groups passed | `.tools/reviews/media-codec-green1.json` |
| `go.exe test -count=1 -run '^$' -bench '^BenchmarkMediaMaximumEncrypt$' -benchtime=1x -benchmem ./internal/media` | exit0; PASS; one generated maximum-plaintext encryption, 86,164,600ns/op, 171,104B/op, 1,819allocs/op | `.tools/reviews/media-codec-allocation.txt` |
| `go.exe test -count=1 -json ./internal/media ./internal/privatefs` on final source | exit0; media44pass/0fail/0skip; privatefs17pass/0fail/2skip; total61pass/0fail/2skip test entries (parents/subtests counted) | `.tools/reviews/media-codec-final-tests.json` |
| `go.exe vet ./internal/media ./internal/privatefs` | exit0; no output | `.tools/reviews/media-codec-final-vet.txt` |

Final platform skips were `TestPrivateDirNeverRepairsExisting` (Unix-style
broad permissions; explicit broad DACL rejection passed in the Windows test)
and `TestPrivateSymlinkTraversal` (normal host lacks symlink creation
privilege). The existing Windows junction, broad ACL, inherited-sidecar and
raw-path alias fixtures passed. No failures were omitted.

## Review focus mapped to evidence

| Required behavior | Named fixture and observation |
| --- | --- |
| Strict source key forms and precedence | `TestMediaKeyFormsAndPrecedence`: raw base64, base64 ASCII hex, mixed-case preferred hex, ignored invalid fallback, invalid preferred key, whitespace, noncanonical padding bits, invalid hex, unsupported/excess lengths |
| Independent wire vector and complete padding | `TestMediaECBKnownVectorAndPadding`: FIPS-197 AES block `69c4e0d86a7b0430d8cdb78070b4c55a` for key `000102030405060708090a0b0c0d0e0f` and plaintext `00112233445566778899aabbccddeeff`; extra decrypted block equals sixteen16 bytes; lengths0 through31 round-trip |
| Maximum aligned block includes extra padding | `TestMediaMaximumAlignedRoundTrip`: generated4MiB plaintext produces4MiB+16 ciphertext and decrypts to exactly4MiB with every byte checked; plaintext maximum+1 and ciphertext maximum+1 return limit/nil Artifact; no spool remains |
| No invalid plaintext exposure | `TestMediaRefusesInvalidCipherBeforeExposure`: empty,15-byte,17-byte and one-byte ciphertext, zero/oversized/mismatched PKCS7; each returns invalid/nil Artifact and leaves no spool |
| IO and cooperative cancellation cleanup | `TestMediaIOAndCancellationCleanup`: both modes cover actual reader error, writer error, short write, sync error, write-close error, cancellation during read and pre-expired deadline; exact safe errors or context identities and no remaining spool |
| Same-path replacements retained | `TestArtifactReplacementOwnership`: replacement before reopen returns private/nil Artifact; replacement before failure cleanup preserves original IO error; replacement before Close returns stable private refusal; replacement bytes remain unchanged |
| Completed handle and idempotent concurrent Close | `TestArtifactReadAtAndConcurrentClose`: published privacy verification and ReadAt, eight concurrent Close calls with nil result and no spool, reads after Close return safe IO |

The initial Windows replacement Close fixture attempted renaming while the
normal `os.Open` handle was live. Windows denied the rename, and t.TempDir
cleanup also reported the still-open handle; that failing evidence is retained.
The corrected synthetic fixture closes only its private internal read handle
before replacing the path, retaining the captured original identity. This
tests replacement refusal when replacement is possible; it does not claim a
normal live Windows read handle permits rename. Root confirmed this fixture
setup. Production ownership, ACLs, opening and cleanup were unchanged.

## Self-review, rulings and limitations

Self-review examined strict parsing, actual read/output bounds, maximum aligned
padding, invalid final-block withholding, safe error paths, context checks,
handle closure/reopen and original identity on every cleanup path. No unresolved
blocking task issue was found in that author review. Independent review remains
the primary's responsibility; this is weaker than a fresh reviewer.

Ruling: use the private synthetic handle-close setup above for the Windows
artifact replacement Close fixture. It directly checks original identity after
replacement while preserving ordinary Windows share behavior. Cost if wrong:
an OS sharing or cleanup edge could escape this fixture; root/fresh review and
native platform evidence remain necessary. No deferred minor finding was made.

- Cancellation is cooperative between actual reads and surrounding writes/
  publication. A blocked arbitrary Reader or filesystem call is not forcibly
  interrupted. No forced deadline, RSS or abandoned-worker guarantee is made.
  Later outer admission must remain occupied until actual work/Close completes.
- The allocation result includes Windows privatefs/ACL and fixture work for one
  finite generated maximum encryption. It is not a calibrated allocation ceiling,
  decrypt allocation probe, heap profile, process RSS, throughput promise or
  512MB/VPS evidence. Concurrent codec admission and durable quotas belong to
  later integration.
- Ownership uses the existing privatefs same-OS-user trust model; it does not
  defend against a malicious process as that user racing checks and removal.
  Callers must preserve completed-file immutability. Failure cleanup attempts
  owned-only removal; arbitrary OS removal/close failures can leave an original
  spool. Before a usable original identity is established, unsafe Create refusal
  cannot authorize deletion of an unknown object.
- No native Linux/arm64 execution, native race, full integration/scanners,
  backend format acceptance, real encrypted media, GIF/sticker, parser/image
  validation, actual service startup or final release is established here.

Source/tests and this report are frozen for the primary's independent review.
