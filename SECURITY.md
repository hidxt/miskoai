# MiskoAI security policy

Initial policy, 2026-10-09. Applies to every component, contributor and test fixture. Public repository; privacy and data integrity take priority over convenience.

## Assets and trust boundaries
Protect API keys, WeChat tokens, management sessions/passwords, SSH credentials, private media, chats, facts and backups. Administrators configure providers and authorize channels; chat users access only their own account/user/conversation scope. The model, retrieved web pages, documents, images, filenames and messages are untrusted. A model response cannot authorize a tool or change system policy.

## Threat model and controls
- Credential leakage: environment or owner-only files; never return secrets to browsers, print raw remote errors, or log request bodies, messages or tokens. Synthetic test data only.
- SSRF and credential forwarding: TLS to approved provider hosts; disable redirects; do not fetch arbitrary user/search URLs. Media downloads require an audited host and IP policy including DNS rebinding protection. No localhost/private/link-local/cloud metadata destinations.
- Cross-user memory disclosure: enforce account/user scope in every database read/write/export and authorization check; no model-controlled SQL.
- Duplicate side effects: persist message claim before model/tools. Sending cannot be perfectly atomic with SQLite; ambiguous send outcomes must stop for reconciliation rather than retry blindly.
- Web attacks: loopback binding, authentication, constant-time credential verification, bounded sessions/login attempts, HttpOnly/SameSite cookies, CSRF token and same-origin checks, escaped text, restrictive CSP, upload limits. No credentials in responses.
- Resource exhaustion: bounded input/output/media sizes, contexts/deadlines, global work limits, queue caps, retention caps and graceful shutdown. No unbounded goroutine per incoming message.
- Storage corruption: transactional versioned migrations, SQLite consistency backups, integrity validation before restore, restricted permissions. Never copy a live WAL database as a backup.
- Supply chain: pinned dependencies, license inventory, vulnerability/static/secret scans before publication. No remote download-and-execute install scripts.

## Prohibited behavior
No arbitrary model shell/filesystem/network capability, platform authentication bypass, fabricated identities, unauthorized external actions, telemetry/hidden uploads, local model service, containers or extra permanent runtime. Do not commit credentials, actual chats, private media, database/WAL/SHM, backup or logs. Never rewrite published history without explicit authorization. If a public leak occurs, revoke/rotate first.

## Reporting and release gates
Do not post real secrets in public issues. Use the repository's private security reporting if enabled; otherwise request a private contact without disclosing the payload. Release requires unit/integration tests, vet, applicable race check, govulncheck, gosec/equivalent, secret scan, license review and manual review. Unsupported checks stay explicitly unverified. Remote server use requires the user's informed approval under TESTING.md.
