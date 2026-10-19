# Attachment integration test import-cycle correction

Engineering progress remains 78% (39/50). This is isolated test infrastructure
work, pending primary review and integrated acceptance. Native CI, live provider,
real WeChat media, server/512MB and final acceptance receive no credit here.

## Scope and dependency

Immutable dependency: `.tools/reviews/image-final-candidate2-root-oct19-src`,
499 public files. Working/final copy:
`.tools/reviews/attachment-cycle-fix-oct19-src`. Only these paths differ:

- `internal/attachment/document_agent_test.go`
- new `internal/attachment/export_test.go`
- this report

All 498 other dependency files, including all production sources, remain exact
SHA256/length matches. The dependency and earlier candidate were not modified.
LICENSE and existing content remain preserved. No main checkout or Git changes.

## Root cause and correction

The same-package attachment integration tests import Agent. The newer Agent
image reply implementation imports attachment, yielding the setup-time cycle
`attachment -> agent -> attachment`. The normal-owner RED run reproduces that
exact compiler diagnosis and test exit 1 before any named test can execute.

Move only `document_agent_test.go` to the external `attachment_test` package.
Four small adapters in `export_test.go` reuse the existing generated DOCX,
completed private artifact, descriptor, and real reused document processor
fixtures. Test-only exports do not enter normal production builds. A local
artifact wrapper preserves the existing atomic close counter and optional
close failure callback; unused read/size override hooks are unnecessary here.

Both integration test bodies and all assertions remain, apart from qualified
helper/type references. TXT, Markdown, DOCX and PDF still use real parsers and
actual completed encrypted/decrypted private artifacts with the real Agent and
SQLite. Model/sender and the download boundary retain the same synthetic
delegates. Coverage still asserts durable claim before download, extraction
content/ranges/provenance, descriptor exclusion, artifact lifetime through model
return and before send, usage on success and cleanup failure, private cleanup,
empty-genuine-USER history exclusion, and duplicate side-effect suppression.

## Actual local verification

Native Windows verified Go 1.27.2; offline `GOTOOLCHAIN=local`, `GOPROXY=off`,
`GOSUMDB=off`, `CGO_ENABLED=0`, existing local Go caches/tmp. Existing MISKOAI_*
and provider-key environment names were cleared and synthetic keys supplied.
No private data, network/CDN/provider or live channel request was used.

Runner: `.tools/reviews/attachment-cycle-fix-oct19-check.ps1 -Phase red|green`.
Go commands, run from the isolated source:

```
go test -json -count=1 -timeout=5m ./internal/attachment
go vet ./internal/attachment
```

- RED: exact import-cycle setup failure, actual test exit 1.
- GREEN: complete attachment package, 155 named test/subtest PASS, zero FAIL or
  SKIP; package PASS, actual test exit 0. The four real-parser subtests and the
  cleanup-failure/usage integration test all passed.
- Scoped vet: actual exit 0, zero diagnostic bytes.
- `gofmt` applied to both changed Go test files.

Initial restricted RED execution stalled before producing test evidence and was
interrupted with Ctrl-C, process exit 1. A first owner retry encountered the
still-open log file before Go execution. The later completed owner RED and
GREEN are the accepted evidence; interrupted/setup attempts earn no pass credit.
The initial copy completed with 499 files before the accepted RED invocation.

Per primary task ownership, no simultaneous heavy whole-project suite was run;
primary must review the patch and rerun integrated checks. Scoped evidence does
not establish whole-product/native CI acceptance. No production behavior,
authorization, persistence policy, dependencies, protocol or resource limits
were changed, and no assertion was removed or weakened.

## Frozen evidence

Prefix `.tools/reviews/attachment-cycle-fix-oct19` contains RED/GREEN JSONL,
stderr, actual exit JSON, empty scoped-vet output, the unified patch, full source,
dependency and owned-path SHA256/length manifests, and an evidence manifest.
The freeze script compares every dependency path and rejects changes outside
the three owned paths. Source is frozen after this report; primary adoption
and integrated verification are separate work.
