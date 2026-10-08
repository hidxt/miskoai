# Development memory

2026-10-09 Phase0/1 checkpoint. This file contains agent engineering state only.
- Repository initially has LICENSE only, clean main tracking origin/main, initial commit f1babb6. Remote Public hidxt/miskoai; no gh executable detected. Go 1.27.0 windows/amd64, Python available, gcc/gitleaks not detected.
- Master requirements captured in REQUIREMENTS.md; no requirement changes approved.
- Root documents initialized before business code. Module github.com/hidxt/miskoai.
- Research agents: gpt-6.1-sol medium for official WeChat source, low for current official cloud docs. Only these two child configurations allowed.
- Official cloud docs confirm DeepSeek native vision and direct hosted Ollama search. Need primary review/source pin notes.
- Branch codex/foundation. Commit c9a432d records the13 initial root documents and planning artifacts before Go implementation. Phase1 code/checkpoint follows that commit; use git log for actual commit/push state.
- Implemented CLI, bounded cloud transport/DeepSeek/Ollama, pinned official WeChat text adapter and pure-Go SQLite FTS5/facts/message states/backup validation. Linux cross-builds succeeded; they are development PoC binaries, not a release.
- Final integrated Windows synthetic tests:63 passed, one Unix permissions check explicitly skipped; vet/govulncheck passed. Independent Medium review's six findings were fixed. Gosec24 warnings individually triaged in docs/research/gosec-disposition.md; full product audit remains pending.
- User explicitly authorized one fixed text/stream/image/search test each, safe status retries at most2 each, total at most12HTTP attempts, plus human QR authorization. Four cloud probes ran once and all passed, with text/vision deepseek-flash. NEVER rerun the cloud batch under this allowance. Temporary credentials now exist privately; never print values. Human QR authorization pending.
- Windows Token ACL and atomic key helper were verified synthetically; user then entered real temporary API keys in visible hidden-input PowerShell, and cloud probes used them privately. Windows race unavailable without gcc. First native Linux CI failed tests; Go1.27 t.TempDir uses0777/umask, so private fixtures now use newly created0700 children; detailed annotations/revised CI pending. VPS is still unsupplied.
- Keep Go caches and test temp under .tools; default Windows sandbox temporary-directory rename produced access denied. Workspace temporary-directory restore integration passes.
- Native deployment remains Go-only. Python, scanners and Windows PowerShell scripts are development tooling, not server requirements.
