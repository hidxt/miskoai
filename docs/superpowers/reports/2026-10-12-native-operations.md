# 原生运维 Task1 实现记录

2026-10-12。执行者 ONLY `gpt-6.1-sol` / `reasoning_effort medium`，无下级代理。工程进度保持 **72%（36/50）**，本报告只提交 Task1 待主代理独立审查；不自行给予 package41 信用，不宣称最终验收。

## 交付与范围

仅新增三个公开文件：

- `deploy/systemd/miskoai.service`：用户 manager、固定 `%h/.local/bin/miskoai serve`、loopback 8080、必需私有 EnvironmentFile、0077、NoNewPrivileges、SIGTERM/control-group、120s 有限停止超时、on-failure/10s 和 300s/3 次启动窗口。
- `docs/operations/native-linux.md`：中文原生产物验证、私有安装/配置/一次初始化、合成 doctor、用户 manager/lingering、SSH、安全生命周期、备份升级恢复与默认保留私有数据的卸载手册。
- 本报告：实际有限本地证据及未验证范围。

没有修改 Go、根文档、README、工作流、spec、plan、LICENSE、Git index/历史或其他代理文件。所有检查工具和证据仅位于忽略的 `.tools/reviews/native-operations-*` 前缀；没有永久运行时、实际部署或安装脚本。

读过 AGENTS、REQUIREMENTS、SECURITY、ARCHITECTURE、TASKS、MEMORY、DELIVERY，及绑定的 design/plan；依实际 config/settings/credentials、CLI/serve/status、privatefs、maintenance/backup/restore/pause、Web Host/Origin 和认证恢复通道接口核对全部示例。本任务属于纯文档/模板范围，按明确 spec 执行实际现有 CLI 检查，不新增实现镜像源代码测试。使用 executing-plans 与 verification-before-completion，根目录台账/审查/Git 由主代理负责。

## 具体合同核对

| 事项 | 代码或主文档依据 | 文档处理 |
| --- | --- | --- |
| 既有路径/链接 | privatefs.Resolve、ancestors、CheckDir/CheckFile、O_EXCL | 各层真实目录、身份/权限检查，拒绝异常/链接/既有未知目标；仅创建明确最后一层目录；复制新文件使用 noclobber，不递归修复 |
| 凭据/优先级 | config.Load、loadKeys、strictObject | 0600/0700、单硬链接；本机编辑；环境文件不作为 shell 执行；默认→settings→env；供应商 env 优先于文件 |
| 管理密码 | CLI serve、web.New | 16–256 UTF-8 字节/无控制字符；占位密码不能投入真实启动；config validate 不证明 serve 密码门槛 |
| 固定 loopback | unit Environment、config.Listen、Web exact Host/Origin | unit 和手册 127.0.0.1:8080；EnvironmentFile 只保留三个约定字段；其覆盖 Environment 的性质明确说明；隧道两端相同端口 |
| 登录生命周期 | systemd.unit、loginctl | enable 与 start 分开；登出后运行需管理员批准 lingering；不实际执行、不提供第二套系统级模板 |
| 停止/重启 | cmd NotifyContext、CLI joined Run、systemd.service/kill | 120s 后可能强杀；source join 与 Linux 实际 graceful 证据分开；窗口限次不是终身总次数限制 |
| 升级/一致性备份 | StoppedBackup、Acquire、Store.BackupOwned | 升级前旧二进制/stop/独占锁；新备份目标；单独保存配置、授权、旧二进制及独立文件范围；不能复制 live WAL |
| schema 回退/恢复 | PrepareRestore、InstallRestore、ensurePause | 二进制回退不能撤迁移；restore 可升级候选 schema；保留 pre-restore、暂停标记；人工对账后在认证 UI 确认，不盲删标记/锁/journal/重发 |
| 卸载 | 已记录身份/摘要、已 stop 的运行边界 | 只删除明确核实的 unit/二进制；默认保留数据/配置/备份；私有删除须另一次 owner 决定，无递归删除配方 |

自查更正：Web 要求精确 Host 与 Origin，不能在本地改 SSH 隧道端口而服务器仍用 8080。最终手册拒绝这一不兼容路径，不改安全策略。已有认证管理 UI 的“人工核对后恢复通道”是恢复暂停的审查路径；不存在对应自动 CLI 快捷命令。

## 实际 Windows 检查

主代理在并行的后续库开发开始前指定只使用已接受不可变源码 `.tools/reviews/core-document-fixture-fix1-reviewed-src`，其 Go/module/workflow 对应已发布 `08814e0`。本任务没有从正在变化的工作树编译。源码各文件完整 SHA256/字节清单位于 `native-operations-basis-manifest.json`；主代理负责再次验证其与已接受公开源码的身份。

执行入口为 `.tools/reviews/native-operations-cli-checks.ps1`，原始完整脚本保留。正常文件所有者上下文的 `require_escalated` 执行用于保留现有 Windows owner-only ACL 检查，未放宽 ACL。该进程只按名称清除继承 `MISKOAI_*` / DEEPSEEK_API_KEY / OLLAMA_API_KEY，不输出或保存原值；设置固定合成数据路径/loopback/合成密码，没有真实凭据文件。离线已有 Go1.27.2 工具链，GOTOOLCHAIN=local、GOPROXY=off、GOSUMDB=off、CGO_ENABLED=0、GOOS=windows、GOARCH=amd64，已有 module/toolchain cache，无下载安装。

实际 Go 可执行路径为 `.tools/toolchains/go1.27.2-verified/go/bin/go.exe`；工作目录为上述已接受冻结源码；实际构建参数 `build -trimpath -o C:\Users\auzasr\Documents\Projects\miskoai\.tools\reviews\native-operations-cli\miskoai.exe ./cmd/miskoai`。之后对该构建执行以下有限命令：

| 实际命令/参数 | 退出码 | 结果/原始证据 |
| --- | --- | --- |
| Go build，参数见上 | 0 | `native-operations-cli/build.txt`，无诊断 |
| `miskoai.exe version` | 0 | `MiskoAI 0.1.0-dev`；version.txt |
| `miskoai.exe licenses` | 0 | 完整嵌入声明输出；licenses.txt；与已接受 notices.txt 文本比较单独记录 |
| `miskoai.exe init` | 0 | initialized private settings; environment overrides saved values；init.txt |
| `miskoai.exe config validate` | 0 | configuration valid; live provider/account readiness is separate；config-validate.txt |
| `miskoai.exe doctor` | 0 | go1.27.2 windows/amd64；SQLite migrations/FTS5/integrity passed，明确 synthetic local database；doctor.txt |

原始 `commands.json` 保存六项实际退出码；最终 exec 进程退出 0。doctor 的供应商 configured 标志均 false，明确输出 Live API, WeChat and Linux 512MB validation not performed。init 只创建独立合成 settings，没有业务数据库；doctor 的临时 SQLite 自行关闭/清理。没有执行 serve、status、weixin login、poc、provider/CDN/账号探测；没有读取原始私有环境值、实际 settings/授权/媒体/DB，实际私有 schema2 未动。

构建文件 19,785,216 字节，SHA256 `bdd4722471b666870fe00a4e0dc92ba11cab1a5dc941808e37bc2c303652af6c`。此 Windows 开发构建不是 Linux 发布产物；不能将它作为目标 Linux 安装文件。完整文件与证据身份以忽略目录内的 manifest 为准，主代理重新读取/比对才是集成证据。

## 上游人工核对与限制

2026-10-12 通过浏览工具检查 systemd 项目自己的文档：[service](https://github.com/systemd/systemd/blob/main/man/systemd.service.xml)、[exec](https://github.com/systemd/systemd/blob/main/man/systemd.exec.xml)、[kill](https://github.com/systemd/systemd/blob/main/man/systemd.kill.xml)、[unit](https://github.com/systemd/systemd/blob/main/man/systemd.unit.xml)、[loginctl](https://github.com/systemd/systemd/blob/main/man/loginctl.xml)。其中 exec/unit/loginctl 的 raw 源也用于精确字段核对。freedesktop 托管手册访问返回 403，改用官方源码文档，没有伪造目标版本验证结果。

核对 Type=simple、EnvironmentFile 必需文件/覆盖规则、UMask、NoNewPrivileges、restart/start window、kill control-group/超时强杀、%h 与用户 unit 路径、WantedBy 和 lingering 含义。记录依据为上游当前文档；主机实际版本兼容性不能由它推出。没有使用用户 manager 下未经验证的 mount namespace 指令，没有增加 MemoryMax/CPU 配额，没有更换 provider/SQLite/channel/安全/512MB 策略。

本地检查不包括 Bash 脚本实际 Linux 执行、`systemd-analyze --user verify`、用户 manager 启动/关闭/断登/开机、服务器部署、真正 graceful join、Linux/arm64 实机、512MB idle/chat/sustained RSS、实时供应商/微信/文件/网络测试或最终发布接受。现有 native CI 的状态与本任务单独区分；本任务未创建或声称新的 CI 结果。真实微信已延期，有限付费 allowance 已用完。未提供服务器即保持待验证，不通过容器/WSL替代。

## 冻结、交接与静止

三个公开文件的逐字冻结副本和完整检查证据复制到 `.tools/reviews/native-operations-frozen/`；其 SHA256/bytes 分别记录于 `native-operations-owned-manifest.json` 与 `native-operations-evidence-manifest.json`。没有覆盖或清理历史 runner/evidence，也未 slice/eval 旧执行器；所有 setup 来自本任务新脚本。

作者自查已读完整模板/手册/本报告、全部已执行输出/退出码与 licenses 文本身份比较；没有原生 systemd 成功断言。主代理接手 fresh Medium review、完整集成检查、公开 secret/license 扫描、根台账、package41 进度决定和 Git。有限执行已结束，无运行的本任务进程或服务；冻结后停止写入，等待独立审查。

## FIX1：合成占位字符串的公开策略校正

2026-10-12。主代理原始公开策略检查实际退出1：339个 git-visible 文件，手册第113行报 credential-value，原因是大写合成密码占位字符串未匹配策略已有的大小写敏感 synthetic 前缀豁免。FIX1 只把该示例赋值改为小写 `synthetic-only-replace-in-local-editor`，仍必须在本机私有编辑器替换后才能启动；模板、配置接口、scanner、规则和 allowlist 未改。新增本报告校正记录，除此之外不改其他公开内容。

原始三个公开文件冻结、11项检查证据及各原 manifest 原样保留，不覆盖。FIX1 新身份/复制清单位于 `.tools/reviews/native-operations-fix1-owned-manifest.json` 与 `native-operations-fix1-evidence-manifest.json`，新副本位于 `native-operations-fix1-frozen/`。逆向恢复唯一占位字符串后，手册须与原冻结逐字相等；报告去掉本追加段后须与原冻结逐字相等，这项证明单独保存。

修正后对 git-visible、非忽略公开文件执行原 `scripts/secret_scan.py`，实际退出0；完整输出保存为 `native-operations-fix1-secret-policy.txt`，文件数和退出码保存为新证据。未扫描忽略的真实私有目录。六项原始构建/CLI退出0仍是先前有限Windows证据，未重复运行，不能把本次文本修正当作新的运行测试或 native 验证。最新应用 native CI 失败的原因仍由主代理独立调查，本修正不宣称它已通过。

FIX1 不改变工程72%或 Linux/systemd/部署/资源/live/final 未验证边界；完成复制及身份核对后再次静止，交主代理进行 fresh review。
