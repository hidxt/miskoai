# Trial revisions local verification — 2026-10-29

Engineering remains **98% (49/50 historical phase packages)**. Owner explicitly started the previously recorded F01–F08 changes. All eight groups are implemented in the working candidate; this record does not accept the revised candidate as deployed, native-CI verified, live-tested or released.

## Candidate and scope

The primary froze 540 public files over base `df32a022b631ee993dcf509388ed1af46fbb6715` before integrated verification. Go 1.27.3 was checked by SHA256. No module/dependency, schema, provider transport, image sender lifecycle or resource-budget change was introduced. Later evidence/operations documentation edits are outside that initial full-tree identity; compiled inputs are checked separately. Existing pilot remains on its original accepted binary.

| Feedback | Implemented behavior | New evidence |
|---|---|---|
| F01 | Bounded Chinese question fragments reach existing scoped literal fact search | Actual Agent/model-context regression, foreign/expired/unrelated facts and quotas |
| F02 | Same Chat call suggests at most one offered static image; no random/literal gate | Catalogue/output bounds, invalid advice, cooldowns, cancellation, receipts and once-only lifecycle |
| F03 | Username plus password; old deployment defaults to account `admin` | Strict login/nonces/rate/session/CSRF tests; actual synthetic browser login |
| F04 | Hours/minutes, compact counts, derived diagnostic codes, newest three events and independent page | Actual details expansion, `/events`, newest-first pages 1/2 and return preserving login |
| F05 | One backup/restore workspace groups both supported formats | Actual responsive layout; existing restore/clearing regression tests retained |
| F06 | Readonly settings until Edit; Save/Cancel; defaults remain a draft | DOM readonly property; visible edit/default/cancel checks and Node behavior tests |
| F07 | 人物设定; no builtin choices; name/style and optional advanced preferences | Actual create/select, existing persona retained, 513-byte style refused; ID-preservation tests |
| F08 | Cool glass surfaces, local fonts, responsive forms/navigation | Actual 1280px and 390px checks; all five main views fit mobile width; PNG preview decoded 48×48 |

## Primary checks

- Full Windows native `go test -p=1 -json -count=1 -timeout=10m ./...`: **3319 named PASS / 0 FAIL / 4 platform SKIP**, exit 0, 1100.88 seconds. Full `go vet -p=1 ./...`: exit 0. No platform skips are credited as runtime passes.
- Independently rerun Node suites: web 11, library 23, bundle 27, **61 total PASS**, all exits 0. Earlier count 62 was corrected by reading the actual receipts.
- Go 1.27.3 source/symbol `govulncheck` against the official database: exit 0, **0 findings**; database timestamp `2026-10-27T21:16:18Z`. This is a known-vulnerability check, not proof of absence of unknown vulnerabilities.
- Native Windows amd64, Linux amd64 and Linux arm64 build commands all exit 0. Linux amd64 candidate: 13,885,600 bytes, SHA256 `4662ee3458227996673cb1984a4f3b9f332467279f730f804c3f4486c8909a16`. Cross-builds are not native Linux/arm64 runtime evidence.
- Fresh independent GPT-6.1 Sol/Medium review of immutable backend/UI and final Agent deltas: **0 unresolved Critical / Important / Minor**. Reviewer independently reran the three JS suites; author Go receipts were inspected, not reviewer-executed. Primary then ran the full integrated Go suite above.
- Root adapted only two old Core plaintext image-model mocks to choose from the actual offered catalogue. The original lifecycle assertions remain; captured original targeted RED precedes the passing integrated suite. No Core production delta.
- A narrow independent follow-up reviewed that Core fixture diff, the username operator correction and prepared maintenance scope: 0 unresolved C/I/M. Primary checked all 260 compiled/embedded/notice inputs against the tested immutable candidate with no differences, verified three binary metadata/hash records, unchanged LICENSE and exact emitted embedded notices. Initial notice comparison used an incorrect assumed root filename and failed before comparison; the corrected check uses the actual embedded `internal/notices/notices.txt`, without source/build changes.
- History plus initial working-tree secret policy scan: 1712 contents, 0 findings, exit 0. Generated binaries, synthetic databases and private credential directories remain ignored. The final documentation-only delta receives a fresh working-tree scan before any proposed source publication; no staging/commit/push has yet occurred.

Synthetic browser fixture uses actual revised backend/UI with the accepted baseline Agent, never calls `Core.Run`, and denies external delegates. First fixture completed its 30-minute lifetime with joined cleanup and zero external calls; the initial browser opening timed out and is not counted as a pass. The fresh fixture supplied the actual browser checks above, then accepted `quit`: run/close both succeeded, process exit 0 and zero external calls. Temporary viewport overrides were reset. A final browser logout action was unavailable after browser control disconnected and is not counted as an actual-browser pass. This browser evidence does not cover the revised Agent's real-provider behavior, OS file picker, an actual restore submission, or full assistive-technology/motion emulation. Existing restore warnings and Windows large-library 30-second limit remain.

## Remaining gates

Fresh exact-source native Linux CI/race, test-server maintenance and owner verification of natural recall/semantic image replies remain pending. Previous CI, live and resource reports retain their original source/workload scope. No new paid/WeChat request, real-data migration, public tag/Release or server binary replacement was performed for this record. The lexical recall correction does not implement synonym search or a vector database. Static PNG/JPEG reply support does not imply native animated stickers.
