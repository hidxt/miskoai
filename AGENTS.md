# MiskoAI agent collaboration

Read REQUIREMENTS.md, SECURITY.md, ARCHITECTURE.md, TASKS.md, MEMORY.md and DELIVERY.md before work. Preserve LICENSE and existing content. Module: github.com/hidxt/miskoai. Brand: MiskoAI; repository: miskoai.

## Responsibility
The primary GPT-6.1 High agent owns architecture, scope decisions, integration, security, performance and acceptance. Child agents may use **only GPT-6.1 Sol Medium** (owner updated the rule on 2026-10-09). The primary chooses each task's effort explicitly; no alternate model. Before every child dispatch/resume, report the exact model and reasoning effort to the user together with whole-project progress. Every progress report uses xx% engineering progress from docs/superpowers/project-progress.md and distinguishes implementation, local tests, native CI, live tests and final acceptance. Harness identifier is gpt-6.1-sol with reasoning_effort medium only.
Medium: protocols, provider/storage design, concurrency, complex fixes, security review. The earlier Low assignment is superseded: bounded fixtures/tests, CLI/static views, templates and documentation also use Medium. No alternate model or effort may be used by a child.

## Handoff and file ownership
Each task states goal, dependencies, owned files, interfaces, tests and limitations. Do not edit another agent's files. No concurrent modification of a file. Return actual commands/results, changed files and unresolved issues. Primary reads code/diff and re-runs integrated checks; agent assertions alone are not evidence. Use a fresh reviewer where practical.

## Recovery and decisions
Update TASKS.md, MEMORY.md and DELIVERY.md at phase boundaries; record implementation choices in DECISIONS.md. MEMORY is development state only; never user chats or credentials. Scope changes involving provider, SQLite, channel strategy, deployment, safety or 512MB require explicit user approval. Routine internals proceed autonomously under the user's master task. The user has explicitly requested continuous development without routine approval interruptions; skill process gates do not add permission requirements for already-authorized work.

## Git and test hygiene
No broad staging before inspection. Before every commit/push: inspect ignore coverage, status, staged diff and secret scan. Before first new public push: scan full history and working tree. Do not force push. Use native Go builds; do not add container/WSL workflows. Separate mock, Windows host, live provider, real WeChat and native Linux/server evidence.
