# kuake CLI 使用说明

命令行工具的完整参考（配置、子命令、输出约定与示例）。**OpenClaw 用户**：安装 Releases 中的 `kuake` 并配置 `PATH` 与凭证（如下文环境变量）；技能加载与自检见 [openclaw/kuake_skill/SKILL.md](../openclaw/kuake_skill/SKILL.md)、[verification.md](../openclaw/kuake_skill/verification.md)。

## 配置说明

### 凭证来源

> **v1.5.0 BREAKING**：自 v1.5.0 起 `kuake` 不再支持 `config.json`，`-c, --config` 选项已移除。原 `access_tokens` 中的多账号轮询能力**暂未保留**——如需恢复，请提 issue。

凭证按以下优先级解析（前一项 trim 后非空即生效）：

1. `KUAKE_COOKIE`（整段浏览器 Cookie）
2. `KUAKE_PUS` + `KUAKE_PUUS`（值，不要写 `__pus=` / `__puus=` 前缀）
3. `-cookies` / `--cookies` 命令行参数

**Cookie 内容**：从浏览器登录 `pan.quark.cn` 后，开发者工具 → Network → 复制整段 `Cookie` 请求头值。一段完整 Cookie 通常含 `__pus`、`__puus`、`_UP_*`、`tfstk` 等键，下载等场景需要完整段。

**安全提示**：Cookie 等于完整登录态，请保管好；勿提交到版本控制；勿分享给他人。

## CLI 工具使用

### 基本用法

```bash
kuake [options] <command> [arguments...]
```

**选项**：
- `-cookies, --cookies <value>`: 在 `KUAKE_COOKIE` 为空（或 trim 后为空）时指定 Cookie；与 `KUAKE_COOKIE` 走相同的规范化（裸串补 `__pus=`、含 `__puus=` 时不重复加 `__pus=`、末尾分号）

### Cookie 持久化

CLI 可以把当前环境中的 Cookie 保存到用户配置目录，之后无需重复 `export`：

```bash
export KUAKE_COOKIE='浏览器复制的完整 Cookie'
kuake auth save
unset KUAKE_COOKIE
kuake auth status
kuake user
```

默认路径为系统用户配置目录下的 `kuake/credentials.json`（Linux 通常为
`~/.config/kuake/credentials.json`）。Linux/macOS 上配置目录权限为 `0700`、文件权限为
`0600`；Windows 上目录和文件使用禁止继承的 ACL，仅向当前进程用户 SID 授予访问权限，
不依赖 `os.Chmod` 模拟 Unix 权限。
`KUAKE_CONFIG_DIR` 可以覆盖配置目录。使用 `kuake auth clear` 删除持久化凭证。
`auth status` 仅输出 Cookie 名称，不输出任何值。

凭证优先级：`KUAKE_COOKIE` > `KUAKE_PUS+KUAKE_PUUS` > `-cookies` > 持久化 Cookie。

### 环境变量参考

仓库根目录提供 **[`.env.example`](../.env.example)**，可复制为 `.env` 后按需填写。`kuake` 在**解析完命令行之后**、创建客户端之前，若**当前工作目录**下存在 `.env` 则自动加载（已在进程环境中的键**不会被覆盖**，与 [godotenv](https://github.com/joho/godotenv) 的 `Load` 语义一致）。设置 **`KUAKE_LOAD_DOTENV=0`** 可关闭自动加载。仍可在 shell、`direnv` 或 CI 中事先 `export`，优先级高于 `.env` 文件中的默认值。

| 变量名 | 谁读取 | 用途 | 说明 |
|--------|--------|------|------|
| `KUAKE_LOAD_DOTENV` | `kuake`（cmd） | 是否自动加载 cwd 下的 `.env` | 仅当值为 **`0`**（trim 后）时关闭；未设置或其它值均启用（仅当 `.env` 存在才会加载） |
| `KUAKE_COOKIE` | `kuake`（cmd），`kuake-mcp` | 整段会话 Cookie | **优先于**下方拆分变量；trim 并规范化后非空则作为凭证（覆盖 `-cookies`） |
| `KUAKE_PUS` | `kuake`（cmd），`kuake-mcp` | `__pus` 的**值**（不要写 `__pus=` 前缀） | 仅当 `KUAKE_COOKIE` 规范化后为空时使用；可与 `KUAKE_PUUS` 组合 |
| `KUAKE_PUUS` | `kuake`（cmd），`kuake-mcp` | `__puus` 的**值**（不要写 `__puus=` 前缀） | 同上；可单独使用（仅 `__puus`）或仅 `KUAKE_PUS` 或两者一起 |
| `-cookies` / `--cookies` | `kuake`（cmd） | 同上 | 当 `KUAKE_COOKIE` 为空时使用；仍会通过 CLI 做与 env 相同的规范化（`__pus=`、分号） |
| `KUAKE_CONFIG_DIR` | `kuake`（cmd） | 持久化凭证目录 | 未设置时使用系统用户配置目录下的 `kuake`；仅 CLI 读取 |
| `KUAKE_UPLOAD_PARALLEL` | `kuake`（cmd，`upload`）与 SDK（`UploadFile`） | 上传并行 worker 数 1–16 | CLI：未传 `--max_upload_parallel` 时从本变量读取并 `Setenv`；**SDK：上传时若本变量合法则覆盖服务端 `part_thread`**，且不超过分片总数与 16；**命令行 flag 优先于**仅由 shell `export` 的值 |
| `KUake_DEBUG` | SDK（`QuarkClient`） | 调试输出 | 设为 `1` 开启；变量名大小写以代码为准 |
| `E2E_REGRESSION` / `INTEGRATION_TEST` | `go test ./sdk` | 启用端到端回归 `TestE2E_Regression_CoreFlow` | 置 `1` 后须提供 **`KUAKE_COOKIE` 或 `KUAKE_PUS`+`KUAKE_PUUS`**；测试会尝试加载 cwd 下的 `.env`；非 `kuake` 二进制行为 |

#### `kuake-mcp` 专用变量

| 变量名 | 用途 | 默认 |
|--------|------|------|
| `KUAKE_DENY_OPS` | 冒号分隔的禁用操作名（`upload`/`delete`/`move`/`copy`/`rename`/`create`/`share_create`/`share_delete`/`share_save`/`user`） | 空 |
| `KUAKE_DENY_PATHS` | 冒号分隔的远端禁用路径前缀；命中前缀或 == 路径的远端操作被拒 | 空 |
| `KUAKE_DENY_EXTS` | 冒号分隔的禁止上传扩展名（按小写匹配） | 空 |
| `KUAKE_MAX_UPLOAD_MB` | 上传单文件大小上限（MiB），超过即拒 | 0（不限制） |
| `KUAKE_DOWNLOAD_DIR` | 下载沙箱根目录；`quark_download` 把文件写入此路径 + `local_sub_dir` | 当前工作目录 |

`kuake-mcp` 还内置一份**硬编码黑名单**，无需配置：上传时拒绝 `/etc/`、`/proc/`、`/sys/`、`/dev/`、`/root/`、`/var/{log,lib,spool,db,root}/`、`.ssh/`、`.aws/`、`.gnupg/`、`.kube/`、`.docker/`、`.config/gh/` 等系统/凭证目录，以及 `id_rsa`、`id_ed25519`、`.netrc`、`.pgpass` 等敏感 basename；下载时拒绝远端 `file_name` 含 `..`、`/`、`\` 的路径穿越尝试。

使用 OpenClaw 等自动化环境时，请保证 **`kuake` 在 `PATH` 中**；`kuake` **不**读取 `KUAKE_PATH` 环境变量。

### 可用命令

| 命令 | 说明 | 示例 |
|------|------|------|
| `auth <save\|status\|clear>` | 保存、检查或清除本地持久化 Cookie | `kuake auth save` |
| `user` | 获取用户信息 | `kuake user` |
| `list [path] [--stream]` | 列出目录内容（默认: "/"），使用 `--stream` 输出流式 JSON 用于管道模式 | `kuake list "/"` 或 `kuake list "/" --stream` |
| `info <path>` | 获取文件/文件夹信息（支持管道模式） | `kuake info "/file.txt"` |
| `download <path> [dest] [--workers N]` | 下载文件或递归下载目录；目录下载支持并发与进度（支持管道模式） | `kuake download "/folder" ./local --workers 4` |
| `upload <file> <dest> [--max_upload_parallel N]` | 上传文件（上传进度输出到 stderr，支持并行上传） | `kuake upload "file.txt" "/file.txt"` 或 `kuake upload "file.txt" "/file.txt" --max_upload_parallel 4` |
| `create <name> <pdir>` | 创建文件夹（pdir 为父目录路径，根目录使用 "/"） | `kuake create "test_folder" "/"` |
| `move <src> <dest>` | 移动文件/文件夹 | `kuake move "/file.txt" "/folder/"` |
| `copy <src> <dest>` | 复制文件/文件夹 | `kuake copy "/file.txt" "/folder/"` |
| `rename <path> <newName>` | 重命名文件/文件夹 | `kuake rename "/file.txt" "new_name.txt"` |
| `delete <path>` | 删除文件/文件夹（支持管道模式） | `kuake delete "/file.txt"` |
| `share <path> <days> <passcode>` | 创建分享链接 | `kuake share "/file.txt" 7 "false"` |
| `share-delete <share_id_or_path> [share_id_or_path2] ...` | 取消分享（支持通过 share_id 或文件路径） | `kuake share-delete "fdd8bfd93f21491ab80122538bec310d"` 或 `kuake share-delete "/file.txt"` |
| `share-list [page] [size] [orderField] [orderType]` | 获取我的分享列表 | `kuake share-list` 或 `kuake share-list 1 50 "created_at" "desc"` |
| `share-save <share_link> [passcode] [dest_dir]` | 转存分享文件到自己的网盘 | `kuake share-save "https://pan.quark.cn/s/xxx"` 或 `kuake share-save "https://pan.quark.cn/s/xxx" "1234" "/folder"` |
| `help` | 显示帮助信息 | `kuake help` |

**重要提示**：
- 所有路径参数必须用引号包裹（`"path"`）
- 根目录使用 `"/"` 表示
- 下载单文件时，`dest` 不带扩展名会自动创建/复用为目录；带扩展名时作为目标文件名
- 下载远程目录时，`dest` 始终作为目录，即使目录名带扩展名；默认并发数为 4，可用 `--workers 1..16` 调整
- 断点数据保存在系统用户缓存目录的 `kuake/downloads` 下，按远程文件 ID 与本地绝对路径隔离；再次执行相同命令会通过 HTTP Range 续传，不会占用正常的 `.part` 文件名。旧版 `.part` 与未完整最终文件不再自动作为断点使用，会保留并重新下载。完成时先复制到目标目录的临时文件再替换目标，因此需要额外磁盘空间。
- 交互终端中的目录下载进度会原地刷新；重定向日志时每 5 秒输出一次整体进度，避免刷屏
- `days` 参数：`0`=永久，`1`=1天，`7`=7天，`30`=30天
- `passcode` 参数：`"true"`=需要提取码，`"false"`=不需要提取码
- `share-save` 命令说明：
  - `share_link`: 分享链接（如 `https://pan.quark.cn/s/xxx`），会自动提取 pwd_id
  - `passcode`: 提取码（可选），如果分享链接中包含提取码会自动提取
  - `dest_dir`: 目标目录（可选，默认 `"/"`），可以是路径或 FID
  - 默认会转存分享中的所有文件到指定目录
- **并行上传参数**：
  - `--max_upload_parallel N`：设置上传并行 worker 数（1–16）；**传入本 flag 时优先于**环境变量 `KUAKE_UPLOAD_PARALLEL`（CLI 会写入进程环境供 SDK 读取）
  - 未传 `--max_upload_parallel` 时，可由环境变量 `KUAKE_UPLOAD_PARALLEL`（1–16）控制；**均未设置时由服务端预上传返回的 `part_thread` 决定**（常见约 3）
  - 实际上传 worker 数还会受 **分片总数** 上限约束（不超过 `ceil(文件大小 / part_size)`）
  - 并行上传仅在多分片且最终并行度大于 1 时启用；断点续传在相同条件下仍可走并行路径
- **管道模式**：
  - `list` 命令使用 `--stream` 选项输出流式 JSON（每行一个文件对象）
  - `delete`、`info`、`download` 命令支持从 stdin 读取 JSON 输入
  - 自动检测 stdin，有数据时自动进入管道模式
  - 每行输入应为 JSON 对象，包含 `path` 或 `fid` 字段
  - 支持与其他 Unix 工具组合使用，如 `jq`、`grep`、`head` 等

### 输出格式

所有命令的结果都以 JSON 格式输出到 stdout：

**成功响应**：
```json
{
  "success": true,
  "code": "OK",
  "message": "操作成功",
  "data": {
    ...
  }
}
```

**错误响应**：
```json
{
  "success": false,
  "code": "ERROR_CODE",
  "message": "错误描述",
  "error": "详细错误信息"
}
```

**注意**：
- 所有结果（包括成功和错误）都以 JSON 格式输出到 stdout
- 上传进度、帮助信息和序列化错误输出到 stderr
- 这样设计便于其他进程解析 JSON 结果，进度信息不会混入 JSON 输出

### 退出码

- `0`: 操作成功
- `1`: 操作失败

### 使用示例

```bash
# 获取用户信息（凭证来自 .env / 环境变量，见上文）
./kuake-{version}-{os}-{arch} user

# 列出根目录
./kuake-{version}-{os}-{arch} list "/"

# 获取文件信息
./kuake-{version}-{os}-{arch} info "/file.txt"

# 获取文件下载链接
./kuake-{version}-{os}-{arch} download "/file.txt"

# 上传文件（使用默认并行度 4）
./kuake-{version}-{os}-{arch} upload "file.txt" "/file.txt"

# 上传文件（指定并行度为 8）
./kuake-{version}-{os}-{arch} upload "file.txt" "/file.txt" --max_upload_parallel 8

# 上传文件（通过环境变量设置并行度）
export KUAKE_UPLOAD_PARALLEL=8
./kuake-{version}-{os}-{arch} upload "file.txt" "/file.txt"

# 创建文件夹（根目录）
./kuake-{version}-{os}-{arch} create "test_folder" "/"

# 移动文件
./kuake-{version}-{os}-{arch} move "/file.txt" "/folder/"

# 复制文件
./kuake-{version}-{os}-{arch} copy "/file.txt" "/folder/"

# 重命名文件
./kuake-{version}-{os}-{arch} rename "/file.txt" "new_name.txt"

# 删除文件
./kuake-{version}-{os}-{arch} delete "/file.txt"

# 创建分享链接（7天，不需要提取码）
./kuake-{version}-{os}-{arch} share "/file.txt" 7 "false"

# 取消分享（通过 share_id）
./kuake-{version}-{os}-{arch} share-delete "fdd8bfd93f21491ab80122538bec310d"

# 取消分享（通过文件路径，会自动查找对应的 share_id）
./kuake-{version}-{os}-{arch} share-delete "/file.txt"

# 同时取消多个分享
./kuake-{version}-{os}-{arch} share-delete "share_id1" "share_id2" "/file.txt"

# 获取我的分享列表（使用默认参数）
./kuake-{version}-{os}-{arch} share-list

# 获取我的分享列表（指定分页和排序参数）
./kuake-{version}-{os}-{arch} share-list 1 50 "created_at" "desc"

# 转存分享文件到根目录
./kuake-{version}-{os}-{arch} share-save "https://pan.quark.cn/s/xxx"

# 转存分享文件（指定提取码和目标目录）
./kuake-{version}-{os}-{arch} share-save "https://pan.quark.cn/s/xxx" "1234" "/folder"

# 查看帮助
./kuake-{version}-{os}-{arch} help

# 使用 -cookies 参数（在 KUAKE_COOKIE 未设置时生效）
./kuake-{version}-{os}-{arch} -cookies "your_cookie_value_here" user
./kuake-{version}-{os}-{arch} -cookies "your_cookie_value_here" upload "file.txt" "/folder/file.txt"

# 管道模式示例
# 列出文件并批量删除
./kuake-{version}-{os}-{arch} list "/photos" --stream | ./kuake-{version}-{os}-{arch} delete

# 列出文件并获取每个文件的信息
./kuake-{version}-{os}-{arch} list "/" --stream | ./kuake-{version}-{os}-{arch} info

# 列出文件并获取下载链接
./kuake-{version}-{os}-{arch} list "/documents" --stream | ./kuake-{version}-{os}-{arch} download

# 结合 jq 进行过滤：列出大文件并删除
./kuake-{version}-{os}-{arch} list "/" --stream | jq -r 'select(.size > 1000000) | .path' | ./kuake-{version}-{os}-{arch} delete

# 列出文件并下载到指定目录
./kuake-{version}-{os}-{arch} list "/videos" --stream | ./kuake-{version}-{os}-{arch} download "./downloads"
```

**注意**：
- 示例中的 `{version}`、`{os}`、`{arch}` 需要替换为实际值
- Windows 用户需要添加 `.exe` 扩展名并使用 `.\` 前缀
- 如果已添加到 PATH，可以直接使用 `kuake` 命令

## 注意事项

- **文件名格式**：二进制文件名包含版本号，格式为 `kuake-{version}-{os}-{arch}` 或 `kuake-{version}-{os}-{arch}.exe`（Windows）
- **执行权限**：Linux/macOS 二进制文件已包含执行权限，可直接使用 `./` 前缀执行
- **路径参数**：所有路径参数必须用引号包裹（包含空格或特殊字符时），例如：`"./file name.txt"`、`"/path/to/file"`
- **跨平台路径支持**：
  - Windows 用户可以使用 Windows 风格的路径（`d:\a\b\c`），会自动转换为 Unix 风格
  - Linux/macOS 用户继续使用标准 Unix 路径格式（`/a/b/c`）
  - 所有路径最终都会标准化为 Unix 风格，确保跨平台一致性
- **Cookie 与凭证优先级**：
  - 顺序为：**`KUAKE_COOKIE`（整段，trim 后非空）** 优先于 **`KUAKE_PUS` + `KUAKE_PUUS`（拼接后再规范化）**，再优先于 **`-cookies` / `--cookies`**
  - 若 `KUAKE_COOKIE` 规范化后非空，则以其为准；否则若 `KUAKE_PUS` / `KUAKE_PUUS` 任一侧非空，则拼接后规范化为准；上述任一成立时，`-cookies` 不会作为会话凭证使用
  - `-cookies`、整段 `KUAKE_COOKIE` 与拆分拼接结果使用相同的规范化规则（裸串补 `__pus=`、含 `__puus=` 时不重复加 `__pus=`、末尾分号）
  - 示例：`kuake -cookies "your_cookie_value" user`（在环境变量未提供有效凭证时）
  - 端到端回归（`E2E_REGRESSION` / `INTEGRATION_TEST`）：**仅**从环境变量（及测试内加载的 `.env`）取凭证
- **操作说明**：
  - 所有操作都通过夸克网盘 API 进行
  - 需要有效的 Cookie（access_token）才能使用
  - 上传操作支持进度显示（输出到 stderr）
  - 上传并行度：未传 `--max_upload_parallel` 时可使用 `KUAKE_UPLOAD_PARALLEL`（1–16），由 SDK 读取并覆盖服务端 `part_thread`（且不超过分片总数）；**传入 `--max_upload_parallel` 时覆盖环境变量**；均未设置时由服务端 `part_thread` 决定
  - 删除目录会递归删除所有子文件和子目录
- **输出格式**：
  - CLI 工具的所有结果以 JSON 格式输出到 stdout，方便其他进程解析
  - 上传进度、帮助信息和序列化错误输出到 stderr，不会混入 JSON 输出
  - 成功时退出码为 0，失败时为 1
