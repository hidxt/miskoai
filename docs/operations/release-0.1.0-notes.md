# MiskoAI 0.1.0

超轻量AI微信聊天伙伴：原生Go单体、SQLite、内嵌管理界面；使用已配置的DeepSeek官方模型（默认deepseek-flash）和Ollama云端搜索，无本地模型或常驻前端/解析运行时。

本版本提供持久接收与去重、范围隔离的事实/历史/摘要/档案、文本/视觉/搜索、有界TXT/Markdown/DOCX/PDF提取、静态图片库与可分别确认的文字后附图片回复，以及受登录/CSRF保护的管理、备份恢复与CLI。Go1.27.3构建解决Go1.27.2的GO-2026-6809证书校验安全阻挡。

源码 `dce3c1b480fbe8acb2223a498edbe9baeac7533b`；[精确原生CI](https://github.com/hidxt/miskoai/actions/runs/39604537169)12步成功；本地全量3266PASS/0FAIL/4平台跳过、vet与漏洞扫描通过。Windowsamd64/Linuxamd64/arm64三个包经过模块/架构/完整声明/哈希核验；arm64为交叉构建，未宣称实机运行。先核对SHA256SUMS，再核对解包二进制的BINARY_SHA256SUMS；完整LICENSE和第三方声明随包提供。

已有有限真实文字、静态PNG、受控重连及官方主动断开后的授权恢复证据；自然到期未实测。此前Go1.27.2二进制在可见406.57MiB RAM且配置1GiBSwap的Debian服务器上，60秒空闲峰值12.66MiB、480条主消息/10分钟合成负载峰值19.89MiB、采样Swap0；这不是新Go1.27.3包的重新测量，也不代表真实供应商最坏值或通用512MB保证。

已知限制：Windows较大图片库恢复可能触及现有30秒校验期限；401会话拒绝后存在显示为“取消”的文案问题，认证拒绝及私有内容清理有效；原生GIF动画/微信贴图不支持，采用静态PNG/JPEG形式；实际arm64运行、systemd安装、自然TTL、断电恢复和无限期负载未建立。旧Core race重开失败保留，原因不从后续成功反推。

安装、升级、私有凭据、SSH回环管理、恢复与卸载见源码中的docs/operations/native-linux.md。完整备份不含凭据/授权/settings/systemd文件；恢复后通道暂停，先人工核对，不重试结果不明的发送或恢复。此次交付不自动部署或启动真实聊天服务。发布状态以仓库实际Release为准。
