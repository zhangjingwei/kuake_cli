# 变更日志

## [Unreleased]

- 修复正常文件名与 `.part` 断点文件碰撞导致的数据损坏/丢失：断点改为按远程文件 ID 与绝对目标路径隔离的用户缓存，并校验来源；不再自动迁移旧版未验证的部分文件。完成时使用目标目录临时副本替换，保留失败前的目标文件。

- Windows 持久化 Cookie 改用禁止继承、仅允许当前用户 SID 访问的 ACL；Linux/macOS 继续使用目录 `0700`、文件 `0600`。
- 目录下载在远端路径转换为本地路径后，通过绝对路径与 `filepath.Rel` 再次校验目标位于下载根目录内，阻止 Windows 反斜杠路径穿越。
- 独立缓存断点续传会校验 HTTP `206 Content-Range` 起点与本地偏移一致；不一致时删除临时文件并无 Range 重新完整下载。
- CI 增加 Windows 测试；已知的主分支 `internal/guard` Windows 测试暂不纳入该平台任务。

## v1.5.0

### BREAKING

- **`config.json` 支持已移除**。`sdk.LoadConfig` / `sdk.SaveConfig` 删除；`sdk.NewQuarkClient` 签名改为 `NewQuarkClient(cookies ...string)` —— 不再接受 `configPath`，且无 cookie 时直接 panic。CLI 的 `-c` / `--config` 选项移除；`.env` 自动加载也不再读取「`-c` 同目录」那一份，仅从当前工作目录加载。原 `Config` 结构体保留但已无生产路径使用，将来可能删除。
  - 升级方法：把 `config.json` 中 `access_tokens` 的整段 Cookie 改写为环境变量 `KUAKE_COOKIE`（或放到 `.env` 中），凭证优先级 `KUAKE_COOKIE > KUAKE_PUS+KUAKE_PUUS > -cookies/--cookies` 不变。
  - 多账号轮询能力**暂未保留**；如需恢复请提 issue。
- **Go 版本**：`go.mod` 升至 Go **1.25**（受新依赖 `mark3labs/mcp-go` 约束）。

### 新功能：`kuake-mcp` MCP server

- 新增独立二进制 `kuake-mcp`（位于 `mcp/`），以 stdio JSON-RPC 协议把 14 个网盘操作暴露为 MCP 工具：`quark_user`、`quark_list`、`quark_info`、`quark_download`、`quark_upload`、`quark_create`、`quark_move`、`quark_copy`、`quark_rename`、`quark_delete`、`quark_share_create`、`quark_share_delete`、`quark_share_list`、`quark_share_save`。可与 Claude Code 等 MCP 客户端配合使用。
- 新增 `internal/guard` 包：环境变量驱动的黑名单守门，控制可执行的操作（`KUAKE_DENY_OPS`）、远端路径（`KUAKE_DENY_PATHS`）、上传扩展名（`KUAKE_DENY_EXTS`）、上传体积上限（`KUAKE_MAX_UPLOAD_MB`）、下载沙箱根目录（`KUAKE_DOWNLOAD_DIR`）。
- **MCP 安全硬化**（不可关闭，硬编码）：
  - **下载** —— 校验远端 API 返回的 `file_name`，拒绝空、含 `/`、`\` 或 `..`，防止恶意 / 被污染的远端响应突破下载沙箱。
  - **上传** —— 拒绝 `localPath` 落在系统路径（`/etc/`、`/proc/`、`/sys/`、`/dev/`、`/root/`、`/var/{log,lib,spool,db,root}/` 及 `/private/` 镜像）、凭证目录（`.ssh/`、`.aws/`、`.gnupg/`、`.kube/`、`.docker/`、`.config/gh/`）或敏感 basename（`id_rsa`、`id_ed25519`、`id_dsa`、`id_ecdsa`、`.netrc`、`.pgpass`、`.my.cnf`、`*_history`）；解析符号链接后再匹配，防止 symlink 绕过；macOS 的 `/var/folders/`、`/var/tmp/` 仍可使用。
  - **share_save** —— 增加 `CheckPath(dst)`，使 `KUAKE_DENY_PATHS` 也能保护转存目标目录。
- 新增 `mcp/server.go`、`mcp/main.go`、`mcp/tools/{types,file,share}.go`：stdio 端将 `os.Stdout` 重定向到 `stderr`，仅由 `Listen(...)` 写入真正的 stdout fd，避免任何库的 `fmt.Print*` 污染 JSON-RPC 通道；`KUAKE_COOKIE` 缺失时服务器仍能启动，每次工具调用返回明确错误（不直接退出，方便 MCP 客户端自检）。

### 构建与文档

- `build.sh` 新增 5 个平台的 `kuake-mcp` 产物：`kuake-mcp-{linux,darwin}-{amd64,arm64}` 与 `kuake-mcp-windows-amd64.exe`。
- 新增仓库根 [`.mcp.json.example`](../.mcp.json.example)，作为 Claude Code MCP 集成模板（含 cookie + 黑名单/沙箱环境变量），`.mcp.json` 已加入 `.gitignore` 防误提交。
- README、`docs/cli.md`、`.env.example`：删除 `-c` / `--config` 与 `config.json` 相关章节；新增 `kuake-mcp` 简介与专用环境变量参考表。

## v1.4.5

### BREAKING

- **CLI**：移除已废弃的「子命令之后首个以 `.json` 结尾的位置参数视为配置文件」行为（曾导致 `upload` 本地 JSON 文件等误解析）。指定配置文件请始终使用 **`-c` / `--config`** 并传入路径，凭证亦可使用 `.env` / `KUAKE_COOKIE` 等；曾写 `kuake user ./my.json` 的脚本请改为 `kuake -c ./my.json user`。

## v1.4.4

### BREAKING

- **认证凭证来源优先级**调整为：`KUAKE_COOKIE`（trim 后非空）优先于 `-cookies` / `--cookies`，再优先于配置文件。曾依赖「命令行覆盖已 export 的 `KUAKE_COOKIE`」的脚本须先清除环境变量（POSIX: `unset KUAKE_COOKIE`；PowerShell: `Remove-Item Env:KUAKE_COOKIE`）或改用配置文件。
- **上传**：未传 `--max_upload_parallel` 时，`kuake` 会读取 `KUAKE_UPLOAD_PARALLEL`（1–16）；传入 flag 时 **flag 优先于环境变量**。
- **文档**：已移除「`kuake` 二进制通过 `KUAKE_PATH` 解析路径」的表述；请通过系统 **PATH** 或包装脚本定位 `kuake`。
- **Go module（BREAKING）**：`module` 路径改为 `github.com/zhangjingwei/kuake_cli`，与 GitHub 仓库 `zhangjingwei/kuake_cli` 对齐，可使用 `go get github.com/zhangjingwei/kuake_cli@<版本>`。请将原 `import "kuake_sdk/..."` 全部改为 `import "github.com/zhangjingwei/kuake_cli/..."`。

### 构建、发布与文档

- **构建与发布**：`build.sh` 在 `dist/` 中随版本打包 OpenClaw 技能目录 `openclaw/kuake_skill`（默认 `kuake_skill-<版本>.zip`；若无 `zip` 命令则生成 `kuake_skill-<版本>.tar.gz`）；构建产物列表使用 `ls -lhA` 以包含 `.env.example`
- **发布脚本**：`push.sh` 将上述技能包作为 Release 附件上传，并移除已不存在的 `openclaw/DEPLOYMENT.md`、`openclaw/SKILL.md` 引用；若缺少技能包则重新执行 `./build.sh`
- **文档**：OpenClaw 相关说明收敛为「预编译 `kuake` + PATH + 技能目录」的普通用户路径；修复 `docs/cli.md` 中失效链接，README 文档表与功能描述同步

## v1.4.3

- **SDK 健壮性与校验（OpenSpec `robustness-refactor`）**
  - 新增 `sdk/validation`：链式校验器、分页与路径安全（含 `ValidPathResult`）、默认值注入、`crypto/rand` 安全随机数、统一错误码与中文化校验消息
  - `UploadFile` / `DownloadFile` 对远端路径做规范化与安全校验；分享列表类方法分页与排序入参校验；CLI 经 `cmd/validation` 安全解析分页与 JSON 路径
- **上传**：`UploadFile` 读取 `KUAKE_UPLOAD_PARALLEL`（1–16）覆盖服务端 `part_thread`（不超过分片数）；修复并行上传出错时重复关闭 `jobCh` 引发的 panic。

## v1.4.2

- **SDK 路径与列表**
  - 修复 `listByFid` 翻页：仅以本页条数是否满页决定是否继续，避免依赖不可靠的 `total` 导致列表缺项
  - Windows 下远程路径统一按 POSIX 处理：`GetFileInfo` / `UploadFile` 使用 `path.Base` 解析远端路径；`GetFileInfo` 列表回退分支使用 `path.Join` 拼接子路径
  - `listByFid` 兼容 JSON 将 `fid` 解析为 `float64` 的情况
- **下载（OSS 直链鉴权）**
  - `DownloadFile` 对下载 URL 的 GET 请求补充与网盘 Web 页一致的请求头（如 `User-Agent`、`Referer`、`Sec-Fetch-*`、`Accept`/`Cache-Control` 等），并设置由客户端解析得到的完整 `Cookie` 头；下载使用独立 `http.Client` 且 Transport **启用 HTTP/2**，与主 API 客户端（强制 HTTP/1.1）分离，减轻 OSS 回调侧 **HTTP 403** / `RequestDeniedByCallback` 类失败（见 `buglist.txt` ISSUE-006）。
  - 若 `access_tokens` 仍仅为不完整片段（例如仅 `__pus`），仍可能 403；需使用浏览器 `pan.quark.cn` 复制的整段 Cookie（调试模式下见 `DownloadFile` 相关提示）。
- **测试与记录**
  - 新增可选端到端回归 `TestE2E_Regression_CoreFlow`（`E2E_REGRESSION=1` 或 `INTEGRATION_TEST=1`；凭证为 `KUAKE_COOKIE` 或 `KUAKE_PUS`/`KUAKE_PUUS`，与 CLI 一致）
  - 问题与回归说明见仓库根目录 `buglist.txt`

## v1.4.1

- 新增主规格架构文档：`specs/architecture/spec.md`
- 补充项目目录和模块划分说明
- 更新 `.gitignore`，排除本地辅助配置但保留 `.github/workflows/`
- 归档 OpenSpec 变更：`openspec/changes/archive/2026-04-11-main-architecture`
- 新增 OpenClaw skill 包支持：添加 `openclaw/kuake_skill/SKILL.md`，提供标准 OpenClaw skill 格式以便 agent 集成 kuake CLI 能力

## v1.4.0

- **OpenClaw 技能集成**
  - 新增 kuake OpenClaw 技能支持
  - 添加环境变量 `KUAKE_COOKIE` 支持，符合 OpenClaw 标准配置方式
  - 认证优先级：`-cookies` 参数 > 环境变量 `KUAKE_COOKIE` > 配置文件（**历史记载有误**，已于 **v1.4.4** 更正：`KUAKE_COOKIE` trim 后非空优先，见该版本 BREAKING）
  - 支持通过 `KUAKE_PATH` 环境变量指定完整路径，不依赖 PATH 检测（**历史记载有误**，`kuake` 不读取 `KUAKE_PATH`；请使用 PATH，见 **v1.4.4** BREAKING）
  - 优化 OpenClaw 技能文档，添加 fallback 逻辑说明
  - 简化部署文档，提供更清晰的配置选项

## v1.3.9

- 新增 `--policy` 上传去重策略（PR #16）
  - 新增 `UploadPolicy`/`UploadOptions` 类型定义
  - 支持三种策略：`skip`（跳过已存在文件）、`rename`（重命名）、`overwrite`（覆盖）
  - `UploadFile` 函数签名扩展为 4 参数，支持策略配置
- 并行上传优化（PR #18，8 项核心改动）
  - 嵌入式哈希：MD5+SHA1 嵌入分片读取，提高上传效率
  - `parallel_upload` 握手协议：优化并行上传协商流程
  - `X-Oss-Hash-Ctx` MarshalBinary 修复：修复序列化问题
  - Nl/Nh 32 位拆分：支持 >536MB 大文件上传
  - 多线程并发上传：提升上传速度至 7-14 MB/s
  - 分片级指数退避重试：每个分片最多重试 3 次，提高成功率
  - 断点续传 PartThread 恢复：支持中断后恢复并行上传状态
  - `x-oss-user-agent` 版本统一：统一版本标识
- `user` 命令容量查询 + `--version`（PR #17）
  - `getMemberInfo()` 合并容量/会员信息：统一用户信息获取接口
  - 版本号常量定义：规范化版本管理
  - `--version` 参数拦截：新增版本号查询命令参数
- 新增管道模式（Pipeline Pattern）支持，支持命令链式组
  - `list` 命令新增 `--stream` 选项，输出流式 JSON（每行一个文件对象）
  - `delete`、`info`、`download` 命令支持从 stdin 读取 JSON 输入
  - 自动检测 stdin，有数据时自动进入管道模式
  - 支持与其他 Unix 工具（如 `jq`、`grep`、`head` 等）组合使用
  - 保持向后兼容，无 stdin 时使用命令行参数
  - 实现流式处理，支持逐行处理大量文件，内存占用低
  - 改进错误处理，优雅处理 broken pipe 错误
  - 统一数据类型处理，只处理 `QuarkFileInfo` 类型，提高代码一致性和可维护性
  - 优化代码结构，移除多类型处理的复杂逻辑，简化代码实现

## v1.3.8

- 新增 `-cookies` 参数支持，可直接通过命令行指定 cookie 值，无需配置文件
  - 自动为 cookie 值添加 `__pus=` 前缀（如果缺失）
  - 自动添加末尾分号（如果缺失）
  - 使用 `-cookies` 参数时，不会读取配置文件，提高效率并避免不一致
- 修复并行上传逻辑，多分片文件禁用并行上传（因为需要使用 X-Oss-Hash-Ctx）

## v1.3.7

- 新增并行上传功能，支持通过 `--max_upload_parallel` 参数或 `KUAKE_UPLOAD_PARALLEL` 环境变量配置并行度（1-16，默认 4）
- 改进路径参数处理，明确要求所有路径参数必须用引号包裹
- 新增转存分享文件功能，新增 `share-save` CLI 命令

## v1.3.6

- 新增 X-Oss-Hash-Ctx 支持，实现 OSS 分片上传的增量 SHA1 哈希上下文
- 改进断点续传功能，支持 HashCtx 的保存和恢复

## v1.3.5

- 新增断点续传功能，上传中断后可自动恢复
- 改进上传进度显示，显示上传速度、剩余时间等信息
- 优化命令行参数解析，支持 `-c/--config` 参数指定配置文件路径
- 增强上传错误处理和超时处理
- 改进分享创建错误处理，增加重试机制

## v1.3.4

- 修复配置文件读取路径问题，支持从可执行文件所在目录读取配置文件

## v1.3.3

- 修复 Windows 路径处理问题，支持跨平台路径兼容性

## v1.3.2

- 新增取消分享功能，新增 `share-delete` CLI 命令

## v1.3.1

- 修复 CLI 错误消息转义问题
- 优化 API 错误响应处理
- 新增完整的单元测试套件
