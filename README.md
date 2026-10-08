# MiskoAI

A lightweight AI WeChat companion built in Go, using SQLite, DeepSeek official cloud APIs and Ollama hosted Web Search. Targets native Linux amd64/arm64 on 2 vCPU / 512MB machines. Development has started; this repository is **not yet a production release**. See DELIVERY.md for actual implemented/tested status.

## Intended deployment
One `miskoai` binary, embedded Web resources, configuration/data directory and optional systemd service. No container, WSL, local model service, permanent Node/Python/Java process or external database service. Server needs a supported Linux kernel and trusted TLS CA certificates. The management listener defaults to 127.0.0.1; use an SSH tunnel for remote access.

## Configuration and credentials
`.env.example` lists safe synthetic examples. A future `init` command creates local configuration. Go does not implicitly load `.env` files; set environment variables using your service manager or a permission-restricted environment file. Never supply secrets as plaintext CLI arguments, commit `.env`, database/WAL/SHM, actual chat/media, session files, logs or backups. No API credentials/account/test server have been supplied or used.

## Build and verification
Development toolchain detected: Go 1.27.0 Windows amd64. Module will be github.com/hidxt/miskoai. Expected native build command after implementation: `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./cmd/miskoai` (set environment using your shell); arm64 uses GOARCH=arm64. Build and API capability claims require actual commands; current evidence is in DELIVERY.md.

## Operations and lifecycle
Installation/start/stop/upgrade/uninstall and backup/restore commands will be documented alongside their verified implementation. Until then no installation script or release is provided. Planned CLI commands: init, serve, status, doctor, config validate, backup, restore, version. Planned upgrade: consistent backup → stop → replace verified binary → start/migrate → health check. Uninstall must preserve user data unless explicitly requested.

## Project records
REQUIREMENTS.md is product scope; ARCHITECTURE.md describes module boundaries and budgets; SECURITY.md defines trust and prohibited behavior; TESTING.md distinguishes mocks/live APIs/WeChat/VPS; TASKS.md and MEMORY.md capture current progress. Licensed under Apache-2.0; dependency license inventory will accompany audited builds.
