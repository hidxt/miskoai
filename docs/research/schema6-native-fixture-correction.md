# Schema6 native fixture correction

2026-10-20; engineering progress remains **78%** (39/50). This is a synthetic test-fixture correction. Implementation, local verification, native CI, live tests and final acceptance remain separate evidence categories.

The primary supplied the actual failed native run [39238746403](https://github.com/hidxt/miskoai/actions/runs/39238746403), job118014131198, for commit `3e06732449f447a7c39dcfab0be42da331c4d173`. Unit/integration failed at the schema6 legacy migration, corrupt-backup admission, historical deleted-asset reopen and image clear/backup fixtures. Vet, race, vulnerability and build steps were skipped. That RED result is preserved; this Windows task does not replace it with a native success claim.

The four fixtures placed database files directly inside `t.TempDir()`. Verified Go1.27.2 `src/testing/testing.go:1636` creates the leaf temporary directory using `os.Mkdir(dir, 0o777)`. On Unix, the effective permissions depend on umask; with the usual022 mask the leaf is0755. Storage `privatePath` intentionally rejects an existing Unix parent with any group/other permission (`Perm() & 0077 != 0`), and never chmods an unrelated existing directory. Windows bypasses this Unix-bit check and has its separate ACL boundary. The actual native failures plus these source contracts explain the fixture mismatch; no Unix execution or umask measurement was performed in this Windows task.

All four fixture paths now include a dedicated `private` child directory. Backup's existing `privatePath` creates a missing child with0700. The legacy fixture calls `os.Mkdir(filepath.Dir(path), 0700)` before the manual `sql.Open`/DDL construction, because database/sql does not create missing parent directories. This follows the existing private-child pattern in schema5 and library backup tests. The unchanged production Open/Backup path admission supplies the relevant Unix privacy assertion when native CI runs; no redundant test of the literal path construction was added.

The complete product diff is four path expressions and the three-line legacy parent creation. Legacy schema1..5 DDL, manifest checks, existing corruption mutations, immutable refusal side-effect assertions, history/receipt expectations and no-replay assertions remain intact. No production, migration strategy, dependency, permission policy, retry, skip or test assertion changed.

Verification uses the native verified Windows Go1.27.2 toolchain, offline `GOTOOLCHAIN=local`, `GOPROXY=off`, `GOSUMDB=off`, `CGO_ENABLED=0`, project-local caches/tmp, with MISKOAI_* and DEEPSEEK_API_KEY/OLLAMA_API_KEY environment variables removed by name. Frozen tracked source contains504 files. Commands, immutable preimages, full source manifest, scoped JSON test events and vet output are retained under ignored `.tools/reviews/schema6-native-fixture-oct20-*`.

Commands executed from the frozen source:

```text
go test -json -p=1 -count=1 -timeout=5m -run '^(TestSchema6|TestImageReply)' ./internal/storage
go vet -p=1 ./internal/storage
```

The scoped Windows tests completed64 named passes,0 failures,0 skips; package elapsed36.964s, actual test exit0. Storage vet completed with actual exit0 and empty diagnostics; the runner joined with process exit0. All504 frozen source identities remained unchanged. The ignored summary is `.tools/reviews/schema6-native-fixture-oct20-summary.json`; test JSON, empty test stderr, vet output and exits JSON remain separate original artifacts. An initial summary-reader attempt ran before the exits file existed; a later reader expected UTF16 for the ASCII exits JSON. Both were evidence-reader errors only; the corrected BOM-aware reader consumed the original files without altering them or rerunning Go.

Changed source SHA256 identities:

| File | Bytes | SHA256 |
|---|---:|---|
| `internal/storage/schema6_test.go` |9098|`584744afde4ba89d1cc9bf073de9e12138dd0b9b761ff1dd6551ef78a41a3900`|
| `internal/storage/image_replies_test.go` |16655|`60589d4da14823b9548b29becd9d983607f33bffdfca8a9ccc4c717d4364dcf1`|

Preimage SHA256 identities are respectively `2ef856ec239c0499825d4806e0d8914a0137f2550abadaeb92de3c90630cecb1` and `b57f8ec1ad2bdf662344fcb2ed70395933ca786468a47535a477f4eba77b8967`. The verified Go testing source SHA256 is `27c59df1832eae458ab8d2d4b6a6252bbd1ea2a3001734910d1ce2decff16f81`.

This is a scoped fixture check, not a new full-project run. The earlier coherent Windows3249-pass/full-vet0 result remains a separate historical source-bound gate. Primary review and a subsequent exact-source native CI gate are still required. No live provider, WeChat, private database, server, arm64 runtime,512MB workload or final acceptance evidence is added.
