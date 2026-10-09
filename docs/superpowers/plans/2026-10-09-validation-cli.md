# Phase1 validation CLI plan

> For agentic workers: use superpowers:executing-plans; the supplied user task authorizes routine implementation. Only GPT-6.1 Medium children permitted.

Goal: make protocol/provider/storage PoCs runnable from the same native binary without secrets in arguments. Spec: REQUIREMENTS.md, ARCHITECTURE.md and docs/superpowers/specs/2026-10-09-phase0-1-design.md.
Architecture: internal/cli calls validated config, provider/channel/storage interfaces; no resident runtime beyond Go. Commands never claim a live pass from mock evidence. First implement init/doctor/config validate/backup/restore, then opt-in provider and QR probes. Stop external probes until the user supplies credentials and approves costs/account use. Phase2 business development depends on Phase1 live gates in the master task.

Global constraints: Linux amd64/arm64, SQLite, native Go, 512MB target unverified, DeepSeek and hosted Ollama, Public secret gates, no containers/WSL. Brand MiskoAI, module github.com/hidxt/miskoai.

Review focus: accidentally running paid probes; credential echoes; overriding existing configuration/data; restoring a live WAL database; private/symlink file paths.

Task1 files internal/config/config.go/config_test.go: Load() (Config,error) validates loopback listener/endpoints/nonempty model/bounded timeouts; environment credentials never marshaled. Tests reject public bind, invalid endpoints and invalid numeric bounds before implementation.

Task2 files internal/cli/cli.go/cli_test.go: Run existing version/help; init creates dedicated owner-only data directory/config template exclusively and never overwrites; doctor uses temporary synthetic SQLite test not operational chat DB, prints runtime/version/features and missing-key flags only; config validate validates environment. Backup live SQLite with Store.Backup; restore only when service lock can be acquired and after integrity validation. Tests assert second init refuses overwrite, doctor reports without secrets, invalid restore is rejected before replacing existing bytes.

Task3 files internal/cli/probes.go/probes_test.go: poc deepseek/stream/search/vision commands explicitly perform real synthetic requests after env creds. Vision accepts one validated image path, size4MiB/dimension8192 and MIME sniff, base64 once. weixin login prints ephemeral QR content, polls statuses, accepts human verification code via private terminal input if needed, follows approved IDC endpoints, writes owner-only auth JSON exclusively without token stdout. Tests use injected mocks only for HTTP and verify payload/safe output; no actual auth/cost in automated suite.

Final: integrated tests/vet, native cross-builds and actual binary hash/format verification; scan/review/license inventory; update DELIVERY/TASKS/MEMORY; live protocol/provider acceptance pending user authorization. No release or server-memory acceptance at this stage.
