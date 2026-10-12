# MiskoAI 原生 Linux 运维手册

本手册交付单个 Go 可执行文件和用户级 systemd 模板。运行时只有一个 MiskoAI 进程、SQLite 和内嵌管理页面；无需 Go 编译器、Node、Python、Java、Redis、容器、WSL 或本地模型。适用目标为原生 Linux amd64 / arm64。开发版本和 CI 交叉构建产物仅用于开发验证，最终发布产物、实际 arm64 运行和 2 vCPU / 512MB 资源验收仍是独立门槛。本次文档验证没有安装或启动真实服务，没有访问私人账号或数据库。

## 1. 产物、架构与安装路径

先由操作者取得可信渠道提供的原生二进制、对应源码版本、LICENSE / THIRD_PARTY_NOTICES 和可信 SHA256 清单。不要下载后直接执行，也不要使用下载即运行的安装脚本。没有可信发布清单时，自行计算的摘要只能标识文件，不能证明来源或完成发布验收。

```bash
uname -m
sha256sum /absolute/path/to/supplied/miskoai
```

`x86_64` 对应 Linux amd64，`aarch64` 对应 Linux arm64；其他架构不在本模板范围。把摘要逐字与独立取得的可信清单中该架构、该版本的条目比对，并保留版本和摘要记录。以下命令使用已有的 Bash 与常规 Linux 文件工具，所有路径变量必须改为本机已核实的绝对路径；不要把示例占位值当作真实配置。

| 对象 | 固定安装位置 | 要求 |
| --- | --- | --- |
| 二进制 | `~/.local/bin/miskoai` | 当前运行用户拥有的普通文件，示例模式 0700 |
| 数据 | `~/.local/share/miskoai` | 0700；包含 settings、数据库、账号授权和恢复状态 |
| 私有配置 | `~/.config/miskoai` | 0700 |
| systemd 环境文件 | `~/.config/miskoai/miskoai.env` | 0600；不能提交到仓库或发到聊天中 |
| 供应商凭据 | `~/.config/miskoai/provider.json` | 0600、单硬链接、运行用户拥有 |
| 用户 unit | `~/.config/systemd/user/miskoai.service` | 本仓库模板；示例模式 0600 |
| 私有备份 | `~/.local/share/miskoai-backups` | 0700；新备份文件 0600，不覆盖旧备份 |

不用 root 运行 MiskoAI。执行命令的用户、文件所有者和 `systemctl --user` 的用户必须一致；不要用 `sudo systemctl --user` 安装到另一用户的 manager。管理员若需要系统级部署，应另行审查其政策，本包只提供用户级模板。

## 2. 检查既有对象并仅创建明确目录

先检查 HOME 和每一层祖先：任何路径组件是符号链接、目标异常、由其他用户控制或现有安装来源不明，都停止并由操作者调查。应用 privatefs 会拒绝符号链接路径、非普通文件、非本用户所有的私有对象、组/其他用户可访问的私有对象和有多个硬链接的私有文件；不会替你修复权限。不要对既有树递归 chmod/chown、移动或删除。

以下辅助函数只创建列出的最后一层目录，不跟随符号链接、不覆盖文件。检查假设没有恶意同用户进程并发替换路径；若其他进程会操作安装目录，先协调其停止。既有私有目录权限不符时函数拒绝，不能当作自动修复器。

```bash
set -euo pipefail
umask 077
test -d "$HOME" && test ! -L "$HOME"
test "$(realpath -e -- "$HOME")" = "$HOME"
test "$(stat -c %u -- "$HOME")" = "$(id -u)"
home_mode="$(stat -c %a -- "$HOME")"
(( (8#$home_mode & 0022) == 0 ))

ensure_dir() {
    local p="$1"
    if test -e "$p" || test -L "$p"; then
        test ! -L "$p" && test -d "$p"
        test "$(realpath -e -- "$p")" = "$p"
        test "$(stat -c %u -- "$p")" = "$(id -u)"
        local mode="$(stat -c %a -- "$p")"
        (( (8#$mode & 0022) == 0 ))
    else
        test "$(realpath -e -- "$(dirname -- "$p")")" = "$(dirname -- "$p")"
        mkdir -m 700 -- "$p"
    fi
}
private_dir() {
    ensure_dir "$1"
    test "$(stat -c %a -- "$1")" = 700
}
check_regular() {
    test ! -L "$1" && test -f "$1"
    test "$(realpath -e -- "$1")" = "$1"
    test "$(stat -c %u -- "$1")" = "$(id -u)"
    test "$(stat -c %h -- "$1")" = 1
}
copy_new_regular() {
    check_regular "$1"
    test ! -e "$2" && test ! -L "$2"
    test "$(realpath -e -- "$(dirname -- "$2")")" = "$(dirname -- "$2")"
    ( set -o noclobber; cat -- "$1" > "$2" )
    check_regular "$2"
    test "$(sha256sum -- "$1" | cut -d ' ' -f 1)" = \
         "$(sha256sum -- "$2" | cut -d ' ' -f 1)"
}
ensure_dir "$HOME/.local"
ensure_dir "$HOME/.local/bin"
ensure_dir "$HOME/.local/share"
ensure_dir "$HOME/.config"
ensure_dir "$HOME/.config/systemd"
ensure_dir "$HOME/.config/systemd/user"
private_dir "$HOME/.local/share/miskoai"
private_dir "$HOME/.config/miskoai"
private_dir "$HOME/.local/share/miskoai-backups"
```

这段检查不是安装授权，也不会识别来源不明的现有内容。已有 `miskoai` 二进制、unit、配置或数据时，先确认所属安装和备份，再进入升级流程；不要重复首次安装。如果命令失败，停止该段后续命令，不要以忽略退出码的方式继续。

首次安装确认目标不存在后，从已比对摘要的本用户拥有的普通产物复制（解包后的产物路径也不能经过符号链接）：

```bash
artifact=/absolute/path/to/supplied/miskoai
unit_source=/absolute/path/to/reviewed/source/deploy/systemd/miskoai.service
check_regular "$artifact"
check_regular "$unit_source"
copy_new_regular "$artifact" "$HOME/.local/bin/miskoai"
chmod 700 -- "$HOME/.local/bin/miskoai"
copy_new_regular "$unit_source" "$HOME/.config/systemd/user/miskoai.service"
"$HOME/.local/bin/miskoai" version
"$HOME/.local/bin/miskoai" licenses
```

最后两个命令分别显示版本和随二进制嵌入的完整第三方声明；同时保留源码 LICENSE / THIRD_PARTY_NOTICES，不能仅凭版本号认定文件来自可信构建。

## 3. 私有配置与一次性初始化

在本机私有编辑器中**新建** `~/.config/miskoai/miskoai.env`，保存为运行用户拥有的 0600 普通文件，父目录保持 0700。先确认目标不存在；若已存在，就核对来源和身份后编辑既有文件，不能用重定向覆盖未知对象。关闭编辑器云同步、协作分享及不受保护的自动备份。只在编辑器里替换密码；不要在 shell 参数、历史记录、日志或 unit 中写真实密码。

文件内容示意如下；路径中的 `REPLACE_LOCAL_USER` 必须换成本机用户的实际 HOME 路径，密码必须换为自己的随机密码后才可启动：

```ini
MISKOAI_DATA_DIR=/home/REPLACE_LOCAL_USER/.local/share/miskoai
MISKOAI_CREDENTIALS_FILE=/home/REPLACE_LOCAL_USER/.config/miskoai/provider.json
MISKOAI_ADMIN_PASSWORD=synthetic-only-replace-in-local-editor
```

这里是 systemd EnvironmentFile 格式，不能 `source`/`.` 执行。它不展开 `$HOME`、`~` 或 `%h`；空格路径/特殊字符须按目标主机 `systemd.exec` 的引用规则在编辑器中正确引用。模板使用 `%h` 的地方由 systemd 展开。上述简单示例适用于不含空格的 HOME。管理密码需要有效 UTF-8、16–256 **字节**、非空白且无控制字符；优先使用随机 ASCII 密码。`config validate` 不证明密码达到 `serve` 门槛，也不联系供应商验证密钥。

同样在私有编辑器中创建 0600 的 `provider.json`，使用既有接口：

```json
{
  "DEEPSEEK_API_KEY": "REPLACE_IN_LOCAL_PRIVATE_EDITOR",
  "OLLAMA_API_KEY": "REPLACE_IN_LOCAL_PRIVATE_EDITOR"
}
```

这是格式示例，不能拿占位字符串发请求。当前凭据文件严格只接受这两个字段，两个值都须为有效非空文本、无控制字符、每个不超过 1024 字节；整个文件不超过 8KiB。未知/重复字段、null、不安全权限或链接都会拒绝。该文件不含管理密码和微信授权。真正的账号授权是数据目录内独立的私有 `weixin-auth.json`；不要从聊天消息手写或复制凭据。真实微信登录/收发须另行由操作者明确授权，本手册不以启动服务代替微信协议验收。

仅对操作者已经新建并确认身份的这两个文件，检查所有权、普通文件、单硬链接和 0600 权限；编辑器保存后再次检查。`stat` 只查看元数据，不输出内容：

```bash
check_regular "$HOME/.config/miskoai/miskoai.env"
check_regular "$HOME/.config/miskoai/provider.json"
test "$(stat -c %a -- "$HOME/.config/miskoai/miskoai.env")" = 600
test "$(stat -c %a -- "$HOME/.config/miskoai/provider.json")" = 600
```

设置优先级为默认设置 → 数据目录 `settings.json` → `MISKOAI_*` 环境覆盖；供应商 `DEEPSEEK_API_KEY` / `OLLAMA_API_KEY` 环境值优先于凭据文件。模型默认保持 `deepseek-flash`。unit 固定设置 `MISKOAI_LISTEN=127.0.0.1:8080`，环境文件只填写上述三个字段，不能加入 LISTEN 或其他设置覆盖；systemd EnvironmentFile 的赋值会覆盖 Environment，故编辑者必须保持这个约定。不要把 manager 继承的旧配置误当新配置；在本机检查时只枚举变量**名称**，不要显示其值。启动前清除 manager 中遗留的 `MISKOAI_*` 和两个供应商变量，或由管理员提供已确认干净的用户 manager；不要打印 `systemctl show-environment`。

在确认该用户 manager 的这些变量没有被其他服务依赖后，可用下列**仅名称**命令清除当前版本识别的配置变量；它改变该用户 manager 的环境，不改变已经运行的进程，也不替其他服务作配置决定。若其他服务依赖这些变量，先由管理员协调而不执行这段：

```bash
systemctl --user unset-environment \
    MISKOAI_DATA_DIR MISKOAI_LISTEN MISKOAI_DEEPSEEK_URL MISKOAI_MODEL \
    MISKOAI_VISION_MODEL MISKOAI_WEIXIN_URL MISKOAI_MAX_OUTPUT_TOKENS \
    MISKOAI_CONTEXT_BYTES MISKOAI_CONTEXT_TOKENS MISKOAI_CREDENTIALS_FILE \
    MISKOAI_ADMIN_PASSWORD DEEPSEEK_API_KEY OLLAMA_API_KEY
```

CLI 的本地检查可在独立子 shell 内清除继承名称，不读取环境文件。以下示例明确设置路径与 loopback，没有真实密码参数；只在确认 `settings.json` 不存在时执行 `init` 一次：

```bash
(
    for name in ${!MISKOAI_@}; do unset "$name"; done
    unset DEEPSEEK_API_KEY OLLAMA_API_KEY
    export MISKOAI_DATA_DIR="$HOME/.local/share/miskoai"
    export MISKOAI_LISTEN=127.0.0.1:8080
    export MISKOAI_CREDENTIALS_FILE="$HOME/.config/miskoai/provider.json"
    test ! -e "$MISKOAI_DATA_DIR/settings.json"
    test ! -L "$MISKOAI_DATA_DIR/settings.json"
    "$HOME/.local/bin/miskoai" init
    "$HOME/.local/bin/miskoai" config validate
)
```

`init` 只创建 settings，不创建业务数据库、不验证账号、不会覆盖既有 settings；它保存当时生效的设置（含这里的 8080）。已有安装跳过 `init`，只做验证；不要因“already exists”而删掉 settings 重试。

正式启动前，另建一个全新、真实祖先路径下的临时**合成**目录，用于有限 doctor 检查，避免加载既有私有设置/凭据。doctor 自己创建、关闭并清理临时合成 SQLite 数据库，不打开业务 DB、不请求 API。它通过并不意味着目标数据完整或服务已准备就绪：

```bash
(
    for name in ${!MISKOAI_@}; do unset "$name"; done
    unset DEEPSEEK_API_KEY OLLAMA_API_KEY
    scratch="$(mktemp -d "$HOME/.local/share/miskoai-doctor.XXXXXXXX")"
    test "$(realpath -e -- "$scratch")" = "$scratch"
    export MISKOAI_DATA_DIR="$scratch/private"
    export MISKOAI_LISTEN=127.0.0.1:8080
    "$HOME/.local/bin/miskoai" config validate
    "$HOME/.local/bin/miskoai" doctor
    rmdir -- "$scratch"
)
```

`config validate` 在空合成路径只检验配置合同；正式目录还须单独验证。该 `rmdir` 只删除刚创建且仍为空的精确临时目录；失败时保留调查，不递归删除。

## 4. 安装 unit、兼容性与用户 manager 生命周期

先在**实际原生 Linux 目标机**执行以下只验证/装载命令，记录版本、退出码和诊断，不忽略 unknown directive 或权限错误：

```bash
systemd --version
systemd-analyze --user verify "$HOME/.config/systemd/user/miskoai.service"
systemctl --user daemon-reload
```

截至 2026-10-12，本任务没有原生 Linux 主机，未执行上述命令。上游手册逐项核对不能替代目标机语法检查；语法检查也不能证明实际启动、网络就绪或关闭正确。

模板的 Type=simple 以单个前台进程运行；manager 认为启动成功不等于 HTTP 已绑定或账号可用。UMask=0077 限制新建文件，不能修复已有权限；NoNewPrivileges 防止经 exec 获得新权限。EnvironmentFile 是必需文件，没有忽略缺失的 `-` 前缀。没有用户 manager 下不可靠的挂载命名空间指令，也未设置未经测量的 MemoryMax / CPU 限制。[systemd.service 上游](https://github.com/systemd/systemd/blob/main/man/systemd.service.xml)、[执行环境上游](https://github.com/systemd/systemd/blob/main/man/systemd.exec.xml)。

Restart=on-failure 在异常失败后等待 10 秒重启；300 秒窗口最多允许 3 次启动尝试，超限后需排查再显式 reset-failed。它不是全局终身重试次数限制，正常退出和人工 stop 不触发该策略。KillSignal=SIGTERM 先通知整个 control-group，TimeoutStopSec=120s 过后仍未退出时 systemd 可强制终止。Go 源码会取消并等待 Web/Core 的工作返回后关闭存储，但超时后的强制结束不能声称优雅清理成功；遗留锁/日志须人工核实。[终止策略上游](https://github.com/systemd/systemd/blob/main/man/systemd.kill.xml)、[启动限额和用户 unit 路径上游](https://github.com/systemd/systemd/blob/main/man/systemd.unit.xml)。

用户 manager 的生命周期通常受登录会话及主机政策影响。`enable` 只是挂接 `default.target`，既不立即启动服务，也不保证退出登录后继续运行。需要登出后常驻或开机启动用户 manager 时，由管理员明确批准该用户的 lingering 政策，再由管理员执行 `loginctl enable-linger USER`；这不是本手册自动执行步骤。管理员可使用 `loginctl show-user USER -p Linger` 查看该单项状态。主机没有用户 manager 或 lingering 不被允许时，保留此限制，不悄悄切换系统级服务。[loginctl 上游](https://github.com/systemd/systemd/blob/main/man/loginctl.xml)。

enable 会创建指向这个已核实普通 unit 的 systemd 管理链接；它与来源不明的安装输入符号链接不同。启用前核对既有同名 unit、drop-in 和 wants 链接，冲突时停止，不使用 `--force` 覆盖他人的部署。

## 5. 明确准备就绪后才启动与管理

先完成产物验证、私有文件检查、密码替换、正式配置验证，并确认本次启动允许处理私人账号流量。`serve` 会打开并可能迁移业务数据库；若有效授权和供应商配置已存在，可能立即轮询/处理消息及调用远程供应商。本文的开发验证未执行这些动作。

操作者明确准备就绪后，才执行：

```bash
systemctl --user enable miskoai.service
systemctl --user start miskoai.service
systemctl --user show miskoai.service -p ActiveState -p SubState -p Result -p MainPID
```

下面的停止会等待 manager 完成停止作业；随后核对结果、主进程和是否发生 timeout / 强杀。不要只看 inactive 就推断所有应用清理已成功。

```bash
systemctl --user stop miskoai.service
systemctl --user show miskoai.service -p ActiveState -p SubState -p Result -p MainPID
```

故障原因已排除且允许重新处理账号流量后，可以 `systemctl --user reset-failed miskoai.service` 再 `start`；配置变更用私有编辑器保存并检查权限后显式 `restart`，unit 变更先 `daemon-reload`。两者都会造成停止/启动边界，不能无意识连续执行。

应用 `status` 会用管理密码登录本机 loopback、读取有界状态摘要，不直接打开数据库。可在服务器终端通过静默输入取得密码，只在这个子 shell 的进程环境中使用；不要开 shell xtrace，也不要把这段保存为带真实密码的脚本：

```bash
(
    set +x
    for name in ${!MISKOAI_@}; do unset "$name"; done
    unset DEEPSEEK_API_KEY OLLAMA_API_KEY
    export MISKOAI_DATA_DIR="$HOME/.local/share/miskoai"
    export MISKOAI_LISTEN=127.0.0.1:8080
    export MISKOAI_CREDENTIALS_FILE="$HOME/.config/miskoai/provider.json"
    read -r -s -p 'MiskoAI admin password: ' MISKOAI_ADMIN_PASSWORD
    printf '\n'
    export MISKOAI_ADMIN_PASSWORD
    "$HOME/.local/bin/miskoai" status
)
```

环境式密码是现有产品接口，应把 OS 同用户进程也作为信任边界；权限不能防御恶意同用户进程。停止时 status 可返回不可达；不要按错误提示盲目手工 `serve`，以免绕开 manager 或重复占用数据锁。

远程管理沿用 SSH loopback 隧道，在**本地电脑**运行（替换用户/主机，不包含凭据）：

```bash
ssh -N -o ExitOnForwardFailure=yes -L 127.0.0.1:8080:127.0.0.1:8080 USER@HOST
```

保持 SSH 会话，在本地浏览器打开 `http://127.0.0.1:8080`，使用管理密码登录。应用严格校验 Host 和 Origin 为配置的 `127.0.0.1:8080`；隧道两端端口必须保持一致，不能把本地端口改成 18080 后仍连接服务器 8080，也不能用 localhost 替代地址。如果本地 8080 被其他程序占用，先由操作者协调释放该端口，隧道失败就停止访问，不绕过 Host/Origin 检查。不要把应用监听改为 0.0.0.0，也不要通过防火墙公开管理端口。SSH 连接与访问目标服务器需要操作者已有授权。

## 6. 停止后备份、升级与恢复

备份仅覆盖 SQLite 数据库。另行保留私有 `settings.json`、`weixin-auth.json`（若存在）、恢复暂停/恢复状态、环境文件、供应商凭据、unit 和旧二进制，及未来独立文件库。数据库备份不会自动包含这些文件，保留凭据也不意味着可以重新播放消息。

先 stop 并核实停止结果、MainPID=0、没有另一个手工进程持有数据目录，再以同一运行用户执行以下命令。每次选择**新的**私有备份文件名；下列时间戳目录必须不存在，重复或失败就保留原物并另选名称：

```bash
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
snapshot="$HOME/.local/share/miskoai-backups/$stamp"
test ! -e "$snapshot" && test ! -L "$snapshot"
mkdir -m 700 -- "$snapshot"
(
    for name in ${!MISKOAI_@}; do unset "$name"; done
    unset DEEPSEEK_API_KEY OLLAMA_API_KEY
    export MISKOAI_DATA_DIR="$HOME/.local/share/miskoai"
    export MISKOAI_LISTEN=127.0.0.1:8080
    export MISKOAI_CREDENTIALS_FILE="$HOME/.config/miskoai/provider.json"
    "$HOME/.local/bin/miskoai" backup "$snapshot/database.db"
)
copy_new_regular "$HOME/.local/share/miskoai/settings.json" "$snapshot/settings.json"
copy_new_regular "$HOME/.config/miskoai/miskoai.env" "$snapshot/miskoai.env"
copy_new_regular "$HOME/.config/miskoai/provider.json" "$snapshot/provider.json"
copy_new_regular "$HOME/.config/systemd/user/miskoai.service" "$snapshot/miskoai.service"
copy_new_regular "$HOME/.local/bin/miskoai" "$snapshot/miskoai.previous"
```

仅存在且已经检查为安全普通文件的 `weixin-auth.json` 和其他独立私有文件，才另外用 `copy_new_regular` 复制到该新目录；不得把缺失授权误报为有备份。库文件若尚未交付，就记录该范围，不猜文件布局。不要复制运行中的 DB/WAL/SHM 作为备份。CLI backup 获取独占 `.miskoai.lock`、使用 SQLite 一致性快照并验证备份；它可能在打开 DB 时应用当前二进制支持的迁移。因此应使用**升级前二进制**做备份，不把备份当无副作用文件复制。无业务数据库时不制造“备份成功”。

升级按以下顺序执行，每步失败都停止并保留所有证据：

1. 阅读目标版本兼容/迁移说明，停止并完成上述备份，记录旧二进制摘要与版本。保留旧二进制供审查，不把它立即运行到升级后的 DB。
2. 对新架构产物验证可信来源/摘要、`version`、`licenses` 和前述空合成路径 doctor。确认其适用版本，不能把开发交叉构建视为发布产物。
3. 用 `copy_new_regular` 把新产物复制到同目录全新 `~/.local/bin/miskoai.next-TIMESTAMP`，设为 0700 并再次比对摘要。检查原 `miskoai` 仍是原本用户拥有的单链接普通文件，摘要与步骤 1 完全一致；确认服务仍停止，才用 `mv -T -- "$staged" "$HOME/.local/bin/miskoai"` 在同一文件系统上原子替换这个**明确已核实的安装文件**。`staged` 是操作者设置的上述新文件绝对路径。这里的替换只属于已决定的升级；首次安装绝不能套用它覆盖既有目标。异常路径/链接/摘要变化应拒绝替换。
4. 对原目录运行新二进制 `config validate`，保留配置优先级；需变更 unit 时先保存旧 unit 并审查新差异，不覆盖来源不明对象。在目标 Linux 再做 verify / daemon-reload。
5. 操作者确认迁移和账号流量都准备就绪，再显式 start；检查安全摘要与错误分类。启动可能迁移 DB，失败时停止调查，不能循环换旧二进制尝试。

**仅回退二进制不能撤销 schema 迁移。** 若新版曾打开数据库，旧版是否兼容须按该版本证据判断，不能保证可运行。数据库回退使用经过验证且确认来源的旧备份，经现有 CLI `restore PATH`，不手工覆盖 DB，也不删除不明 journal。示例在已 stop 并核实完成后、使用支持该备份的已验证二进制执行：

只有在已确认旧二进制支持最终数据库 schema、配置和账号恢复状态时，才可按上面相同的已核实停止/摘要/新暂存文件替换流程装回记录过的旧二进制；先恢复它的 0700 执行权限，保留所有恢复快照。无法证明兼容时继续停机，不能用启动旧版试错来代替兼容性结论。

```bash
(
    for name in ${!MISKOAI_@}; do unset "$name"; done
    unset DEEPSEEK_API_KEY OLLAMA_API_KEY
    export MISKOAI_DATA_DIR="$HOME/.local/share/miskoai"
    export MISKOAI_LISTEN=127.0.0.1:8080
    export MISKOAI_CREDENTIALS_FILE="$HOME/.config/miskoai/provider.json"
    "$HOME/.local/bin/miskoai" restore /absolute/path/to/verified/private-backup/database.db
)
```

restore 的候选输入上限为 256MiB，验证 schema / integrity 后安装；过程可把候选升级到该二进制支持的 schema，故 restore 也不等于恢复旧版必能识别的 schema。它保留当前 DB 为 `pre-restore-*.db`，先持久化 `.miskoai-restore-pause.json`，暂停外部频道等待明确人工对账。恢复较旧的 dedup/outbox 状态可能与实际已发消息冲突：核对账号、游标、已发送/未决消息和歧义发送，保持暂停，不能自动重发、盲删暂停标记或授权文件。现有 CLI 没有“自动解除恢复暂停”命令；使用兼容已恢复 schema 的二进制，在操作者明确授权后启动仍保留暂停的服务，通过已认证管理页面的“人工核对后恢复通道”流程，在完成对账并明确允许恢复外部回复后确认。该流程会解除暂停并可能恢复频道流量，不能用来跳过对账；无法完成核对时保留暂停/停机并寻求维护者审查。不要自行编造命令。恢复错误要求 manual recovery 时保留仍存在的候选、旧 DB、锁、暂停标记及所有安全诊断，不能手工清理后重试冒充成功。

遗留 `.miskoai.lock` 不是 OS 自动失效的锁；确认全部持有者退出后仍需审查来源、身份和停机状态，应用不会自动移除旧锁。journal 存在时 restore 会拒绝，应先正确停止/检查点或人工协调；不要用删除 WAL/SHM/journal 来“修复”。停止 timeout 及强杀后尤其要保留这些状态。

## 7. 安全排错与卸载

排错先使用有限 `systemctl --user show` 单项状态、CLI config validate、合成 doctor 和已授权运行中的 status。需要日志时只在本机查看必要的 `journalctl --user -u miskoai.service -n 50 --no-pager`，先检查并脱敏再分享，不能保证第三方/主机日志毫无敏感内容。不要输出 env、printenv、完整 systemctl show / show-environment、环境文件、授权 JSON、数据库或 HTTP 请求/响应体。认证/余额/限流等安全分类错误也不证明实际供应商测试通过。权限错误应核实具体对象身份和所有权，不能放宽权限绕过 privatefs。

卸载默认保留全部数据、配置、账号授权和备份。先确认 unit 属于这次安装（本机比较模板/已记录变更）、二进制摘要与安装记录一致，所有祖先真实且目标仍由本用户拥有的单链接普通文件。停止并检查完成，随后禁用：

```bash
systemctl --user stop miskoai.service
systemctl --user show miskoai.service -p ActiveState -p SubState -p Result -p MainPID
systemctl --user disable miskoai.service
check_regular "$HOME/.config/systemd/user/miskoai.service"
check_regular "$HOME/.local/bin/miskoai"
```

在以上身份/摘要核对通过并明确决定卸载后，只删除这两个精确安装文件：

```bash
rm -- "$HOME/.config/systemd/user/miskoai.service"
rm -- "$HOME/.local/bin/miskoai"
systemctl --user daemon-reload
```

若发现不明 drop-in、替代 unit、符号链接或文件变化，停止并调查，不能顺手删除。没有递归删除配方。私有数据删除是另一次明确的所有者决定；本流程不删目录、备份、配置、授权或遗留恢复状态，也不自动关闭管理员为其他用途批准的 lingering。

## 8. 验证边界

交付的本地证据只证明已接受源码可以原生 Windows 构建，以及 version / licenses / config validate / 合成 doctor 的 CLI 合同。systemd 模板已按上游手册人工逐项核对；Linux 语法验证、实际用户 manager 启停、断登常驻、真正 graceful join、实际服务器部署、arm64 运行、512MB RSS、实时供应商/微信/文件处理和最终发布验收均须单独取得真实证据，不能由本手册或交叉编译推导。
