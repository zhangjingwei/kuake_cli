# SDK 测试说明

本目录包含了 kuake_sdk 的所有单元测试用例。

## 测试文件结构

- `config_test.go` - 配置文件加载和保存的测试
- `quark_client_test.go` - 客户端初始化和基础方法的测试
- `user_test.go` - 用户信息获取的测试
- `file_test.go` - 文件操作（上传、下载、创建、删除、移动、复制、重命名）的测试
- `share_test.go` - 分享相关功能的测试
- `hash_ctx_test.go` - X-Oss-Hash-Ctx 功能的单元测试
- `upload_hash_ctx_integration_test.go` - X-Oss-Hash-Ctx 功能的集成测试

## 运行测试

### 运行所有测试

```bash
go test ./sdk -v
```

### 运行特定包的测试

```bash
# 运行配置相关测试
go test ./sdk -v -run TestLoadConfig

# 运行客户端相关测试
go test ./sdk -v -run TestNewQuarkClient

# 运行文件操作测试
go test ./sdk -v -run TestCreateFolder
```

### 运行特定测试函数

```bash
go test ./sdk -v -run TestNormalizeRootDir
```

## 测试说明

### 单元测试 vs 集成测试

大部分测试用例标记为 `t.Skip()`，因为它们需要：
1. 真实的网络连接
2. 有效的配置文件
3. 有效的访问令牌

这些测试主要用于：
- **单元测试**：测试不依赖外部资源的函数（如 `normalizeRootDir`, `parseCookie`, `ConvertToFileInfo`）
- **集成测试**：需要真实API调用的测试（如 `GetUserInfo`, `UploadFile`, `CreateShare`）

### 运行集成测试

要运行集成测试，需要：

1. 准备有效的配置文件 `config.json`：
```json
{
  "Quark": {
    "access_tokens": [
      "your_valid_token_here"
    ]
  }
}
```

2. 移除测试中的 `t.Skip()` 调用

3. 运行测试：
```bash
go test ./sdk -v
```

## 测试覆盖率

查看测试覆盖率：

```bash
go test ./sdk -cover
```

生成详细的覆盖率报告：

```bash
go test ./sdk -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## 测试用例列表

### config_test.go
- `TestLoadConfig` - 测试配置文件加载
- `TestSaveConfig` - 测试配置文件保存
- `TestLoadConfig_DefaultPath` - 测试默认路径加载

### quark_client_test.go
- `TestNewQuarkClient` - 测试客户端创建
- `TestSetBaseURL` - 测试设置基础URL
- `TestGetCookies` - 测试获取Cookies
- `TestParseCookie` - 测试Cookie解析
- `TestParseResponse` - 测试响应解析
- `TestConvertToFileInfo` - 测试文件信息转换

### user_test.go
- `TestGetUserInfo` - 测试获取用户信息
- `TestGetUserInfo_ErrorHandling` - 测试错误处理

### file_test.go
- `TestNormalizeRootDir` - 测试根目录标准化
- `TestCreateFolder` - 测试创建文件夹
- `TestList` - 测试列出文件
- `TestGetFileInfo` - 测试获取文件信息
- `TestDelete` - 测试删除文件
- `TestMove` - 测试移动文件
- `TestCopy` - 测试复制文件
- `TestRename` - 测试重命名文件
- `TestUploadFile` - 测试上传文件

### share_test.go
- `TestGetShareInfo` - 测试获取分享信息
- `TestCreateShare` - 测试创建分享链接
- `TestGetShareLink` - 测试获取分享链接
- `TestGetShareStoken` - 测试获取分享token
- `TestGetShareList` - 测试获取分享列表
- `TestSaveShareFile` - 测试保存分享文件
- `TestSetSharePassword` - 测试设置分享密码

### hash_ctx_test.go
- `TestEncodeHashCtx` - 测试 HashCtx 编码功能
- `TestEncodeHashCtx_Nil` - 测试 nil HashCtx 处理
- `TestUpdateHashCtxFromHash` - 测试增量 SHA1 哈希上下文更新
- `TestUpdateHashCtxFromHash_Incremental` - 测试增量哈希计算的连续性
- `TestUpdateHashCtxFromHash_Consistency` - 测试一致性
- `TestEncodeDecodeRoundTrip` - 测试编码-解码往返转换
- `TestHashCtx_RealWorldExample` - 使用真实世界数据测试

### upload_hash_ctx_integration_test.go
- `TestUploadFileHashCtx_Integration` - 实际上传测试（需要网络）
- `TestUploadFileHashCtx_Resume` - 断点续传测试（需要网络）
- `TestUploadStateHashCtx` - 状态保存/加载测试
- `TestUploadStateHashCtx_Nil` - nil HashCtx 处理测试

## 注意事项

1. **网络依赖**：大部分测试需要网络连接和有效的API凭证
2. **配置文件**：确保有有效的配置文件才能运行集成测试
3. **测试数据**：某些测试可能会创建或删除实际的文件/文件夹
4. **速率限制**：注意API的速率限制，避免频繁调用

## X-Oss-Hash-Ctx 测试

### 运行单元测试（不需要网络）

```bash
go test ./sdk -v -run "Test.*HashCtx"
```

### 运行集成测试（需要网络和有效配置）

```bash
# Windows PowerShell
$env:INTEGRATION_TEST="1"
go test ./sdk -v -run "TestUploadFileHashCtx_Integration"

# Linux/Mac
INTEGRATION_TEST=1 go test ./sdk -v -run "TestUploadFileHashCtx_Integration"
```

### 验证要点

- partNumber=1 的 PUT 请求**不包含** X-Oss-Hash-Ctx
- partNumber>=2 的 PUT 请求**包含** X-Oss-Hash-Ctx
- HashCtx 值正确递增（Nl 字段递增）
- 断点续传时 HashCtx 正确保存和恢复

## 贡献

添加新测试时，请遵循以下原则：

1. 测试函数名以 `Test` 开头
2. 使用表驱动测试（table-driven tests）提高覆盖率
3. 为需要外部资源的测试添加 `t.Skip()` 或适当的条件检查
4. 确保测试可以独立运行，不依赖执行顺序
5. 清理测试中创建的资源（临时文件等）

