# Kuake SDK

[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

夸克网盘文件管理 CLI 工具。

**注意：本项目仅提供二进制文件，不提供源代码。**

## 📋 目录

- [功能特性](#功能特性)
- [系统要求](#系统要求)
- [安装](#安装)
- [快速开始](#快速开始)
- [配置说明](#配置说明)
- [CLI 工具使用](#cli-工具使用)
- [注意事项](#注意事项)
- [免责声明](#免责声明)
- [许可证](#许可证)

## ✨ 功能特性

- **文件上传**: 将本地文件上传到夸克网盘（支持大文件分片上传）
- **文件下载**: 从夸克网盘下载文件到本地
- **文件删除**: 删除夸克网盘中的文件或目录
- **文件列表**: 列出夸克网盘指定目录下的所有文件和子目录
- **文件信息**: 获取文件或目录的详细信息（大小、修改时间等）
- **文件操作**: 移动、复制、重命名文件或目录
- **文件夹操作**: 创建文件夹
- **分享功能**: 创建分享链接，支持设置有效期和提取码
- **用户信息**: 获取用户信息
- **CLI 工具**: 提供命令行工具，方便其他进程调用

## 🔧 系统要求

- Linux / macOS / Windows 操作系统
- 有效的夸克网盘账号和 Cookie

## 📦 安装

### 方式一：下载预编译二进制文件

从 [Releases](https://github.com/zhangjingwei/kuake_sdk/releases) 页面下载对应平台的二进制文件：

- **Linux**: `kuake-linux-amd64`
- **macOS**: `kuake-darwin-amd64`
- **Windows**: `kuake-windows-amd64.exe`

**安装步骤**：

```bash
# 1. 下载对应平台的二进制文件

# Linux amd64:
wget https://github.com/zhangjingwei/kuake_sdk/releases/latest/download/kuake-linux-amd64

# macOS amd64:
wget https://github.com/zhangjingwei/kuake_sdk/releases/latest/download/kuake-darwin-amd64

# Windows amd64:
wget https://github.com/zhangjingwei/kuake_sdk/releases/latest/download/kuake-windows-amd64.exe

# 2. 下载配置文件示例（可选）
wget https://github.com/zhangjingwei/kuake_sdk/releases/latest/download/config.json

# 3. 添加执行权限（Linux/macOS）
chmod +x kuake-linux-amd64
# 或
chmod +x kuake-darwin-amd64

# 4. 编辑配置文件，填入您的 Cookie
# 使用文本编辑器打开 config.json，替换示例值

# 5. 重命名并移动到 PATH（可选）
mv kuake-linux-amd64 kuake
sudo mv kuake /usr/local/bin/

# 或者直接使用
./kuake-linux-amd64 user-info
```

## 🚀 快速开始

### 1. 创建配置文件

**方式一：从 Release 下载示例配置文件**

从 [Releases](https://github.com/zhangjingwei/kuake_sdk/releases/latest) 页面下载 `config.json` 示例文件，然后编辑填入您的 Cookie。

**方式二：手动创建配置文件**

创建 `config.json` 文件：

```json
{
  "Quark": {
    "access_tokens": [
      "ctoken=your_ctoken_value_here; __pus=your_pus_value_here;"
    ]
  }
}
```

**如何获取 Cookie**：
1. 打开浏览器，登录夸克网盘
2. 打开开发者工具（F12）
3. 在 Network 标签页中，找到任意一个请求
4. 复制请求头中的 `Cookie` 值（完整的 Cookie 字符串）
5. 将 Cookie 值粘贴到 `config.json` 文件的 `access_tokens` 数组中

### 2. 使用 CLI 工具

```bash
# 获取用户信息
./kuake user-info

# 上传文件
./kuake upload file.txt /file.txt

# 列出目录
./kuake list /
```

## ⚙️ 配置说明

### 配置文件格式

```json
{
  "Quark": {
    "access_tokens": [
      "ctoken=your_ctoken_value_here; __pus=your_pus_value_here;"
    ]
  }
}
```

**重要说明**: 
- `access_tokens` 字段是一个字符串数组，支持配置多个用户的 Cookie
- 每个字符串存储的是完整的 Cookie 字符串（所有 cookie 用分号和空格分隔）
- 从浏览器开发者工具中复制完整的 Cookie 值
- 示例格式：`cookie1=value1; cookie2=value2; cookie3=value3`
- 支持多用户配置（在数组中添加多个 Cookie 字符串）

**安全提示**: 
- `config.json` 文件包含敏感信息，请不要将其提交到版本控制系统
- `.gitignore` 文件已包含 `config.json`，确保不会被意外提交
- 请妥善保管您的 Cookie，不要分享给他人

## 💻 CLI 工具使用

### 基本用法

```bash
kuake <command> [config.json] [arguments...]
```

### 可用命令

| 命令 | 说明 | 示例 |
|------|------|------|
| `user-info` | 获取用户信息 | `kuake user-info` |
| `upload <file> <dest>` | 上传文件（上传进度输出到 stderr） | `kuake upload file.txt /file.txt` |
| `list [path]` | 列出目录内容（默认: /） | `kuake list /` |
| `info <path>` | 获取文件/文件夹信息 | `kuake info /file.txt` |
| `create <name> <pdir>` | 创建文件夹（pdir 为父目录 FID，根目录使用 "0"） | `kuake create test_folder 0` |
| `move <src> <dest>` | 移动文件/文件夹 | `kuake move /file.txt /folder` |
| `copy <src> <dest>` | 复制文件/文件夹 | `kuake copy /file.txt /folder` |
| `rename <path> <newName>` | 重命名文件/文件夹 | `kuake rename /file.txt new_name.txt` |
| `delete <path>` | 删除文件/文件夹 | `kuake delete /file.txt` |
| `share <path> <days> <passcode>` | 创建分享链接 | `kuake share /file.txt 7 false` |
| `help` | 显示帮助信息 | `kuake help` |

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
# 获取用户信息（使用默认配置文件）
kuake user-info

# 获取用户信息（使用自定义配置文件）
kuake user-info custom.json

# 上传文件
kuake upload file.txt /file.txt

# 列出根目录
kuake list /

# 创建文件夹
kuake create test_folder 0

# 移动文件
kuake move /file.txt /folder

# 创建分享链接（7天，不需要提取码）
kuake share /file.txt 7 false

# 创建分享链接（30天，需要提取码，使用自定义配置文件）
kuake share /file.txt 30 true custom.json
```

## ⚠️ 注意事项

- **仅提供二进制文件**：本项目不提供源代码，仅提供编译好的二进制可执行文件
- 所有操作都通过夸克网盘 API 进行
- 需要有效的 Cookie（access_token）才能使用
- 上传操作支持进度显示（输出到 stderr）
- 删除目录会递归删除所有子文件和子目录
- CLI 工具的所有结果以 JSON 格式输出到 stdout，方便其他进程解析
- 上传进度、帮助信息和序列化错误输出到 stderr，不会混入 JSON 输出
- 配置文件参数是可选的，不提供时使用默认的 `config.json`
- 配置文件参数必须是 `.json` 扩展名，且放在命令之后、其他参数之前
- 成功时退出码为 0，失败时为 1

## ⚖️ 免责声明

**重要提示**：

1. **非官方工具**: 本项目是一个非官方的第三方 CLI 工具，与夸克网盘官方无关。本项目仅用于学习和研究目的。

2. **API 变更风险**: 夸克网盘可能会随时更改其 API，这可能导致本工具无法正常工作。作者不保证工具的持续可用性。

3. **使用风险**: 使用本工具进行文件操作时，请自行承担以下风险：
   - 数据丢失或损坏
   - 账号被封禁或限制
   - API 调用失败
   - 其他不可预见的后果

4. **Cookie 安全**: 
   - 请妥善保管您的 Cookie，不要分享给他人
   - 不要在公共场合或不可信的环境中运行本工具
   - 定期更换 Cookie 以确保账号安全

5. **服务条款**: 使用本工具即表示您同意遵守夸克网盘的服务条款。任何违反服务条款的行为（如滥用 API、批量操作等）可能导致账号被封禁。

6. **免责**: 
   - 作者不对使用本工具造成的任何损失承担责任
   - 作者不对因 API 变更导致的工具失效承担责任
   - 作者不对因使用本工具导致的账号问题承担责任

7. **建议**: 
   - 建议在生产环境使用前进行充分测试
   - 建议定期备份重要数据
   - 建议遵守夸克网盘的使用规范

**使用本工具即表示您已阅读、理解并同意上述免责声明。**

## 📄 许可证

本项目采用 MIT 许可证。详情请参阅 [LICENSE](LICENSE) 文件。

---

**注意**：本项目仅提供二进制文件，不提供源代码。如需源代码，请联系项目维护者。

---

**Made with ❤️ by the community**
