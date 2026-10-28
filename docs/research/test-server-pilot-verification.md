# Isolated test-server pilot — 2026-10-21/22

Engineering **98%=49/50**. Owner explicitly deferred public release and requested installation on the supplied test server for full-flow hands-on feedback. No tag, GitHub Release or public asset upload. This new trial is separate from accepted implementation/local/native CI and earlier finite live/resource gates; it adds no denominator or acceptance credit.

## Installed candidate and isolation

Exact source `dce3c1b480fbe8acb2223a498edbe9baeac7533b`, version0.1.0, Go1.27.3 linux/amd64. Installed executable SHA256 `6e925066398497df6eb17b760fdf4fbff69a415c8423a31aea63959057abbee0`. Binary, Apache LICENSE and complete third-party notices were compared with the previously verified source-bound candidate; remote packet/installed binary and embedded notices checks passed. No rebuild or product code change.

Fresh locked nologin account `miskoai-test`, nonzeroUID/GID1001, private0700 home/config/data, ordinary0600 single-link management environment/settings. Root administrative SSH installed a system unit that runs explicitly as that unprivileged account. Actual target systemd252 verification/effective properties passed: expected FragmentPath/ExecStart/EnvironmentFile, no drop-ins, NoNewPrivileges, umask0077, Restart=no, static/non-enabled unit and externalMemoryMax192MiB. No linger, firewall, package, swap or existing business-service changes. Existing local auth, model Keys and databases were not imported.

## Actual initial-management checks

- Fresh init/config validation and synthetic SQLite migration/FTS5/integrity doctor succeeded. Actual binary reported0.1.0/go1.27.3/linux/amd64; both provider-presence flags false.
- Exact process executable and actual UID1001 verified, sole application listener `127.0.0.1:18790`. Initial observed processRSS/HWM12612KiB, processSwap0. This is one initial idle observation, not a repeated60s/resource acceptance or real-provider worst-case measurement; previous resource evidence retains its Go1.27.2 scope.
- Through same-port private SSH, nine HTTP checks passed: embedded management200; unauthenticated status401; mismatchingHost403; bootstrap200; login200; authenticated status200; settings200; logout200; subsequent status401. Status was unconfigured, both Keys absent, all chat/summary/vision/search logical attempts0. Desired/effective model both deepseek-flash. Nonce/session/CSRF/password/response bodies were not logged.
- Deliberate systemctl stop completed. Manager journal records “Deactivated successfully”; original process and listener absent, unitinactive/dead/MainPID0/no restarts. The initial checker expected retained ExecMainCode1; target static-unit properties instead reset to0 with empty timestamps after stop. This checker refusal is retained separately and is not represented as observed application exit0. The subsequent manager/process/listener evidence establishes successful stop without assuming retained per-process fields.

## Retained installation/helper failures

The first installer stopped at Debian useradd: CREATE_MAIL_SPOOL is not a login.defs override key. Read-only inspection proved account/group/home/unit absent and target's actual useradd defaultCREATE_MAIL_SPOOL=no. A separately preserved installer revision asserts that default and removes only the invalid override; target syntax, manifests and installation then passed. No system default was modified and no account/path was overwritten.

The revised manifest's first hash guard compared LF in-memory text with the transferred local CRLF file and refused before installation. A separate check bound the actual immutable bytes and all members passed; original evidence/manifest remained unchanged. No checksum validation was bypassed.

An initial detached SSH helper did not retain its tunnel, so the first HTTP probe failed before any HTTP result. An owned foreground SSH terminal then supported the nine successful checks. A private key-input attempt ended before saving either Key or issuing QR; its failure/result was retained. Owner was told Keys had not been saved. A new normal-desktop private input window is staged under the same owner's explicit new-key/QR approval; no ambiguous network operation is retried.

## Current boundary and next stage

Owner explicitly selected newly hidden-input DeepSeek/Ollama test Keys and a fresh QR binding, and approved ordinary manual test-account traffic/model billing, including summaries/vision/search/allowed retries. Old finite test allowances remain spent. The owner completed both private inputs and the fresh remote firstQR; native interactive SSH/login exit0 was recorded without QR/token capture. Local new-key protected owner-only ACL and remote authorization ordinary0600/single-link/nonroot ownership passed. New Keys were exclusively installed via SSHstdin into private0600 provider.json; clean-env config validation succeeded without an API probe.

One deliberate start activated the trial. Actual exact nonroot process/loopback and effective192MiB/no-restart/static unit passed; observedRSS/HWM18856KiB/Swap0. Ten further HTTP checks passed: bootstrap/login, receiving status with both keys, text/vision deepseek-flash settings, scoped profiles/facts/library/history, logout and unauthenticated refusal. At that observation all logical attempts/messages were0. Service remains running for owner trial; first ordinary model reply is subsequently owner-confirmed below; wider full-flow acceptance remains pending. Only the new scanned allowed user/new test data scope is admitted; old local private data remains untouched.

After activation, provide private password/tunnel access and ask the owner to try deliberate text, image/document and requested-search examples, then inspect the corresponding management views. Trial feedback precedes a renewed release proposal. Backups/restores target only this fresh dataset and require deliberate reconciliation/resume; existing Windows larger-library30s limitation and static-image fallback boundaries remain.

## First ordinary text reply

Owner confirmed receiving the AI reply from the newly bound server bot. A separate authenticated counters-only read observed receiving state,1chat attempt/0summary/0vision/0search,1received/1processed/1sent/0failed/0ambiguous, successful usage139tokens. No message content or history body was read for this receipt. This establishes this single ordinary text trial path, not broad media/search/restart/restore acceptance. The service and private tunnel remain available for owner testing; publication remains deferred.

## Owner-requested test persona

Owner explicitly requested applying the proposed test persona. Primary created ali_test through the authenticated existing management API, read back all saved fields, selected it and verified exact active-profile equality; eight HTTP operations200, no service restart or model/WeChat send. The initially proposed long style was shortened to495UTF8 bytes to fit the unchanged512-byte backend contract. The UI currently permits a longer2048-character input, so that input/storage-limit mismatch is retained for trial feedback rather than silently increasing the backend budget. Wider persona behavior remains for owner next-message testing.
