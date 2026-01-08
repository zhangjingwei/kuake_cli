package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"kuake_sdk/sdk"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	ExitSuccess = 0
	ExitError   = 1
)

// Version 版本号
var Version = "v1.3.4"

type CLIResult struct {
	Success bool                   `json:"success"`
	Code    string                 `json:"code,omitempty"`
	Message string                 `json:"message,omitempty"`
	Data    map[string]interface{} `json:"data,omitempty"`
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(ExitError)
	}

	command := os.Args[1]
	args := os.Args[2:]

	// 解析全局配置
	configPath := sdk.DEFAULT_CONFIG_PATH
	if len(args) > 0 && filepath.Ext(args[0]) == ".json" {
		configPath = args[0]
		args = args[1:]
	}

	// 创建客户端
	var client *sdk.QuarkClient
	defer func() {
		if r := recover(); r != nil {
			outputJSON(&CLIResult{
				Success: false,
				Code:    "INIT_ERROR",
				Message: fmt.Sprintf("Failed to initialize client: %v", r),
			})
			os.Exit(ExitError)
		}
	}()
	client = sdk.NewQuarkClient(configPath)

	// 执行命令
	var result *CLIResult
	switch command {
	case "user":
		result = handleUserInfo(client)
	case "list":
		result = handleList(client, args)
	case "info":
		result = handleInfo(client, args)
	case "download":
		result = handleDownloadURL(client, args)
	case "upload":
		result = handleUpload(client, args)
	case "create":
		result = handleCreateFolder(client, args)
	case "move":
		result = handleMove(client, args)
	case "copy":
		result = handleCopy(client, args)
	case "rename":
		result = handleRename(client, args)
	case "delete":
		result = handleDelete(client, args)
	case "share":
		result = handleShareCreate(client, args)
	case "share-delete":
		result = handleShareDelete(client, args)
	case "help", "-h", "--help":
		printUsage()
		os.Exit(ExitSuccess)
	default:
		result = &CLIResult{
			Success: false,
			Code:    "UNKNOWN_COMMAND",
			Message: fmt.Sprintf("Unknown command: %s", command),
		}
	}

	// 输出 JSON 结果
	outputJSON(result)

	// 根据结果设置退出码
	if !result.Success {
		os.Exit(ExitError)
	}
	os.Exit(ExitSuccess)
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Quark Cloud Drive CLI Tool

Usage:
  kuake <command> [config.json] [arguments...]

Commands:
  user                        Get user information
  list [path]                 List directory (default: "/")
  info <path>                 Get file/folder info
  download <path>             Get file download URL
  upload <file> <dest>        Upload file (all parameters must be quoted)
  create <name> <pdir>        Create folder (use "/" for root)
  move <src> <dest>           Move file/folder
  copy <src> <dest>           Copy file/folder
  rename <path> <newName>     Rename file/folder
  delete <path>               Delete file/folder
  share <path> <days> <passcode>  Create share link
                                days: 0=permanent, 1/7/30=days
                                passcode: "true" or "false"
  share-delete <share_id_or_path>...  Delete share(s) by share ID(s) or file path(s)
  help                           Show help

Examples:
  kuake user
  kuake list "/"
  kuake info "/file.txt"
  kuake download "/file.txt"
  kuake upload "file.txt" "/folder/file.txt"
  kuake create "folder" "/"
  kuake move "/file.txt" "/folder/"
  kuake share "/file.txt" 7 "false"
  kuake share-delete "fdd8bfd93f21491ab80122538bec310d"
  kuake share-delete "/file.txt"

Notes:
  - All path parameters must be quoted
  - Root directory is "/"
  - Results output as JSON to stdout
  - Exit code: 0=success, 1=failure
`)
}

func outputJSON(result *CLIResult) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false) // 禁用 HTML 转义，避免 < > 被转义为 \u003c \u003e
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to serialize result: %v\n", err)
		os.Exit(ExitError)
	}
	// Encode 会添加换行符，我们需要去掉它
	output := buf.String()
	if len(output) > 0 && output[len(output)-1] == '\n' {
		output = output[:len(output)-1]
	}
	fmt.Println(output)
}

// handleUserInfo 处理获取用户信息命令
func handleUserInfo(client *sdk.QuarkClient) *CLIResult {
	response, err := client.GetUserInfo()
	if err != nil {
		return &CLIResult{
			Success: false,
			Message: err.Error(),
		}
	}

	if !response.Success {
		return &CLIResult{
			Success: false,
			Code:    response.Code,
			Message: response.Message,
		}
	}

	return &CLIResult{
		Success: true,
		Code:    response.Code,
		Message: response.Message,
		Data:    response.Data,
	}
}

// handleUpload 处理上传文件命令
func handleUpload(client *sdk.QuarkClient, args []string) *CLIResult {
	if len(args) < 2 {
		return &CLIResult{
			Success: false,
			Code:    "INVALID_ARGS",
			Message: `Usage: upload <file> <dest> (all parameters must be quoted, e.g., upload 'file(1).txt' '/dest/file.txt')`,
		}
	}

	filePath := args[0]
	destPath := args[1]

	// 可选：进度回调（静默模式，不输出进度）
	progressCallback := func(progress int) {
		// CLI 模式下可以静默，或者输出到 stderr
		fmt.Fprintf(os.Stderr, "\rUpload progress: %d%%", progress)
		if progress == 100 {
			fmt.Fprintf(os.Stderr, "\n")
		}
	}

	response, err := client.UploadFile(filePath, destPath, progressCallback)
	if err != nil {
		return &CLIResult{
			Success: false,
			Message: err.Error(),
		}
	}

	if !response.Success {
		return &CLIResult{
			Success: false,
			Code:    response.Code,
			Message: response.Message,
		}
	}

	return &CLIResult{
		Success: true,
		Code:    response.Code,
		Message: response.Message,
		Data:    response.Data,
	}
}

// handleList 处理列出目录命令
func handleList(client *sdk.QuarkClient, args []string) *CLIResult {
	dirPath := "/"
	if len(args) > 0 {
		dirPath = args[0]
	}

	response, err := client.List(dirPath)
	if err != nil {
		return &CLIResult{
			Success: false,
			Message: err.Error(),
		}
	}

	if !response.Success {
		return &CLIResult{
			Success: false,
			Code:    response.Code,
			Message: response.Message,
		}
	}

	return &CLIResult{
		Success: true,
		Code:    response.Code,
		Message: response.Message,
		Data:    response.Data,
	}
}

// handleInfo 处理获取文件信息命令
func handleInfo(client *sdk.QuarkClient, args []string) *CLIResult {
	if len(args) < 1 {
		return &CLIResult{
			Success: false,
			Code:    "INVALID_ARGS",
			Message: `Usage: info <path> (path must be quoted, e.g., info 'file(1).txt')`,
		}
	}

	path := args[0]
	response, err := client.GetFileInfo(path)
	if err != nil {
		return &CLIResult{
			Success: false,
			Message: err.Error(),
		}
	}

	if !response.Success {
		return &CLIResult{
			Success: false,
			Code:    response.Code,
			Message: response.Message,
		}
	}

	return &CLIResult{
		Success: true,
		Code:    response.Code,
		Message: response.Message,
		Data:    response.Data,
	}
}

// handleCreateFolder 处理创建文件夹命令
func handleCreateFolder(client *sdk.QuarkClient, args []string) *CLIResult {
	if len(args) < 2 {
		return &CLIResult{
			Success: false,
			Code:    "INVALID_ARGS",
			Message: `Usage: create <name> <pdir> (all parameters must be quoted, e.g., create 'folder(1)' '/')`,
		}
	}

	folderName := args[0]
	pdirArg := args[1]

	// 处理父目录参数：如果是路径（以 / 开头），需要转换为 FID
	var pdirFid string
	if pdirArg == "" || pdirArg == "/" {
		pdirFid = "/" // 根目录使用标准表示 "/"，SDK 会自动转换为 "0"
	} else if strings.HasPrefix(pdirArg, "/") {
		// 是路径字符串，需要转换为 FID
		dirInfo, err := client.GetFileInfo(pdirArg)
		if err != nil {
			return &CLIResult{
				Success: false,
				Code:    "GET_PARENT_DIRECTORY_ERROR",
				Message: fmt.Sprintf("failed to get parent directory info: %v", err),
			}
		}
		if !dirInfo.Success {
			return &CLIResult{
				Success: false,
				Code:    dirInfo.Code,
				Message: fmt.Sprintf("failed to get parent directory: %s", dirInfo.Message),
			}
		}
		// 安全地获取 fid
		fid, ok := dirInfo.Data["fid"].(string)
		if !ok || fid == "" {
			return &CLIResult{
				Success: false,
				Code:    "INVALID_PARENT_DIRECTORY",
				Message: "parent directory info is invalid: fid not found or empty",
			}
		}
		pdirFid = fid
	} else {
		// 假设是 FID（不是以 / 开头的字符串）
		pdirFid = pdirArg
	}

	response, err := client.CreateFolder(folderName, pdirFid)
	if err != nil {
		return &CLIResult{
			Success: false,
			Message: err.Error(),
		}
	}

	if !response.Success {
		return &CLIResult{
			Success: false,
			Code:    response.Code,
			Message: response.Message,
		}
	}

	return &CLIResult{
		Success: true,
		Code:    response.Code,
		Message: response.Message,
		Data:    response.Data,
	}
}

// handleMove 处理移动命令
func handleMove(client *sdk.QuarkClient, args []string) *CLIResult {
	if len(args) < 2 {
		return &CLIResult{
			Success: false,
			Code:    "INVALID_ARGS",
			Message: `Usage: move <src> <dest> (all parameters must be quoted, e.g., move 'file(1).txt' '/dest/')`,
		}
	}

	srcPath := args[0]
	destPath := args[1]

	response, err := client.Move(srcPath, destPath)
	if err != nil {
		return &CLIResult{
			Success: false,
			Message: err.Error(),
		}
	}

	if !response.Success {
		return &CLIResult{
			Success: false,
			Code:    response.Code,
			Message: response.Message,
		}
	}

	return &CLIResult{
		Success: true,
		Code:    response.Code,
		Message: response.Message,
		Data:    response.Data,
	}
}

// handleCopy 处理复制命令
func handleCopy(client *sdk.QuarkClient, args []string) *CLIResult {
	if len(args) < 2 {
		return &CLIResult{
			Success: false,
			Code:    "INVALID_ARGS",
			Message: `Usage: copy <src> <dest> (all parameters must be quoted, e.g., copy 'file(1).txt' '/dest/')`,
		}
	}

	srcPath := args[0]
	destPath := args[1]

	response, err := client.Copy(srcPath, destPath)
	if err != nil {
		return &CLIResult{
			Success: false,
			Message: err.Error(),
		}
	}

	if !response.Success {
		return &CLIResult{
			Success: false,
			Code:    response.Code,
			Message: response.Message,
		}
	}

	return &CLIResult{
		Success: true,
		Code:    response.Code,
		Message: response.Message,
		Data:    response.Data,
	}
}

// handleRename 处理重命名命令
func handleRename(client *sdk.QuarkClient, args []string) *CLIResult {
	if len(args) < 2 {
		return &CLIResult{
			Success: false,
			Code:    "INVALID_ARGS",
			Message: `Usage: rename <path> <newName> (all parameters must be quoted, e.g., rename 'file(1).txt' 'new_name.txt')`,
		}
	}

	path := args[0]
	newName := args[1]

	response, err := client.Rename(path, newName)
	if err != nil {
		return &CLIResult{
			Success: false,
			Message: err.Error(),
		}
	}

	if !response.Success {
		return &CLIResult{
			Success: false,
			Code:    response.Code,
			Message: response.Message,
		}
	}

	return &CLIResult{
		Success: true,
		Code:    response.Code,
		Message: response.Message,
		Data:    response.Data,
	}
}

// handleDelete 处理删除命令
func handleDelete(client *sdk.QuarkClient, args []string) *CLIResult {
	if len(args) < 1 {
		return &CLIResult{
			Success: false,
			Code:    "INVALID_ARGS",
			Message: `Usage: delete <path> (path must be quoted, e.g., delete 'file(1).txt')`,
		}
	}

	path := args[0]
	response, err := client.Delete(path)
	if err != nil {
		return &CLIResult{
			Success: false,
			Message: err.Error(),
		}
	}

	if !response.Success {
		return &CLIResult{
			Success: false,
			Code:    response.Code,
			Message: response.Message,
		}
	}

	return &CLIResult{
		Success: true,
		Code:    response.Code,
		Message: response.Message,
		Data:    response.Data,
	}
}

// handleShareCreate 处理创建分享链接命令
func handleShareCreate(client *sdk.QuarkClient, args []string) *CLIResult {
	if len(args) < 3 {
		return &CLIResult{
			Success: false,
			Code:    "INVALID_ARGS",
			Message: "Usage: share <path> <days> <passcode> (path and passcode must be quoted, e.g., share \"file(1).txt\" 7 \"false\")",
		}
	}

	path := args[0]

	// 解析有效期天数（必传）
	expireDays, err := strconv.Atoi(args[1])
	if err != nil {
		return &CLIResult{
			Success: false,
			Code:    "INVALID_ARGS",
			Message: "days must be a number",
		}
	}

	// 解析是否需要提取码（必传）
	passcodeArg := args[2]
	var needPasscode bool
	switch passcodeArg {
	case "true":
		needPasscode = true
	case "false":
		needPasscode = false
	default:
		return &CLIResult{
			Success: false,
			Code:    "INVALID_ARGS",
			Message: "passcode must be 'true' or 'false'",
		}
	}

	shareInfo, err := client.CreateShare(path, expireDays, needPasscode)
	if err != nil {
		return &CLIResult{
			Success: false,
			Message: err.Error(),
		}
	}

	data := map[string]interface{}{
		"share_url":  shareInfo.ShareURL,
		"pwd_id":     shareInfo.PwdID,
		"passcode":   shareInfo.Passcode,
		"expires_at": shareInfo.ExpiresAt,
	}

	if shareInfo.ExpiresAt > 0 {
		expireTime := time.Unix(shareInfo.ExpiresAt/1000, 0)
		data["expires_at_formatted"] = expireTime.Format("2006-01-02 15:04:05")
	}

	return &CLIResult{
		Success: true,
		Code:    "OK",
		Message: "Share link created successfully",
		Data:    data,
	}
}

// handleDownloadURL 处理获取下载链接命令
func handleDownloadURL(client *sdk.QuarkClient, args []string) *CLIResult {
	if len(args) < 1 {
		return &CLIResult{
			Success: false,
			Code:    "INVALID_ARGS",
			Message: `Usage: download <path> (path must be quoted, e.g., download 'file(1).txt')`,
		}
	}

	path := args[0]

	// 先获取文件信息以获取文件ID
	fileInfo, err := client.GetFileInfo(path)
	if err != nil {
		return &CLIResult{
			Success: false,
			Message: fmt.Sprintf("failed to get file info: %v", err),
		}
	}

	if !fileInfo.Success {
		return &CLIResult{
			Success: false,
			Code:    fileInfo.Code,
			Message: fileInfo.Message,
		}
	}

	// 获取文件ID
	fid, ok := fileInfo.Data["fid"].(string)
	if !ok || fid == "" {
		return &CLIResult{
			Success: false,
			Code:    "INVALID_FILE_INFO",
			Message: "file info does not contain valid fid",
		}
	}

	// 检查是否为目录
	isDir, _ := fileInfo.Data["dir"].(bool)
	if isDir {
		return &CLIResult{
			Success: false,
			Code:    "INVALID_FILE_TYPE",
			Message: "cannot get download URL for directory",
		}
	}

	// 获取下载链接
	downloadURL, err := client.GetDownloadURL(fid)
	if err != nil {
		return &CLIResult{
			Success: false,
			Message: fmt.Sprintf("failed to get download URL: %v", err),
		}
	}

	data := map[string]interface{}{
		"fid":          fid,
		"path":         path,
		"download_url": downloadURL,
	}

	return &CLIResult{
		Success: true,
		Code:    "OK",
		Message: "Download URL retrieved successfully",
		Data:    data,
	}
}

// handleShareDelete 处理取消分享命令
// 支持两种方式：
// 1. 直接提供 share_id: share-delete "fdd8bfd93f21491ab80122538bec310d"
// 2. 提供文件路径: share-delete "/file.txt" (会先获取文件信息，然后从分享列表中查找share_id)
func handleShareDelete(client *sdk.QuarkClient, args []string) *CLIResult {
	if len(args) < 1 {
		return &CLIResult{
			Success: false,
			Code:    "INVALID_ARGS",
			Message: `Usage: share-delete <share_id_or_path> [share_id_or_path2] ... (e.g., share-delete "fdd8bfd93f21491ab80122538bec310d" or share-delete "/file.txt")`,
		}
	}

	var shareIDs []string
	var paths []string

	// 区分 share_id 和文件路径
	// share_id 通常是32位十六进制字符串，不以 "/" 开头
	// 文件路径通常以 "/" 开头
	for _, arg := range args {
		if strings.HasPrefix(arg, "/") {
			// 是文件路径
			paths = append(paths, arg)
		} else {
			// 假设是 share_id
			shareIDs = append(shareIDs, arg)
		}
	}

	// 处理文件路径：获取文件信息，然后从分享列表中查找share_id
	if len(paths) > 0 {
		for _, path := range paths {
			// 获取文件信息
			fileInfo, err := client.GetFileInfo(path)
			if err != nil {
				return &CLIResult{
					Success: false,
					Code:    "GET_FILE_INFO_ERROR",
					Message: fmt.Sprintf("failed to get file info for path '%s': %v", path, err),
				}
			}

			if !fileInfo.Success {
				return &CLIResult{
					Success: false,
					Code:    fileInfo.Code,
					Message: fmt.Sprintf("failed to get file info for path '%s': %s", path, fileInfo.Message),
				}
			}

			// 获取fid
			fid, ok := fileInfo.Data["fid"].(string)
			if !ok || fid == "" {
				return &CLIResult{
					Success: false,
					Code:    "INVALID_FILE_INFO",
					Message: fmt.Sprintf("file '%s' does not have valid fid", path),
				}
			}

			// 从分享列表中查找share_id
			shareID, err := client.GetShareIDByFid(fid)
			if err != nil {
				return &CLIResult{
					Success: false,
					Code:    "GET_SHARE_ID_ERROR",
					Message: fmt.Sprintf("failed to get share_id for file '%s' (fid: %s): %v. The file may not be shared.", path, fid, err),
				}
			}

			shareIDs = append(shareIDs, shareID)
		}
	}

	// 如果没有找到任何 share_id，返回错误
	if len(shareIDs) == 0 {
		return &CLIResult{
			Success: false,
			Code:    "NO_SHARE_IDS",
			Message: "no valid share_ids found. Please provide share_id(s) or file path(s) with active shares.",
		}
	}

	// 删除分享
	err := client.DeleteShare(shareIDs)
	if err != nil {
		return &CLIResult{
			Success: false,
			Message: err.Error(),
		}
	}

	resultData := map[string]interface{}{
		"deleted_share_ids": shareIDs,
	}
	if len(paths) > 0 {
		resultData["processed_paths"] = paths
	}

	return &CLIResult{
		Success: true,
		Code:    "OK",
		Message: "Share deleted successfully",
		Data:    resultData,
	}
}
