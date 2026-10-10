# MiskoAI

A lightweight AI WeChat companion built in Go, using SQLite, DeepSeek official cloud APIs and Ollama hosted Web Search. Targets native Linux amd64/arm64 on 2 vCPU / 512MB machines. Development has started; this repository is **not yet a production release**. See DELIVERY.md for actual implemented/tested status.

## Intended deployment
One `miskoai` binary, embedded Web resources, configuration/data directory and optional systemd service. No container, WSL, local model service, permanent Node/Python/Java process or external database service. Server needs a supported Linux kernel and trusted TLS CA certificates. The management listener defaults to 127.0.0.1; use an SSH tunnel for remote access.

## Configuration and credentials
`.env.example` lists safe synthetic examples. `init` exclusively creates private non-secret settings.json, which is loaded as runtime configuration. Explicitly present environment variables override saved values; invalid or empty overrides fail instead of silently falling back. Desired/effective settings are separate non-secret snapshots. Existing dedicated directories/files must already be owner-private; initialization never repairs unrelated permissions. The parent directory must already exist. Go does not implicitly load `.env` files. Never supply secrets as CLI arguments or commit database/WAL/SHM, actual chats/media, sessions, logs or backups. One authorized cloud test batch has passed with privately supplied temporary keys. deepseek-flash is the owner's explicit model choice for text and vision; endpoints/models remain configurable. API keys may come from environment or the explicitly selected `MISKOAI_CREDENTIALS_FILE` (exact uppercase DeepSeek/Ollama key schema, verified owner-only, <=8KiB); no secrets file is discovered automatically. Admin password remains environment-only. Human QR authorization and controlled text delivery have been verified; strict server acknowledgement, media, reconnect and expiry acceptance remain separate gates.

## Build and verification
Development toolchain baseline: Go1.27.2 after the official security patch. Module github.com/hidxt/miskoai. CGO_ENABLED=0 Windows amd64/Linux amd64/Linux arm64 builds compile. Reviewed checkpointf6fcb3cba1e4524648602bbcaa68495f3449dd25 passed native Linux unit/integration, vet, race, vulnerability/history checks and both Linux architecture builds in [CI run37976808001](https://github.com/hidxt/miskoai/actions/runs/37976808001). That revision includes strict runtime configuration, scoped management-storage APIs, ownership-checked maintenance the joined Core controller and bounded private media codec; subsequent Web working source is excluded. This hosted runner evidence does not establish arm64 runtime or512MB/VPS performance. From a native Unix development shell:

```sh
go test ./... -count=1
go vet ./...
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o dist/miskoai-linux-amd64 ./cmd/miskoai
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o dist/miskoai-linux-arm64 ./cmd/miskoai
```

For this Windows workspace, `./scripts/Test.ps1` sets workspace-local Go cache/temp paths and runs the synthetic suite/vet; ensure its selected Go toolchain satisfies go.mod. These are development tools; no Python or PowerShell daemon is required on the target server. CI uses a native Linux runner, without containers; only completed runs tied to exact revisions count as evidence.

The development binary supports `version`, `licenses`, `init`, `doctor`, `config validate`, `backup PATH`, `restore PATH`, `weixin login` and `poc` commands. `doctor` uses only a synthetic local SQLite database, reports boolean credential presence, and never calls cloud or WeChat APIs. `serve` and `status` are not implemented yet.

## Authorized local integration tests
Cloud tests can incur small charges; explicit authorization is required. The project owner has authorized one bounded synthetic batch for this session. Do not rerun batches without checking that request allowance. Current Windows helper accepts private local credentials only, never arguments:

```powershell
Set-Location C:\Users\auzasr\Documents\Projects\miskoai
./scripts/Set-TestCredentials.ps1
# After the development Windows binary is built:
./scripts/Invoke-LiveProbes.ps1
```

The first script prompts with hidden input and creates ignored secrets/api-keys.json with protected current-owner-only ACL before writing. The second performs one fixed text, stream, search and generated PNG probe, caps output64tokens, restores prior process environment, and prints only status/counts. It checks the credential ACL before reading. Explicit429/503 may retry at most2; stream/ambiguous failures are not retried, so this batch makes at most10 cloud HTTP attempts, within the authorized12.

For human WeChat authorization, run `./scripts/Invoke-LiveProbes.ps1 -Weixin` in a private interactive terminal. Do not record/share QR output. Scan the official authorization with your own account; any one-time code uses hidden input. Authorization is stored only after validating the official endpoint and securing file permissions. Then send exactly `MiskoAI synthetic integration test` from the scanning user and press Enter for one bounded receive/reply probe; other messages are ignored before persistence. This does not prove media/reconnect/cursor acceptance. Real tokens, links and messages must remain outside Git/logs.

## Operations and lifecycle
No installation script or release exists yet. `backup PATH` uses a consistent SQLite snapshot into a new private file. Backup and restore acquire the same exclusive lifecycle lock before opening the database. `restore PATH` requires a stopped service, a dedicated data directory and no unexplained WAL/SHM/rollback journals; it validates supported schema/integrity before candidate admission, preserves the source and prior database, and retains a private pause marker pending explicit reconciliation. Linux directory syncing precedes replacement; failures may require recovery using the retained prior snapshot. Windows checks establish normal restart behavior, not Linux-equivalent power-loss durability. Explicit administrative resume exists in reviewed Core; Web/CLI exposure remains pending. Existing files are never overwritten by `init` or backup. Never publicly upload snapshots. Planned upgrade: consistent backup → stop → replace verified binary → start/migrate → health check. Uninstall must preserve data unless explicitly requested. Durable serial text handling, schema3 memory/profile storage and a bounded summary worker are reviewed internal modules; Core lifetime is internally reviewed; operational serve, Web management, media/documents/stickers and systemd release integration remain pending.

## Project records
REQUIREMENTS.md is product scope; ARCHITECTURE.md describes boundaries/budgets; SECURITY.md defines trust/prohibited behavior; TESTING.md distinguishes mocks/live/WeChat/VPS; TASKS.md and MEMORY.md capture progress. DELIVERY.md is the acceptance record. Apache-2.0; dependency license inventory is in docs/research/dependency-licenses.md, with full notices embedded in `miskoai licenses`.

Development progress: bounded text orchestration, durable serial receive handling, derived summaries/candidates and persistent profile storage have synthetic/reviewed evidence recorded in DELIVERY.md. Text commands include `/remember`, `/memory`, `/forget`, `/memory clear`, `/export-memory` and explicit `/search`/`搜索：`; they remain internal behavior pending lifecycle/CLI exposure. Candidate review and persistent-profile agent integration passed independent Medium review and root local verification; checkpointc17c6ea native CI passed. Strict runtime configuration/raw-path correction and scoped management-storage APIs passed independent Medium reviews, root local verification and native Linux CI at e421940. Shared maintenance passed local review/integration after the backup ownership correction; its native CI is pending. Core/serve and authenticated management remain pending. Three builtin styles preserve honest AI identity. There is no runnable general chat service yet. The owner resumed bounded real WeChat validation; a fresh synthetic marker is pending for the reviewed strict-ACK rule, and prior ambiguous sends are never resent. Real512MB measurement awaits an authorized server.

Current engineering progress: **52%** per docs/superpowers/project-progress.md. Core/media codec local/reviewed/nativeCI gates passed. Loopback authentication and the shared public-IP-pinned transport constructor are locally reviewed with root synthetic tests/vet; their new native CI is pending. Management APIs/view and operational serve remain in development. This percentage is a coarse local delivery tracker, not real-channel/resource/final acceptance.
