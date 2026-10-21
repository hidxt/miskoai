# MiskoAI 0.1.0 发布审批范围

2026-10-21，工程进度 **98%（49/50）**。此计划已准备，尚未执行；只剩50公开二进制发布。完整 [验收与七文件摘要](../research/release-0.1.0-verification.md) 和 [拟发布说明](release-0.1.0-notes.md) 可审阅。

拟在现有公开仓库 hidxt/miskoai 创建 **v0.1.0**，严格指向 `dce3c1b480fbe8acb2223a498edbe9baeac7533b`；其 [原生CI](https://github.com/hidxt/miskoai/actions/runs/39604537169)12步全部成功。当前后续文档提交与候选编译输入相同，不能把文档提交误写为二进制构建来源。

批准后的步骤：先只读确认标签/Release状态和7文件哈希；创建一个草稿Release并上传下面7个固定资产；核对服务器返回的名称/大小/SHA256或下载回读后，再发布草稿。不覆盖已有标签/Release/资产，不强推。任一写入结果不明即停止，只读核对已存在状态，不自动重发。若无法安全取得现有合法GitHub授权，则请所有者协助，不在聊天发送Token。

|固定资产|字节|SHA256|
|---|---:|---|
|miskoai-0.1.0-windows-amd64.zip|11678354|`8ce8a0c0ff667fe0156868782e9add8fbdf747548420c3fb0c16b2d5bade468e`|
|miskoai-0.1.0-linux-amd64.tar.gz|11531998|`8f402ecf9f5987a4ac5b2e3591dc28d790f4228219b36a4fe30c1ca0f9c53387`|
|miskoai-0.1.0-linux-arm64.tar.gz|10727025|`81a02483daab0dffb7da5674aee68d7d060f7e83ac10403b605af287e83b821c`|
|miskoai-0.1.0-source.tar.gz|1477543|`31ac07a5ee05f4c129a2dae39d6d5b932441230393214f6e5f373b02237391f8`|
|PROVENANCE.json|3482|`4113835221963f0fe04bffaba7368f732c1f7d1cf4deb09fbaf0c62e1a7f2bc0`|
|BINARY_SHA256SUMS|264|`2c877e7d04941da4011cab00c86a30f0db4115b9fed08e0b2371c5f2c209cdb0`|
|SHA256SUMS|556|`9143b9e8e216cefa43be9611fa1c1a713b562900a371e86111fa2154c35db504`|

范围是标签、发布说明和上述资产的公开发布；安装、systemd启用、真实数据迁移及新的微信/付费/服务器负载不在这次发布动作内。Windows大库恢复30秒限制、原生GIF不支持及arm64实机/自然TTL/资源观测范围均随说明公开保留。最终7文件只含公开源码、程序、声明、来源及摘要，已审查无真实凭据/聊天/媒体。

包内PROVENANCE记录打包时的未发布快照；公开状态以实际GitHubRelease为准。source归档严格保留构建提交的历史文档，不伪造为其后验收记录。源和制品摘要已核验，不宣称可复现构建或数字签名。
