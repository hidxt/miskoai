# MiskoAI

A lightweight AI WeChat companion built in Go, using SQLite, DeepSeek official cloud APIs and Ollama hosted Web Search. Targets native Linux amd64/arm64 on 2 vCPU / 512MB machines. Development has started; this repository is **not yet a production release**. See DELIVERY.md for actual implemented/tested status.

## Intended deployment
One `miskoai` binary, embedded Web resources, configuration/data directory and optional systemd service. No container, WSL, local model service, permanent Node/Python/Java process or external database service. Server needs a supported Linux kernel and trusted TLS CA certificates. The management listener defaults to 127.0.0.1; use an SSH tunnel for remote access.

## Configuration and credentials
`.env.example` lists safe synthetic examples. `init` creates a private non-secret settings.json template; in this development checkpoint it is not loaded and the environment is authoritative. Go does not implicitly load `.env` files. Never supply secrets as CLI arguments or commit database/WAL/SHM, actual chats/media, sessions, logs or backups. One authorized cloud test batch has passed with privately supplied temporary keys. deepseek-flash is the owner's explicit model choice for text and vision; endpoints/models remain configurable. Human QR/account acceptance remains pending.

## Build and verification
Development toolchain: Go1.27.0 Windows amd64. Module github.com/hidxt/miskoai. Current CGO_ENABLED=0 Windows amd64/Linux amd64/Linux arm64 builds compile; Linux execution remains unverified. From a native Unix development shell:

```sh
go test ./... -count=1
go vet ./...
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o dist/miskoai-linux-amd64 ./cmd/miskoai
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o dist/miskoai-linux-arm64 ./cmd/miskoai
```

For this Windows workspace, `./scripts/Test.ps1` sets workspace-local Go cache/temp paths and runs the synthetic suite/vet. These are development tools; no Python or PowerShell daemon is required on the target server. CI uses a native Linux runner, without containers; configured CI is not a completed run.

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
No installation script or release exists yet. `backup PATH` uses a consistent live SQLite snapshot into a new private file. `restore PATH` requires the service stopped, a dedicated data directory and no existing WAL/SHM; it validates schema/integrity before replacement and preserves an existing database under a private pre-restore name. Existing files are never overwritten by `init` or backup. Never publicly upload snapshots. Planned upgrade: consistent backup → stop → replace verified binary → start/migrate → health check. Uninstall must preserve data unless explicitly requested. systemd, Web management, integrated chat, personality, summaries, documents and stickers are pending.

## Project records
REQUIREMENTS.md is product scope; ARCHITECTURE.md describes boundaries/budgets; SECURITY.md defines trust/prohibited behavior; TESTING.md distinguishes mocks/live/WeChat/VPS; TASKS.md and MEMORY.md capture progress. DELIVERY.md is the acceptance record. Apache-2.0; dependency license inventory is in docs/research/dependency-licenses.md, with full notices embedded in `miskoai licenses`.

Development progress: bounded text orchestration and storage2 receive primitives now pass synthetic integration and independent review. Text commands include `/remember`, `/memory`, `/forget`, `/memory clear`, `/export-memory` and explicit `/search`/`搜索：`; they are internal tested behavior pending runtime/CLI exposure. Three builtin styles preserve honest AI identity. There is no runnable integrated chat service yet. Real WeChat validation is deferred by the owner; real512MB measurement awaits an authorized server.
