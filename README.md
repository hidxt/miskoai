# MiskoAI

超轻量 AI 微信聊天伙伴：Go 模块化单体、SQLite、DeepSeek 官方云模型与 Ollama 云端搜索，管理页面随单个可执行文件内嵌。目标平台为原生 Linux amd64 / arm64、2 vCPU / 512MB。

项目进度 **96%（48/50）**，仍在开发，尚未发布生产版本。剩余 **2 项**见 [任务清单](TASKS.md)；本地实现、原生 CI、真实微信与服务器资源验收分别记录在 [交付状态](DELIVERY.md)。已完成获准的原生服务器空闲与10分钟合成持续负载验收；可见内存与Swap边界单独报告。

## 当前能力与验证

- 有界文本、流式响应、视觉输入与按需搜索；默认模型按项目所有者选择使用 `deepseek-flash`。
- 持久化接收、去重与一次发送；范围隔离的事实、历史、候选、摘要和表达档案。
- 有界 TXT、Markdown、DOCX、PDF 提取；私人媒体校验、本机图片库、数据库快照与完整图片备份恢复。
- 登录保护的本机管理页面、CLI、原生 Linux 运维和用户级 systemd 模板。
- 真实微信固定文字与静态 PNG 测试已通过独立服务端确认、持久保存和所有者收取确认。自动静态 PNG/JPEG 回复已实现并通过本地整合检查；有限真实重连和实际授权失效后重新扫码恢复已通过；重新签发机器人标识时由所有者批准，旧范围数据保留、不混用。真实随机选择/频率等完整运行边界仍见交付记录。GIF 动画与原生微信贴图不宣称支持；按原要求使用已验证的静态图片形式。详见 [图片实测](docs/research/weixin-static-image-live-verification.md)。

0.1.0版本元信息正在配套构建/CI核对；尚未公开发布。

上一次已验收源码 [d9bc732](https://github.com/hidxt/miskoai/commit/d9bc7320ba6f78350fe9d622e35e5e49d28b8f2c) 的 [原生 CI39405918311](https://github.com/hidxt/miskoai/actions/runs/39405918311) 已完成12步成功，包含单元/整合、vet、原生Linux race、依赖漏洞、完整历史秘密检查和双架构构建。集中生产源码审查、保留告警核对与公开内容扫描已通过其验收门槛；制品和有限资源门槛已验收，原生Linux35图浏览器完整恢复已验收；交付记录和公开发布仍有独立项目。Windows较大库恢复的30秒限制保留，详见 [目标环境恢复验证](docs/research/native-browser-recovery-verification.md)。此前5a94038的Core重开失败保留，不从后来成功反推其原因。新一轮Debian测量：产品空闲60秒峰值12.66MiB，480条主消息/10分钟合成Core负载峰值19.89MiB，采样Swap为0；主机可见RAM406.57MiB、配置1GiBSwap，供应商/通道为模拟，详见 [资源验收](docs/research/server-resource-acceptance.md)。交叉构建不代表arm64运行。

## 部署与本机管理

运行时仅需一个 `miskoai` 进程、其私有配置/数据目录、原生 Linux 与可信 TLS CA。无需容器、WSL、本地模型、Redis、外部数据库或常驻 Node/Python/Java。管理监听默认 `127.0.0.1`；systemd 模板固定为 `127.0.0.1:8080`，远程管理使用两端同端口的 `127.0.0.1` SSH 隧道，以满足 Host/Origin 校验。安装、升级、恢复、卸载及 systemd 操作详见 [原生 Linux 运维手册](docs/operations/native-linux.md)。

开发 CLI 支持：

```text
miskoai help
miskoai version
miskoai licenses
miskoai init
miskoai config validate
miskoai doctor
miskoai serve
miskoai status
miskoai backup PATH
miskoai restore PATH
miskoai backup --library PATH
miskoai restore --library PATH
miskoai weixin login
```

无参数调用也显示帮助。另有显式开发探测入口 `miskoai poc deepseek|stream|search|vision PATH|weixin`，仅在相应真实测试获准时使用。`doctor` 使用本机合成 SQLite 数据，只报告凭证是否存在，不调用云端或微信。`serve` 先绑定管理页面，再启动已配置的通道工作；退出时等待实际工作结束后关闭存储。`status` 仅连接配置的本机监听，不打开数据库；Go 堆内存与逻辑尝试数有明确标注，当前不提供进程 RSS 验收结果。

`backup/restore --library` 是最多321MiB的完整数据库与图片包，要求有效管理配置且服务已停止；完整恢复在校验前停止当前代际，失败时可能保持停止。普通数据库快照只含图片元数据，`restore PATH` 输入上限为256MiB。两类备份均不自动包含 `settings.json`、`weixin-auth.json`、环境/凭证文件或 systemd unit。恢复可能回退接收/发送记录，完成后通道保持暂停，必须通过已认证管理页面人工核对后恢复。失败或结果不明确时应先核对状态，不要重复提交。现有私人数据库的迁移、恢复和启动需先核对备份及授权。

## 配置与凭证

`init` 独占创建私有、非秘密的 `settings.json`，要求已有父目录；不会覆盖现有文件或修复无关目录权限。显式存在的环境变量覆盖保存值，空值或无效覆盖会拒绝启动。Go 不会自动加载 `.env`，安全示例见 [.env.example](.env.example)。

API Key 可由环境变量（优先）或显式指定的 `MISKOAI_CREDENTIALS_FILE` 提供；凭证文件须符合精确键名、最多8KiB和所有者私有权限要求，不会自动发现。管理密码仅由 `MISKOAI_ADMIN_PASSWORD` 环境变量提供，要求16–256字节的有效 UTF-8、非空白且无控制字符。不要将密钥、令牌、真实聊天/媒体、数据库及其日志文件、会话或备份放入 Git，也不要在 CLI 参数中传递秘密。

项目先前获准的一组有限真实云端测试已执行完毕；新的付费或微信测试需按具体范围授权。`scripts/Set-TestCredentials.ps1` 采用隐藏输入和私有文件；`scripts/Invoke-LiveProbes.ps1` 是显式测试工具，不能把历史授权视为新的调用额度。二维码和授权输出仅在私人交互终端使用。

## 构建与开发检查

模块为 `github.com/hidxt/miskoai`，开发工具链基线 Go1.27.2。从原生 Unix 开发 shell 执行：

```sh
go test ./... -count=1
go vet ./...
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o dist/miskoai-linux-amd64 ./cmd/miskoai
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o dist/miskoai-linux-arm64 ./cmd/miskoai
```

Windows 工作区的 `./scripts/Test.ps1` 设置项目内 Go 缓存/临时目录并运行合成测试和 vet；所选工具链须满足 `go.mod`。这些是开发工具，目标服务器不需要 Python/PowerShell 守护进程。源码与候选二进制的完整许可证已核对；安全/声明、开发候选制品及有限资源门槛已验收；目标Linux功能/恢复已验收；0.1.0交付记录和发布尚未完成，证据分类见 [TESTING.md](TESTING.md)。

## 项目记录与许可证

[需求](REQUIREMENTS.md) · [架构](ARCHITECTURE.md) · [安全策略](SECURITY.md) · [当前交付](DELIVERY.md) · [任务](TASKS.md) · [开发状态](MEMORY.md)。Apache-2.0，见 [LICENSE](LICENSE)；第三方清单见 [依赖许可证](docs/research/dependency-licenses.md)，完整嵌入声明可用 `miskoai licenses` 查看。原 README 的全部历史内容按原字节保存在 [历史记录](docs/superpowers/history/2026-10-19-README.md)。
