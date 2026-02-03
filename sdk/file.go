package sdk

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// getUploadStatePath 获取上传状态文件路径
func getUploadStatePath(filePath, destPath string) string {
	// 基于文件路径和目标路径生成唯一的状态文件路径
	hash := md5.Sum([]byte(filePath + "|" + destPath))
	hashStr := fmt.Sprintf("%x", hash)
	stateDir := filepath.Join(os.TempDir(), "kuake_upload_state")
	os.MkdirAll(stateDir, 0755)
	return filepath.Join(stateDir, hashStr+".json")
}

// loadUploadState 加载上传状态
func loadUploadState(statePath string) (*UploadState, error) {
	data, err := os.ReadFile(statePath)
	if err != nil {
		return nil, err
	}
	var state UploadState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// saveUploadState 保存上传状态
func saveUploadState(statePath string, state *UploadState) error {
	state.CreatedAt = time.Now()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(statePath, data, 0644)
}

// deleteUploadState 删除上传状态文件
func deleteUploadState(statePath string) error {
	return os.Remove(statePath)
}

// formatSpeed 格式化速度字符串
func formatSpeed(speed float64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)

	if speed >= GB {
		return fmt.Sprintf("%.2f GB/s", speed/GB)
	} else if speed >= MB {
		return fmt.Sprintf("%.2f MB/s", speed/MB)
	} else if speed >= KB {
		return fmt.Sprintf("%.2f KB/s", speed/KB)
	}
	return fmt.Sprintf("%.0f B/s", speed)
}

// formatDuration 格式化时间间隔字符串
func formatDuration(d time.Duration) string {
	if d < 0 {
		return "0s"
	}

	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if hours > 0 {
		return fmt.Sprintf("%dh%dm%ds", hours, minutes, seconds)
	} else if minutes > 0 {
		return fmt.Sprintf("%dm%ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

// normalizePath 将路径标准化为 Unix 风格（使用 / 作为分隔符）
func normalizePath(path string) string {
	path = strings.ReplaceAll(path, "\\", "/")
	for strings.Contains(path, "//") {
		path = strings.ReplaceAll(path, "//", "/")
	}
	if len(path) > 1 && strings.HasSuffix(path, "/") {
		path = strings.TrimSuffix(path, "/")
	}
	return path
}

// normalizeRootDir 将根目录路径转换为 API 所需的 FID "0"
func normalizeRootDir(path string) string {
	path = normalizePath(path)
	if path == "" || path == "/" || path == "." {
		return "0"
	}
	return path
}

// upPre 预上传请求
func (qc *QuarkClient) upPre(fileName, mimeType string, size int64, parentID string) (*PreUploadResponse, error) {
	now := time.Now().UnixMilli()
	data := map[string]interface{}{
		"ccp_hash_update": true,
		"dir_name":        "",
		"file_name":       fileName,
		"format_type":     mimeType,
		"l_created_at":    now,
		"l_updated_at":    now,
		"pdir_fid":        parentID,
		"size":            size,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal pre-upload data: %w", err)
	}

	respMap, err := qc.makeRequest("POST", FILE_UPLOAD_PRE, bytes.NewBuffer(jsonData), nil)
	if err != nil {
		return nil, fmt.Errorf("pre-upload request failed: %w", err)
	}

	var preResp PreUploadResponse
	if err := qc.parseResponse(respMap, &preResp); err != nil {
		return nil, fmt.Errorf("failed to decode pre-upload response: %w", err)
	}

	if preResp.Code != 0 || preResp.Status != 200 {
		return nil, fmt.Errorf("pre-upload failed: code=%d, status=%d", preResp.Code, preResp.Status)
	}

	return &preResp, nil
}

// upHash 提交文件哈希验证
func (qc *QuarkClient) upHash(md5Hash, sha1Hash, taskID string) (*HashResponse, error) {
	data := map[string]interface{}{
		"md5":     md5Hash,
		"sha1":    sha1Hash,
		"task_id": taskID,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal hash data: %w", err)
	}

	respMap, err := qc.makeRequest("POST", FILE_UPDATE_HASH, bytes.NewBuffer(jsonData), nil)
	if err != nil {
		return nil, fmt.Errorf("hash update request failed: %w", err)
	}

	var hashResp HashResponse
	if err := qc.parseResponse(respMap, &hashResp); err != nil {
		return nil, fmt.Errorf("failed to decode hash response: %w", err)
	}

	if hashResp.Code != 0 || hashResp.Status != 200 {
		return nil, fmt.Errorf("hash update failed: code=%d, status=%d", hashResp.Code, hashResp.Status)
	}

	return &hashResp, nil
}

// upPart 上传文件分片
func (qc *QuarkClient) upPart(pre *PreUploadResponse, mimeType string, partNumber int, chunkData []byte) (string, error) {
	now := time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")
	authMeta := fmt.Sprintf("PUT\n\n%s\n%s\nx-oss-date:%s\nx-oss-user-agent:aliyun-sdk-js/6.6.1 Chrome 98.0.4758.80 on Windows 10 64-bit\n/%s/%s?partNumber=%d&uploadId=%s",
		mimeType, now, now, pre.Data.Bucket, pre.Data.ObjKey, partNumber, pre.Data.UploadID)

	// 使用 client 方法获取 Authorization
	authKey, err := qc.getOSSAuthKey(authMeta, pre.Data.AuthInfo, pre.Data.TaskID)
	if err != nil {
		return "", err
	}

	// 构建上传 URL
	uploadURLBase := pre.Data.UploadURL
	if strings.HasPrefix(uploadURLBase, "https://") {
		uploadURLBase = uploadURLBase[8:] // 去掉 "https://"
	} else if strings.HasPrefix(uploadURLBase, "http://") {
		uploadURLBase = uploadURLBase[7:] // 去掉 "http://"
	}
	uploadURL := fmt.Sprintf("https://%s.%s/%s",
		pre.Data.Bucket,
		uploadURLBase,
		pre.Data.ObjKey)

	// 使用统一的请求创建方法
	headerBuilder := &OSSPartUploadHeaderBuilder{
		AuthKey:   authKey,
		MimeType:  mimeType,
		Timestamp: now,
	}
	req, err := qc.newRequestWithHeaders("PUT", uploadURL, bytes.NewReader(chunkData), headerBuilder)
	if err != nil {
		return "", fmt.Errorf("failed to create upload request: %w", err)
	}

	params := req.URL.Query()
	params.Set("partNumber", fmt.Sprintf("%d", partNumber))
	params.Set("uploadId", pre.Data.UploadID)
	req.URL.RawQuery = params.Encode()

	// 为上传请求设置较长的超时时间（30分钟），主要依赖服务器端响应
	// 这个超时仅作为安全网，防止网络问题导致的永久挂起
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	req = req.WithContext(ctx)

	// 发送请求
	resp, err := qc.HttpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to upload chunk: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		bodyStr := string(bodyBytes)

		// 处理 PartAlreadyExist 错误（409）：分片已存在，从响应中提取 ETag
		if resp.StatusCode == 409 && strings.Contains(bodyStr, "PartAlreadyExist") {
			// 解析 XML 响应，提取 PartEtag
			type OSSError struct {
				XMLName    xml.Name `xml:"Error"`
				Code       string   `xml:"Code"`
				Message    string   `xml:"Message"`
				PartEtag   string   `xml:"PartEtag"`
				PartNumber string   `xml:"PartNumber"`
			}
			var ossErr OSSError
			if err := xml.Unmarshal(bodyBytes, &ossErr); err == nil && ossErr.PartEtag != "" {
				// 移除 ETag 两端的引号（如果有）
				etag := strings.Trim(ossErr.PartEtag, "\"")
				return etag, nil
			}
		}

		return "", fmt.Errorf("upload chunk failed with status %d: %s", resp.StatusCode, bodyStr)
	}

	// 从响应头获取 ETag
	etag := resp.Header.Get("ETag")
	if etag == "" {
		return "", fmt.Errorf("no ETag in response")
	}

	return etag, nil
}

// upCommit 提交上传（完成分片上传）
func (qc *QuarkClient) upCommit(pre *PreUploadResponse, etags []string) (*FinishResponse, error) {
	// 构建 XML body
	xmlParts := make([]string, len(etags))
	for i, etag := range etags {
		xmlParts[i] = fmt.Sprintf("<Part><PartNumber>%d</PartNumber><ETag>%s</ETag></Part>", i+1, etag)
	}
	xmlBody := fmt.Sprintf("<CompleteMultipartUpload>%s</CompleteMultipartUpload>", strings.Join(xmlParts, ""))

	// 计算 Content-MD5
	hash := md5.Sum([]byte(xmlBody))
	contentMD5 := base64.StdEncoding.EncodeToString(hash[:])

	// 获取 callback（从 pre.Data.Callback 中解析）
	var callbackB64 string
	var callbackObj map[string]interface{}
	if err := json.Unmarshal(pre.Data.Callback, &callbackObj); err == nil {
		// callback 是对象，需要序列化
		callbackJSON, _ := json.Marshal(callbackObj)
		callbackB64 = base64.StdEncoding.EncodeToString(callbackJSON)
	} else {
		// callback 可能是字符串，直接使用
		callbackB64 = base64.StdEncoding.EncodeToString(pre.Data.Callback)
	}

	now := time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")

	// 构建 auth_meta for commit
	authMeta := fmt.Sprintf("POST\n%s\napplication/xml\n%s\nx-oss-callback:%s\nx-oss-date:%s\nx-oss-user-agent:aliyun-sdk-js/6.6.1 Chrome 98.0.4758.80 on Windows 10 64-bit\n/%s/%s?uploadId=%s",
		contentMD5, now, callbackB64, now, pre.Data.Bucket, pre.Data.ObjKey, pre.Data.UploadID)

	// 使用 client 方法获取 Authorization
	authKey, err := qc.getOSSAuthKey(authMeta, pre.Data.AuthInfo, pre.Data.TaskID)
	if err != nil {
		return nil, err
	}

	// 构建上传 URL
	uploadURLBase := pre.Data.UploadURL
	if strings.HasPrefix(uploadURLBase, "https://") {
		uploadURLBase = uploadURLBase[8:]
	} else if strings.HasPrefix(uploadURLBase, "http://") {
		uploadURLBase = uploadURLBase[7:]
	}
	uploadURL := fmt.Sprintf("https://%s.%s/%s",
		pre.Data.Bucket,
		uploadURLBase,
		pre.Data.ObjKey)

	// 使用统一的请求创建方法
	headerBuilder := &OSSCommitHeaderBuilder{
		AuthKey:    authKey,
		ContentMD5: contentMD5,
		Callback:   callbackB64,
		Timestamp:  now,
	}
	req, err := qc.newRequestWithHeaders("POST", uploadURL, bytes.NewReader([]byte(xmlBody)), headerBuilder)
	if err != nil {
		return nil, fmt.Errorf("failed to create commit request: %w", err)
	}

	params := req.URL.Query()
	params.Set("uploadId", pre.Data.UploadID)
	req.URL.RawQuery = params.Encode()

	// 为提交上传请求设置较长的超时时间（5分钟），主要依赖服务器端响应
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	req = req.WithContext(ctx)

	// 发送请求
	commitResp, err := qc.HttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to commit upload: %w", err)
	}
	defer commitResp.Body.Close()

	if commitResp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(commitResp.Body)
		return nil, fmt.Errorf("commit upload failed with status %d: %s", commitResp.StatusCode, string(bodyBytes))
	}

	// 读取响应体
	bodyBytes, err := io.ReadAll(commitResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read commit response: %w", err)
	}

	// OSS CompleteMultipartUpload 成功时返回 XML 格式，不是 JSON
	// 如果 HTTP 状态码是 200，说明 OSS 上传成功
	// 返回一个表示成功的响应，实际的成功状态需要通过 upFinish 确认
	if commitResp.StatusCode == 200 {
		// OSS commit 成功，返回一个临时成功响应
		// 真正的成功需要通过 upFinish 确认
		return &FinishResponse{
			Code:   0,
			Status: 200,
			Data:   make(map[string]interface{}),
		}, nil
	}

	// 如果状态码不是 200，尝试解析错误响应
	return nil, fmt.Errorf("commit upload failed with status %d: %s", commitResp.StatusCode, string(bodyBytes))
}

// upFinish 完成上传流程
func (qc *QuarkClient) upFinish(pre *PreUploadResponse) (*FinishResponse, error) {
	data := map[string]interface{}{
		"obj_key": pre.Data.ObjKey,
		"task_id": pre.Data.TaskID,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal finish data: %w", err)
	}

	respMap, err := qc.makeRequest("POST", FILE_UPLOAD_FINISH, bytes.NewBuffer(jsonData), nil)
	if err != nil {
		return nil, fmt.Errorf("finish request failed: %w", err)
	}

	var finishResp FinishResponse
	if err := qc.parseResponse(respMap, &finishResp); err != nil {
		return nil, fmt.Errorf("failed to decode finish response: %w", err)
	}

	if finishResp.Code != 0 || finishResp.Status != 200 {
		return nil, fmt.Errorf("finish failed: code=%d, status=%d", finishResp.Code, finishResp.Status)
	}

	return &finishResp, nil
}

// UploadFile 上传文件到夸克网盘，支持大文件分片上传
// progressCallback: 进度回调函数，如果为 nil 则不显示进度
func (qc *QuarkClient) UploadFile(filePath, destPath string, progressCallback func(*UploadProgress)) (*StandardResponse, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return &StandardResponse{
			Success: false,
			Code:    "FILE_OPEN_ERROR",
			Message: fmt.Sprintf("failed to open file: %v", err),
			Data:    nil,
		}, nil
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return &StandardResponse{
			Success: false,
			Code:    "FILE_INFO_ERROR",
			Message: fmt.Sprintf("failed to get file info: %v", err),
			Data:    nil,
		}, nil
	}

	fileSize := fileInfo.Size()
	localFileName := fileInfo.Name()

	// 记录开始时间，用于计算速度和剩余时间
	startTime := time.Now()

	destPath = normalizePath(destPath)
	var destFileName string
	if strings.HasSuffix(destPath, "/") || filepath.Base(destPath) == "" || filepath.Base(destPath) == "." {
		destPath = strings.TrimSuffix(destPath, "/") + "/" + localFileName
		destFileName = localFileName
	} else {
		destFileName = filepath.Base(destPath)
	}

	destDirPath := destPath
	if destDirPath == "/" || destDirPath == "" {
		destDirPath = "/"
	} else {
		lastSlash := strings.LastIndex(destDirPath, "/")
		if lastSlash == 0 {
			destDirPath = "/"
		} else if lastSlash > 0 {
			destDirPath = destDirPath[:lastSlash]
		} else {
			destDirPath = "/"
		}
	}
	destDirPath = normalizePath(destDirPath)

	if destDirPath != "/" && destDirPath != "" && destDirPath != "." {
		destDirInfo, err := qc.GetFileInfo(destDirPath)
		needCreate := err != nil || (destDirInfo != nil && !destDirInfo.Success && destDirInfo.Code == "FILE_NOT_FOUND")
		if needCreate {
			parts := strings.Split(strings.Trim(destDirPath, "/"), "/")
			currentPath := ""
			var lastCreatedFid string // 记录最后创建的目录 FID
			for _, part := range parts {
				if part == "" {
					continue
				}
				if currentPath == "" {
					currentPath = "/" + part
				} else {
					currentPath = currentPath + "/" + part
				}
				currentPath = normalizePath(currentPath)
				checkInfo, err := qc.GetFileInfo(currentPath)
				needCreatePath := err != nil || (checkInfo != nil && !checkInfo.Success && checkInfo.Code == "FILE_NOT_FOUND")
				if needCreatePath {
					parentPathForCreate := "/"
					if currentPath != "/" && currentPath != "" {
						if lastSlash := strings.LastIndex(currentPath, "/"); lastSlash > 0 {
							parentPathForCreate = normalizePath(currentPath[:lastSlash])
						}
					}
					createResp, createErr := qc.CreateFolder(part, parentPathForCreate)
					if createErr != nil {
						return &StandardResponse{
							Success: false,
							Code:    "CREATE_DIRECTORY_ERROR",
							Message: fmt.Sprintf("failed to create directory %s: %v", currentPath, createErr),
							Data:    nil,
						}, nil
					}
					if createResp == nil || !createResp.Success {
						msg := "unknown error"
						if createResp != nil {
							msg = createResp.Message
						}
						return &StandardResponse{
							Success: false,
							Code:    "CREATE_DIRECTORY_ERROR",
							Message: fmt.Sprintf("failed to create directory %s: %s", currentPath, msg),
							Data:    nil,
						}, nil
					}
					// 如果创建成功，从返回的 Data 中获取 FID
					if createResp.Data != nil {
						if fid, ok := createResp.Data["fid"].(string); ok && fid != "" {
							lastCreatedFid = fid
						}
					}
				}
			}
			// 如果创建了目录并获取到了 FID，直接使用 FID，否则再次查询路径
			if lastCreatedFid != "" {
				destDirPath = lastCreatedFid
				destDirInfo = &StandardResponse{
					Success: true,
					Code:    "OK",
					Message: "Directory created",
					Data:    map[string]interface{}{"fid": lastCreatedFid},
				}
			} else {
				destDirInfo, err = qc.GetFileInfo(destDirPath)
				if err != nil {
					return &StandardResponse{
						Success: false,
						Code:    "GET_DIRECTORY_INFO_ERROR",
						Message: fmt.Sprintf("failed to get destination directory info: %v", err),
						Data:    nil,
					}, nil
				}
			}
		}
		if !destDirInfo.Success {
			return &StandardResponse{
				Success: false,
				Code:    destDirInfo.Code,
				Message: fmt.Sprintf("failed to get destination directory: %s", destDirInfo.Message),
				Data:    nil,
			}, nil
		}
		fid, ok := destDirInfo.Data["fid"].(string)
		if !ok || fid == "" {
			return &StandardResponse{
				Success: false,
				Code:    "INVALID_DIRECTORY_INFO",
				Message: "destination directory info is invalid: fid not found or empty",
				Data:    nil,
			}, nil
		}
		destDirPath = fid
	} else {
		destDirPath = "0"
	}

	mimeType := mime.TypeByExtension(filepath.Ext(destFileName))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	// 先检查是否有保存的上传状态（断点续传）
	statePath := getUploadStatePath(filePath, destPath)
	var savedState *UploadState
	var pre *PreUploadResponse
	var useSavedState bool

	// 尝试加载保存的上传状态
	if state, loadErr := loadUploadState(statePath); loadErr == nil {
		// 验证状态是否有效：文件路径、大小、目标路径是否匹配
		if state.FilePath == filePath && state.DestPath == destPath && state.FileSize == fileSize {
			// 尝试使用保存的状态，构建 PreUploadResponse
			pre = &PreUploadResponse{
				Code:   0,
				Status: 200,
			}
			pre.Data.TaskID = state.TaskID
			pre.Data.Bucket = state.Bucket
			pre.Data.ObjKey = state.ObjKey
			pre.Data.UploadID = state.UploadID
			pre.Data.UploadURL = state.UploadURL
			pre.Data.AuthInfo = state.AuthInfo
			pre.Data.Callback = state.Callback
			pre.Metadata.PartSize = state.PartSize

			// 验证 uploadId 是否仍然有效：尝试上传一个空分片或查询分片列表
			// 由于没有查询 API，我们直接尝试使用，如果失败再重新获取
			useSavedState = true
			savedState = state
		} else {
			// 文件不匹配，删除旧状态
			deleteUploadState(statePath)
		}
	}

	// 如果没有保存的状态或状态无效，调用 upPre 获取新的上传信息
	if !useSavedState {
		pre, err = qc.upPre(destFileName, mimeType, fileSize, destDirPath)
		if err != nil {
			return &StandardResponse{
				Success: false,
				Code:    "PRE_UPLOAD_ERROR",
				Message: fmt.Sprintf("pre-upload failed: %v", err),
				Data:    nil,
			}, nil
		}
	}

	file.Seek(0, 0)
	md5Hash := md5.New()
	sha1Hash := sha1.New()
	multiWriter := io.MultiWriter(md5Hash, sha1Hash)

	if _, err := io.Copy(multiWriter, file); err != nil {
		return &StandardResponse{
			Success: false,
			Code:    "CALCULATE_HASH_ERROR",
			Message: fmt.Sprintf("failed to calculate hash: %v", err),
			Data:    nil,
		}, nil
	}

	md5Sum := fmt.Sprintf("%x", md5Hash.Sum(nil))
	sha1Sum := fmt.Sprintf("%x", sha1Hash.Sum(nil))

	hashResp, err := qc.upHash(md5Sum, sha1Sum, pre.Data.TaskID)
	if err != nil {
		// 如果使用保存的状态但 hash 验证失败，可能是 taskId 已过期，重新获取
		if useSavedState {
			deleteUploadState(statePath)
			pre, err = qc.upPre(destFileName, mimeType, fileSize, destDirPath)
			if err != nil {
				return &StandardResponse{
					Success: false,
					Code:    "PRE_UPLOAD_ERROR",
					Message: fmt.Sprintf("pre-upload failed: %v", err),
					Data:    nil,
				}, nil
			}
			hashResp, err = qc.upHash(md5Sum, sha1Sum, pre.Data.TaskID)
			if err != nil {
				return &StandardResponse{
					Success: false,
					Code:    "HASH_VERIFICATION_ERROR",
					Message: fmt.Sprintf("hash verification failed: %v", err),
					Data:    nil,
				}, nil
			}
			useSavedState = false
		} else {
			return &StandardResponse{
				Success: false,
				Code:    "HASH_VERIFICATION_ERROR",
				Message: fmt.Sprintf("hash verification failed: %v", err),
				Data:    nil,
			}, nil
		}
	}

	if hashResp.Data.Finish {
		finish, err := qc.upFinish(pre)
		if err != nil {
			return &StandardResponse{
				Success: false,
				Code:    "FINISH_UPLOAD_ERROR",
				Message: fmt.Sprintf("finish upload failed: %v", err),
				Data:    nil,
			}, nil
		}
		if finish.Code != 0 || finish.Status != 200 {
			return &StandardResponse{
				Success: false,
				Code:    "FINISH_UPLOAD_ERROR",
				Message: fmt.Sprintf("finish upload failed: code=%d, status=%d", finish.Code, finish.Status),
				Data:    nil,
			}, nil
		}
		if progressCallback != nil {
			elapsed := time.Since(startTime)
			// 秒传：文件已存在于服务器，无需实际上传
			progressInfo := &UploadProgress{
				Progress:     100,
				Uploaded:     fileSize,
				Total:        fileSize,
				Speed:        0,
				SpeedStr:     "秒传（文件已存在）",
				Remaining:    0,
				RemainingStr: "0s",
				Elapsed:      elapsed,
			}
			progressCallback(progressInfo)
		}
		// 删除状态文件（如果存在）
		deleteUploadState(statePath)
		responseData := make(map[string]interface{})
		for k, v := range finish.Data {
			if k != "preview_url" {
				responseData[k] = v
			}
		}
		return &StandardResponse{
			Success: true,
			Code:    "OK",
			Message: "上传完成",
			Data:    responseData,
		}, nil
	}

	partSize := pre.Metadata.PartSize
	file.Seek(0, 0)

	var etags []string
	var startPartNumber int = 1

	// 如果使用保存的状态，恢复已上传的分片信息
	if useSavedState {
		etags = make([]string, 0, len(savedState.UploadedParts))
		// 按 partNumber 排序，填充已上传的分片 ETag
		maxPart := 0
		for partNum := range savedState.UploadedParts {
			if partNum > maxPart {
				maxPart = partNum
			}
		}
		// 填充已上传的分片（partNumber 从 1 开始）
		for i := 1; i <= maxPart; i++ {
			if etag, ok := savedState.UploadedParts[i]; ok {
				etags = append(etags, etag)
				startPartNumber = i + 1
			} else {
				// 如果中间有缺失的分片，从第一个缺失的分片开始
				startPartNumber = i
				break
			}
		}
	}

	// 用于计算速度和剩余时间
	var lastUpdateTime time.Time
	var lastUploaded int64

	// 如果从断点续传，计算已上传的字节数并跳过已上传的分片
	if useSavedState && startPartNumber > 1 {
		lastUploaded = int64(startPartNumber-1) * partSize
		if lastUploaded > fileSize {
			lastUploaded = fileSize
		}
		// 跳过已上传的分片
		skipBytes := int64(startPartNumber-1) * partSize
		if skipBytes > 0 {
			file.Seek(skipBytes, 0)
		}
	}

	partNumber := startPartNumber
	for {
		chunk := make([]byte, partSize)
		n, err := file.Read(chunk)
		if err == io.EOF {
			break
		}
		if err != nil {
			// 上传失败，保存当前状态以便断点续传
			if !useSavedState {
				savedState = &UploadState{
					FilePath:      filePath,
					DestPath:      destPath,
					FileSize:      fileSize,
					UploadID:      pre.Data.UploadID,
					TaskID:        pre.Data.TaskID,
					Bucket:        pre.Data.Bucket,
					ObjKey:        pre.Data.ObjKey,
					UploadURL:     pre.Data.UploadURL,
					PartSize:      partSize,
					UploadedParts: make(map[int]string),
					MimeType:      mimeType,
					AuthInfo:      pre.Data.AuthInfo,
					Callback:      pre.Data.Callback,
				}
			}
			// 保存已上传的分片
			for i, etag := range etags {
				savedState.UploadedParts[i+1] = etag
			}
			saveUploadState(statePath, savedState)

			return &StandardResponse{
				Success: false,
				Code:    "READ_FILE_ERROR",
				Message: fmt.Sprintf("failed to read file chunk: %v", err),
				Data:    nil,
			}, nil
		}

		if n == 0 {
			break
		}

		chunk = chunk[:n]

		etag, err := qc.upPart(pre, mimeType, partNumber, chunk)
		if err != nil {
			// 上传失败，保存当前状态以便断点续传
			if !useSavedState {
				savedState = &UploadState{
					FilePath:      filePath,
					DestPath:      destPath,
					FileSize:      fileSize,
					UploadID:      pre.Data.UploadID,
					TaskID:        pre.Data.TaskID,
					Bucket:        pre.Data.Bucket,
					ObjKey:        pre.Data.ObjKey,
					UploadURL:     pre.Data.UploadURL,
					PartSize:      partSize,
					UploadedParts: make(map[int]string),
					MimeType:      mimeType,
					AuthInfo:      pre.Data.AuthInfo,
					Callback:      pre.Data.Callback,
				}
			}
			// 保存已上传的分片
			for i, etag := range etags {
				savedState.UploadedParts[i+1] = etag
			}
			saveUploadState(statePath, savedState)

			return &StandardResponse{
				Success: false,
				Code:    "UPLOAD_PART_ERROR",
				Message: fmt.Sprintf("failed to upload part %d: %v", partNumber, err),
				Data:    nil,
			}, nil
		}

		etags = append(etags, etag)

		// 更新上传状态
		if !useSavedState {
			savedState = &UploadState{
				FilePath:      filePath,
				DestPath:      destPath,
				FileSize:      fileSize,
				UploadID:      pre.Data.UploadID,
				TaskID:        pre.Data.TaskID,
				Bucket:        pre.Data.Bucket,
				ObjKey:        pre.Data.ObjKey,
				UploadURL:     pre.Data.UploadURL,
				PartSize:      partSize,
				UploadedParts: make(map[int]string),
				MimeType:      mimeType,
				AuthInfo:      pre.Data.AuthInfo,
				Callback:      pre.Data.Callback,
			}
		}
		savedState.UploadedParts[partNumber] = etag
		// 每上传一个分片后保存状态
		saveUploadState(statePath, savedState)

		// 更新进度
		if progressCallback != nil {
			now := time.Now()
			uploaded := int64(partNumber) * int64(partSize)
			if uploaded > fileSize {
				uploaded = fileSize
			}

			progress := int(float64(uploaded) / float64(fileSize) * 100)
			if progress > 100 {
				progress = 100
			}

			elapsed := now.Sub(startTime)

			// 计算速度（使用最近一次更新的数据，避免初始速度不稳定）
			var speed float64
			var remaining time.Duration

			if !lastUpdateTime.IsZero() && elapsed > 0 {
				// 使用最近一次更新后的数据计算速度
				deltaTime := now.Sub(lastUpdateTime)
				deltaUploaded := uploaded - lastUploaded
				if deltaTime > 0 {
					speed = float64(deltaUploaded) / deltaTime.Seconds()
				} else {
					// 如果时间间隔太小，使用总体速度
					speed = float64(uploaded) / elapsed.Seconds()
				}

				// 计算剩余时间
				if speed > 0 {
					remainingBytes := fileSize - uploaded
					remaining = time.Duration(float64(remainingBytes)/speed) * time.Second
				}
			} else {
				// 第一次更新，使用总体速度
				if elapsed > 0 {
					speed = float64(uploaded) / elapsed.Seconds()
					if speed > 0 {
						remainingBytes := fileSize - uploaded
						remaining = time.Duration(float64(remainingBytes)/speed) * time.Second
					}
				}
			}

			// 格式化速度和剩余时间
			speedStr := formatSpeed(speed)
			remainingStr := formatDuration(remaining)

			progressInfo := &UploadProgress{
				Progress:     progress,
				Uploaded:     uploaded,
				Total:        fileSize,
				Speed:        speed,
				SpeedStr:     speedStr,
				Remaining:    remaining,
				RemainingStr: remainingStr,
				Elapsed:      elapsed,
			}

			progressCallback(progressInfo)

			lastUpdateTime = now
			lastUploaded = uploaded
		}

		partNumber++
	}

	// 10. 提交上传
	finish, err := qc.upCommit(pre, etags)
	if err != nil {
		return &StandardResponse{
			Success: false,
			Code:    "COMMIT_UPLOAD_ERROR",
			Message: fmt.Sprintf("commit upload failed: %v", err),
			Data:    nil,
		}, nil
	}

	// OSS commit 成功后，需要调用 upFinish 通知夸克服务器
	if finish.Code == 0 && finish.Status == 200 {
		// 调用 upFinish 确认上传完成
		finishResp, err := qc.upFinish(pre)
		if err != nil {
			return &StandardResponse{
				Success: false,
				Code:    "FINISH_UPLOAD_ERROR",
				Message: fmt.Sprintf("finish upload failed: %v", err),
				Data:    nil,
			}, nil
		}
		if finishResp.Code != 0 || finishResp.Status != 200 {
			return &StandardResponse{
				Success: false,
				Code:    "FINISH_UPLOAD_ERROR",
				Message: fmt.Sprintf("finish upload failed: code=%d, status=%d", finishResp.Code, finishResp.Status),
				Data:    nil,
			}, nil
		}

		// 上传成功，删除状态文件
		deleteUploadState(statePath)

		// 移除 preview_url 字段
		responseData := make(map[string]interface{})
		for k, v := range finishResp.Data {
			if k != "preview_url" {
				responseData[k] = v
			}
		}
		return &StandardResponse{
			Success: true,
			Code:    "OK",
			Message: "上传完成",
			Data:    responseData,
		}, nil
	}

	// 如果 commit 失败
	return &StandardResponse{
		Success: false,
		Code:    "COMMIT_UPLOAD_ERROR",
		Message: fmt.Sprintf("commit upload failed: code=%d, status=%d", finish.Code, finish.Status),
		Data:    nil,
	}, nil
}

// CreateFolder 创建文件夹
func (qc *QuarkClient) CreateFolder(folderName, pdirFid string) (*StandardResponse, error) {
	pdirFid = normalizeRootDir(pdirFid)

	data := map[string]interface{}{
		"pdir_fid":      pdirFid,
		"file_name":     folderName,
		"dir_path":      "",
		"dir_init_lock": false,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return &StandardResponse{
			Success: false,
			Code:    "CREATE_FOLDER_ERROR",
			Message: fmt.Sprintf("failed to marshal create folder data: %v", err),
			Data:    nil,
		}, nil
	}

	respMap, err := qc.makeRequest("POST", CREATE_FOLDER, bytes.NewBuffer(jsonData), nil)
	if err != nil {
		return &StandardResponse{
			Success: false,
			Code:    "CREATE_FOLDER_REQUEST_ERROR",
			Message: fmt.Sprintf("create folder request failed: %v", err),
			Data:    nil,
		}, nil
	}

	var createResp CreateFolderResponse
	if err := qc.parseResponse(respMap, &createResp); err != nil {
		return &StandardResponse{
			Success: false,
			Code:    "CREATE_FOLDER_DECODE_ERROR",
			Message: fmt.Sprintf("failed to decode create folder response: %v", err),
			Data:    nil,
		}, nil
	}

	if createResp.Code != 0 || createResp.Status != 200 {
		return &StandardResponse{
			Success: false,
			Code:    "CREATE_FOLDER_ERROR",
			Message: fmt.Sprintf("create folder failed: code=%d, status=%d", createResp.Code, createResp.Status),
			Data:    nil,
		}, nil
	}

	return &StandardResponse{
		Success: true,
		Code:    "OK",
		Message: "创建文件夹成功",
		Data:    createResp.Data,
	}, nil
}

// Copy 复制文件或目录
func (qc *QuarkClient) Copy(srcPath, destPath string) (*StandardResponse, error) {
	srcPath = normalizePath(srcPath)
	destPath = normalizePath(destPath)

	// 获取源文件/目录信息
	srcInfo, err := qc.GetFileInfo(srcPath)
	if err != nil {
		return &StandardResponse{
			Success: false,
			Code:    "GET_SOURCE_INFO_ERROR",
			Message: fmt.Sprintf("failed to get source info: %v", err),
			Data:    nil,
		}, nil
	}

	// 检查 GetFileInfo 是否成功
	if !srcInfo.Success {
		return &StandardResponse{
			Success: false,
			Code:    srcInfo.Code,
			Message: fmt.Sprintf("failed to get source info: %s", srcInfo.Message),
			Data:    nil,
		}, nil
	}

	// 安全地获取 fid
	srcFid, ok := srcInfo.Data["fid"].(string)
	if !ok || srcFid == "" {
		return &StandardResponse{
			Success: false,
			Code:    "INVALID_SOURCE_INFO",
			Message: "source file info is invalid: fid not found or empty",
			Data:    nil,
		}, nil
	}

	// 获取目标目录信息（如果destPath为空或与源路径相同，则使用源路径的父目录）
	var destDir string
	switch {
	case destPath == "" || destPath == srcPath:
		// 获取源路径的父目录
		srcPath = normalizePath(srcPath)
		parentPath := srcPath
		lastSlash := strings.LastIndex(parentPath, "/")
		if lastSlash == 0 {
			parentPath = "/"
		} else if lastSlash > 0 {
			parentPath = parentPath[:lastSlash]
			if parentPath == "" {
				parentPath = "/"
			}
		} else {
			parentPath = "/"
		}
		parentPath = normalizePath(parentPath)
		switch {
		case parentPath == "/" || parentPath == "." || parentPath == "":
			destDir = normalizeRootDir(parentPath)
		default:
			parentInfo, err := qc.GetFileInfo(parentPath)
			if err != nil {
				return &StandardResponse{
					Success: false,
					Code:    "GET_PARENT_DIRECTORY_INFO_ERROR",
					Message: fmt.Sprintf("failed to get parent directory info: %v", err),
					Data:    nil,
				}, nil
			}
			// 检查 GetFileInfo 是否成功
			if !parentInfo.Success {
				return &StandardResponse{
					Success: false,
					Code:    parentInfo.Code,
					Message: fmt.Sprintf("failed to get parent directory info: %s", parentInfo.Message),
					Data:    nil,
				}, nil
			}
			// 安全地获取 fid
			parentFid, ok := parentInfo.Data["fid"].(string)
			if !ok || parentFid == "" {
				return &StandardResponse{
					Success: false,
					Code:    "INVALID_PARENT_DIRECTORY_INFO",
					Message: "parent directory info is invalid: fid not found or empty",
					Data:    nil,
				}, nil
			}
			destDir = parentFid
		}
	case destPath == "/":
		// 根目录使用标准表示 "/"
		destDir = normalizeRootDir(destPath)
	default:
		// 获取目标目录信息
		destInfo, err := qc.GetFileInfo(destPath)
		if err != nil {
			return &StandardResponse{
				Success: false,
				Code:    "GET_DESTINATION_DIRECTORY_INFO_ERROR",
				Message: fmt.Sprintf("failed to get destination directory info: %v", err),
				Data:    nil,
			}, nil
		}
		// 检查 GetFileInfo 是否成功
		if !destInfo.Success {
			return &StandardResponse{
				Success: false,
				Code:    destInfo.Code,
				Message: fmt.Sprintf("failed to get destination directory info: %s", destInfo.Message),
				Data:    nil,
			}, nil
		}
		// 确保目标路径是一个目录
		isDir, ok := destInfo.Data["dir"].(bool)
		if !ok || !isDir {
			return &StandardResponse{
				Success: false,
				Code:    "DESTINATION_PATH_NOT_A_DIRECTORY",
				Message: fmt.Sprintf("destination path is not a directory: %s", destPath),
				Data:    nil,
			}, nil
		}
		// 安全地获取 fid
		destFid, ok := destInfo.Data["fid"].(string)
		if !ok || destFid == "" {
			return &StandardResponse{
				Success: false,
				Code:    "INVALID_DESTINATION_INFO",
				Message: "destination directory info is invalid: fid not found or empty",
				Data:    nil,
			}, nil
		}
		destDir = destFid
	}

	// 构建复制请求数据
	data := map[string]interface{}{
		"action_type":  1,
		"exclude_fids": []string{},
		"filelist":     []string{srcFid},
		"to_pdir_fid":  destDir,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return &StandardResponse{
			Success: false,
			Code:    "COPY_MARSHAL_ERROR",
			Message: fmt.Sprintf("failed to marshal copy data: %v", err),
			Data:    nil,
		}, nil
	}

	respMap, err := qc.makeRequest("POST", FILE_COPY, bytes.NewBuffer(jsonData), nil)
	if err != nil {
		return &StandardResponse{
			Success: false,
			Code:    "COPY_REQUEST_ERROR",
			Message: fmt.Sprintf("copy request failed: %v", err),
			Data:    nil,
		}, nil
	}

	var copyResp CopyResponse
	if err := qc.parseResponse(respMap, &copyResp); err != nil {
		return &StandardResponse{
			Success: false,
			Code:    "COPY_DECODE_ERROR",
			Message: fmt.Sprintf("failed to decode copy response: %v", err),
			Data:    nil,
		}, nil
	}

	if copyResp.Code != 0 && copyResp.Status != 200 {
		return &StandardResponse{
			Success: false,
			Code:    "COPY_FAILED",
			Message: fmt.Sprintf("copy failed: code=%d, status=%d", copyResp.Code, copyResp.Status),
			Data:    nil,
		}, nil
	}

	return &StandardResponse{
		Success: true,
		Code:    "OK",
		Message: "复制成功",
		Data:    map[string]interface{}{"fid": copyResp.Data.Fid},
	}, nil
}

// Move 移动文件或目录
// srcPath: 源路径（文件或目录）
// destPath: 目标目录路径（目标目录路径，不是文件路径）
func (qc *QuarkClient) Move(srcPath, destPath string) (*StandardResponse, error) {
	srcPath = normalizePath(srcPath)
	destPath = normalizePath(destPath)

	// 获取源文件/目录信息
	srcInfo, err := qc.GetFileInfo(srcPath)
	if err != nil {
		return &StandardResponse{
			Success: false,
			Code:    "GET_SOURCE_INFO_ERROR",
			Message: fmt.Sprintf("failed to get source info: %v", err),
			Data:    nil,
		}, nil
	}

	// 检查 GetFileInfo 是否成功
	if !srcInfo.Success {
		return &StandardResponse{
			Success: false,
			Code:    srcInfo.Code,
			Message: fmt.Sprintf("failed to get source info: %s", srcInfo.Message),
			Data:    nil,
		}, nil
	}

	// 安全地获取 fid
	srcFid, ok := srcInfo.Data["fid"].(string)
	if !ok || srcFid == "" {
		return &StandardResponse{
			Success: false,
			Code:    "INVALID_SOURCE_INFO",
			Message: "source file info is invalid: fid not found or empty",
			Data:    nil,
		}, nil
	}

	// 获取目标目录信息
	var destDir string
	if destPath == "" || destPath == "/" || destPath == "." {
		destDir = normalizeRootDir(destPath)
	} else {
		destInfo, err := qc.GetFileInfo(destPath)
		if err != nil {
			return &StandardResponse{
				Success: false,
				Code:    "GET_DESTINATION_DIRECTORY_INFO_ERROR",
				Message: fmt.Sprintf("failed to get destination directory info: %v", err),
				Data:    nil,
			}, nil
		}

		// 检查 GetFileInfo 是否成功
		if !destInfo.Success {
			return &StandardResponse{
				Success: false,
				Code:    destInfo.Code,
				Message: fmt.Sprintf("failed to get destination directory info: %s", destInfo.Message),
				Data:    nil,
			}, nil
		}

		// 确保目标路径是一个目录，不是文件
		isDir, ok := destInfo.Data["dir"].(bool)
		if !ok || !isDir {
			return &StandardResponse{
				Success: false,
				Code:    "DESTINATION_PATH_NOT_A_DIRECTORY",
				Message: fmt.Sprintf("destination path is not a directory: %s", destPath),
				Data:    nil,
			}, nil
		}

		// 安全地获取 destDir fid
		destFid, ok := destInfo.Data["fid"].(string)
		if !ok || destFid == "" {
			return &StandardResponse{
				Success: false,
				Code:    "INVALID_DESTINATION_INFO",
				Message: "destination directory info is invalid: fid not found or empty",
				Data:    nil,
			}, nil
		}
		destDir = destFid
	}

	data := map[string]interface{}{
		"action_type":  1,
		"exclude_fids": []string{},
		"filelist":     []string{srcFid},
		"to_pdir_fid":  destDir,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return &StandardResponse{
			Success: false,
			Code:    "MARSHAL_MOVE_DATA_ERROR",
			Message: fmt.Sprintf("failed to marshal move data: %v", err),
			Data:    nil,
		}, nil
	}

	respMap, err := qc.makeRequest("POST", FILE_MOVE, bytes.NewBuffer(jsonData), nil)
	if err != nil {
		return &StandardResponse{
			Success: false,
			Code:    "MOVE_REQUEST_ERROR",
			Message: fmt.Sprintf("move request failed: %v", err),
			Data:    nil,
		}, nil
	}

	var moveResp MoveResponse
	if err := qc.parseResponse(respMap, &moveResp); err != nil {
		return &StandardResponse{
			Success: false,
			Code:    "DECODE_MOVE_RESPONSE_ERROR",
			Message: fmt.Sprintf("failed to decode move response: %v", err),
			Data:    nil,
		}, nil
	}

	if moveResp.Code != 0 || moveResp.Status != 200 {
		return &StandardResponse{
			Success: false,
			Code:    "MOVE_FAILED",
			Message: fmt.Sprintf("move failed: code=%d, status=%d", moveResp.Code, moveResp.Status),
			Data:    nil,
		}, nil
	}

	return &StandardResponse{
		Success: true,
		Code:    "OK",
		Message: "移动成功",
		Data:    map[string]interface{}{"fid": moveResp.Data.Fid},
	}, nil
}

// Rename 重命名文件或目录
// oldPath: 原路径
// newName: 新名称
func (qc *QuarkClient) Rename(oldPath, newName string) (*StandardResponse, error) {
	oldPath = normalizePath(oldPath)

	// 获取文件/目录信息
	fileInfo, err := qc.GetFileInfo(oldPath)
	if err != nil {
		return &StandardResponse{
			Success: false,
			Code:    "GET_FILE_INFO_ERROR",
			Message: fmt.Sprintf("failed to get file info: %v", err),
			Data:    nil,
		}, nil
	}

	// 检查 GetFileInfo 是否成功
	if !fileInfo.Success {
		return &StandardResponse{
			Success: false,
			Code:    fileInfo.Code,
			Message: fmt.Sprintf("failed to get file info: %s", fileInfo.Message),
			Data:    nil,
		}, nil
	}

	// 安全地获取 fid
	fileFid, ok := fileInfo.Data["fid"].(string)
	if !ok || fileFid == "" {
		return &StandardResponse{
			Success: false,
			Code:    "INVALID_FILE_INFO",
			Message: "file info is invalid: fid not found or empty",
			Data:    nil,
		}, nil
	}

	data := map[string]interface{}{
		"fid":       fileFid,
		"file_name": newName,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return &StandardResponse{
			Success: false,
			Code:    "MARSHAL_RENAME_DATA_ERROR",
			Message: fmt.Sprintf("failed to marshal rename data: %v", err),
			Data:    nil,
		}, nil
	}

	respMap, err := qc.makeRequest("POST", FILE_RENAME, bytes.NewBuffer(jsonData), nil)
	if err != nil {
		return &StandardResponse{
			Success: false,
			Code:    "RENAME_REQUEST_ERROR",
			Message: fmt.Sprintf("rename request failed: %v", err),
			Data:    nil,
		}, nil
	}

	var renameResp RenameResponse
	if err := qc.parseResponse(respMap, &renameResp); err != nil {
		return &StandardResponse{
			Success: false,
			Code:    "DECODE_RENAME_RESPONSE_ERROR",
			Message: fmt.Sprintf("failed to decode rename response: %v", err),
			Data:    nil,
		}, nil
	}

	if renameResp.Code != 0 || renameResp.Status != 200 {
		return &StandardResponse{
			Success: false,
			Code:    "RENAME_FAILED",
			Message: fmt.Sprintf("rename failed: code=%d, status=%d", renameResp.Code, renameResp.Status),
			Data:    nil,
		}, nil
	}

	return &StandardResponse{
		Success: true,
		Code:    "OK",
		Message: "重命名成功",
		Data:    map[string]interface{}{"fid": renameResp.Data.Fid},
	}, nil
}

// listByFid 通过 FID 列出目录下的文件（内部方法，避免循环调用）
// 支持分页，自动获取所有文件
func (qc *QuarkClient) listByFid(pdirFid string, parentPath ...string) (*StandardResponse, error) {
	// 确定父目录路径：如果提供了 parentPath，使用它；否则根据 pdirFid 判断
	var basePath string
	if len(parentPath) > 0 && parentPath[0] != "" {
		basePath = parentPath[0]
	} else if pdirFid == "0" {
		basePath = "/"
	} else {
		// 如果没有提供 parentPath 且不是根目录，路径为空（无法确定）
		basePath = ""
	}

	// 用于存储所有文件的列表
	allFileList := make([]QuarkFileInfo, 0)
	page := 1
	pageSize := 50 // 每页大小
	hasMore := true

	// 循环获取所有数据
	for hasMore {
		// 构建查询参数
		params := url.Values{}
		params.Set("uc_param_str", "")
		params.Set("pdir_fid", pdirFid)
		params.Set("_page", fmt.Sprintf("%d", page))
		params.Set("_size", fmt.Sprintf("%d", pageSize))
		params.Set("_fetch_total", "1")
		params.Set("_fetch_sub_dirs", "0")
		params.Set("_sort", "file_type:asc,updated_at:desc")
		params.Set("fetch_all_file", "1")
		params.Set("fetch_risk_file_name", "1")

		// 构建完整 URL
		endpoint := FILE_SORT + "?" + params.Encode()
		respMap, err := qc.makeRequest("GET", endpoint, nil, nil)
		if err != nil {
			return &StandardResponse{
				Success: false,
				Code:    "LIST_REQUEST_ERROR",
				Message: fmt.Sprintf("list request failed: %v", err),
				Data:    nil,
			}, nil
		}

		// 检查状态码
		status, _ := respMap["status"].(float64)
		code, _ := respMap["code"].(float64)
		if status >= 400 || code != 0 {
			message, _ := respMap["message"].(string)
			return &StandardResponse{
				Success: false,
				Code:    "LIST_FAILED",
				Message: fmt.Sprintf("list files failed: %s (status: %.0f, code: %.0f)", message, status, code),
				Data:    nil,
			}, nil
		}

		// 解析响应数据
		data, ok := respMap["data"].(map[string]interface{})
		if !ok {
			return &StandardResponse{
				Success: false,
				Code:    "INVALID_RESPONSE_FORMAT",
				Message: "invalid response format: data field not found",
				Data:    nil,
			}, nil
		}

		listData, ok := data["list"].([]interface{})
		if !ok {
			return &StandardResponse{
				Success: false,
				Code:    "INVALID_LIST_FORMAT",
				Message: "invalid list format in response",
				Data:    nil,
			}, nil
		}

		// 如果本次返回的数据为空，说明已经获取了所有数据
		if len(listData) == 0 {
			hasMore = false
			break
		}

		// 转换文件列表，根据实际API响应精准映射所有字段
		for _, item := range listData {
			if itemMap, ok := item.(map[string]interface{}); ok {
				var fileInfo QuarkFileInfo

				// 映射 fid (文件ID)
				if fid, ok := itemMap["fid"].(string); ok {
					fileInfo.Fid = fid
				}

				// 映射 file_name (文件名)
				if name, ok := itemMap["file_name"].(string); ok {
					fileInfo.Name = name
					// 构建文件路径：根据父目录路径和文件名
					if basePath == "/" {
						fileInfo.Path = "/" + name
					} else if basePath != "" {
						fileInfo.Path = normalizePath(filepath.Join(basePath, name))
					} else {
						fileInfo.Path = "" // 无法确定路径
					}
				} else {
					fileInfo.Path = ""
				}

				// 映射 size (文件大小，可能是 float64 或 int)
				if size, ok := itemMap["size"].(float64); ok {
					fileInfo.Size = int64(size)
				} else if size, ok := itemMap["size"].(int); ok {
					fileInfo.Size = int64(size)
				} else if size, ok := itemMap["size"].(int64); ok {
					fileInfo.Size = size
				}

				// 处理创建时间：优先使用 created_at，其次使用 l_created_at（都是毫秒时间戳）
				if createdAt, ok := itemMap["created_at"].(float64); ok {
					fileInfo.CreatedAt = int64(createdAt)
					fileInfo.CreateTime = int64(createdAt) / 1000 // 转换为秒
				} else if createdAt, ok := itemMap["created_at"].(int64); ok {
					fileInfo.CreatedAt = createdAt
					fileInfo.CreateTime = createdAt / 1000
				} else if lCreatedAt, ok := itemMap["l_created_at"].(float64); ok {
					fileInfo.LCreatedAt = int64(lCreatedAt)
					fileInfo.CreateTime = int64(lCreatedAt) / 1000 // 转换为秒
				} else if lCreatedAt, ok := itemMap["l_created_at"].(int64); ok {
					fileInfo.LCreatedAt = lCreatedAt
					fileInfo.CreateTime = lCreatedAt / 1000
				}

				// 处理修改时间：优先使用 updated_at，其次使用 l_updated_at（都是毫秒时间戳）
				if updatedAt, ok := itemMap["updated_at"].(float64); ok {
					fileInfo.UpdatedAt = int64(updatedAt)
					fileInfo.ModifyTime = int64(updatedAt) / 1000 // 转换为秒
				} else if updatedAt, ok := itemMap["updated_at"].(int64); ok {
					fileInfo.UpdatedAt = updatedAt
					fileInfo.ModifyTime = updatedAt / 1000
				} else if lUpdatedAt, ok := itemMap["l_updated_at"].(float64); ok {
					fileInfo.LUpdatedAt = int64(lUpdatedAt)
					fileInfo.ModifyTime = int64(lUpdatedAt) / 1000 // 转换为秒
				} else if lUpdatedAt, ok := itemMap["l_updated_at"].(int64); ok {
					fileInfo.LUpdatedAt = lUpdatedAt
					fileInfo.ModifyTime = lUpdatedAt / 1000
				}

				// 处理是否为目录：优先使用 dir 字段，其次使用 file 字段取反
				if dir, ok := itemMap["dir"].(bool); ok {
					fileInfo.IsDirectory = dir
				} else if file, ok := itemMap["file"].(bool); ok {
					fileInfo.IsDirectory = !file
				}

				// download_url 字段在列表API中通常不存在，需要单独获取
				fileInfo.DownloadURL = ""

				allFileList = append(allFileList, fileInfo)
			}
		}

		// 检查是否还有更多数据
		// 如果返回的数据量少于 pageSize，说明已经获取了所有数据
		if len(listData) < pageSize {
			hasMore = false
		} else {
			// 检查响应中是否有 total 字段来判断是否还有更多数据
			if total, ok := data["total"].(float64); ok {
				currentCount := float64(len(allFileList))
				hasMore = currentCount < total
			} else {
				// 如果没有 total 字段，根据返回的数据量判断
				// 如果返回的数据量等于 pageSize，可能还有更多数据
				hasMore = len(listData) == pageSize
			}
			page++
		}
	}

	return &StandardResponse{
		Success: true,
		Code:    "OK",
		Message: "列出目录成功",
		Data:    map[string]interface{}{"list": allFileList},
	}, nil
}

// List 列出目录下的文件
// dirPath: 目录路径（根目录使用 "/"）
func (qc *QuarkClient) List(dirPath string) (*StandardResponse, error) {
	// 处理目录路径：根目录使用标准表示 "/"
	var pdirFid string
	if dirPath == "" || dirPath == "/" {
		pdirFid = normalizeRootDir(dirPath) // 根目录使用 "/"，转换为 "0"
	} else if dirPath == "0" {
		// 兼容旧代码：如果传入 "0"，也转换为根目录
		pdirFid = "0"
	} else if strings.HasPrefix(dirPath, "/") {
		// 是路径字符串，需要转换为 FID
		dirInfo, err := qc.GetFileInfo(dirPath, true) // 传入 true 跳过路径转换检查
		if err != nil {
			return &StandardResponse{
				Success: false,
				Code:    "GET_DIRECTORY_INFO_ERROR",
				Message: fmt.Sprintf("failed to get directory info: %v", err),
				Data:    nil,
			}, nil
		}
		if !dirInfo.Success {
			return &StandardResponse{
				Success: false,
				Code:    dirInfo.Code,
				Message: fmt.Sprintf("failed to get directory info: %s", dirInfo.Message),
				Data:    nil,
			}, nil
		}
		// 安全地获取 fid
		fid, ok := dirInfo.Data["fid"].(string)
		if !ok || fid == "" {
			return &StandardResponse{
				Success: false,
				Code:    "INVALID_DIRECTORY_INFO",
				Message: "directory info is invalid: fid not found or empty",
				Data:    nil,
			}, nil
		}
		pdirFid = fid
	} else {
		// 假设是 FID（不是以 / 开头的字符串）
		pdirFid = dirPath
	}

	// 确定父目录路径用于构建文件路径
	var parentPath string
	if dirPath == "" || dirPath == "/" || dirPath == "0" {
		parentPath = "/"
	} else if strings.HasPrefix(dirPath, "/") {
		parentPath = dirPath
	} else {
		// 如果传入的是 FID，无法确定路径
		parentPath = ""
	}

	// 使用内部方法通过 FID 列出文件
	return qc.listByFid(pdirFid, parentPath)
}

// GetFileInfo 获取文件或目录信息
func (qc *QuarkClient) GetFileInfo(remotePath string, skipPathConversion ...bool) (*StandardResponse, error) {
	remotePath = normalizePath(remotePath)

	if remotePath == "/" || remotePath == "" || remotePath == "." {
		return &StandardResponse{
			Success: true,
			Code:    "OK",
			Message: "根目录",
			Data: map[string]interface{}{
				"fid":          "0",
				"file_name":    "",
				"path":         "/",
				"size":         0,
				"dir":          true,
				"is_directory": true,
			},
		}, nil
	}

	fileName := filepath.Base(remotePath)
	if fileName == "." || fileName == "/" {
		parts := strings.Split(strings.Trim(remotePath, "/"), "/")
		if len(parts) > 0 && parts[len(parts)-1] != "" {
			fileName = parts[len(parts)-1]
		} else {
			fileName = ""
		}
	}

	parentPath := remotePath
	lastSlash := strings.LastIndex(parentPath, "/")
	if lastSlash == 0 {
		parentPath = "/"
	} else if lastSlash > 0 {
		parentPath = parentPath[:lastSlash]
		if parentPath == "" {
			parentPath = "/"
		}
	} else {
		parentPath = "/"
	}

	var parentFid string
	parentPath = normalizePath(parentPath)
	if parentPath == "/" || parentPath == "." || parentPath == "" {
		parentFid = normalizeRootDir(parentPath)
	} else {
		if parentPath == remotePath {
			return &StandardResponse{
				Success: false,
				Code:    "INVALID_PATH",
				Message: fmt.Sprintf("invalid path: parent path equals current path: %s", remotePath),
				Data:    nil,
			}, nil
		}

		parentInfo, err := qc.GetFileInfo(parentPath, true)
		if err != nil {
			return &StandardResponse{
				Success: false,
				Code:    "GET_PARENT_DIRECTORY_ERROR",
				Message: fmt.Sprintf("failed to get parent directory: %v", err),
				Data:    nil,
			}, nil
		}
		if !parentInfo.Success {
			return &StandardResponse{
				Success: false,
				Code:    parentInfo.Code,
				Message: fmt.Sprintf("failed to get parent directory: %s", parentInfo.Message),
				Data:    nil,
			}, nil
		}
		fid, ok := parentInfo.Data["fid"].(string)
		if !ok || fid == "" {
			return &StandardResponse{
				Success: false,
				Code:    "INVALID_PARENT_DIRECTORY_INFO",
				Message: "parent directory info is invalid: fid not found or empty",
				Data:    nil,
			}, nil
		}
		parentFid = fid
	}

	var parentPathForList string
	parentPath = normalizePath(parentPath)
	if parentPath == "/" || parentPath == "." || parentPath == "" {
		parentPathForList = "/"
	} else {
		parentPathForList = parentPath
	}

	// 使用 listByFid 列出父目录下的文件（避免循环调用）
	listResp, err := qc.listByFid(parentFid, parentPathForList)
	if err != nil {
		return &StandardResponse{
			Success: false,
			Code:    "LIST_DIRECTORY_ERROR",
			Message: fmt.Sprintf("failed to list directory: %v", err),
			Data:    nil,
		}, nil
	}

	// 检查 List 是否成功
	if !listResp.Success {
		return &StandardResponse{
			Success: false,
			Code:    listResp.Code,
			Message: fmt.Sprintf("failed to list directory: %s", listResp.Message),
			Data:    nil,
		}, nil
	}

	listData, ok := listResp.Data["list"]
	if !ok {
		return &StandardResponse{
			Success: false,
			Code:    "INVALID_LIST_DATA",
			Message: "list data not found in response",
			Data:    nil,
		}, nil
	}

	fileList, ok := listData.([]QuarkFileInfo)
	if !ok {
		if listInterface, ok := listData.([]interface{}); ok {
			fileList = make([]QuarkFileInfo, 0, len(listInterface))
			for _, item := range listInterface {
				if itemMap, ok := item.(map[string]interface{}); ok {
					var fileInfo QuarkFileInfo
					if fid, ok := itemMap["fid"].(string); ok {
						fileInfo.Fid = fid
					}
					if name, ok := itemMap["file_name"].(string); ok {
						fileInfo.Name = name
						if parentPathForList == "/" {
							fileInfo.Path = "/" + name
						} else if parentPathForList != "" {
							fileInfo.Path = normalizePath(filepath.Join(parentPathForList, name))
						} else {
							fileInfo.Path = ""
						}
					} else {
						fileInfo.Path = ""
					}
					if size, ok := itemMap["size"].(float64); ok {
						fileInfo.Size = int64(size)
					}
					if createdAt, ok := itemMap["created_at"].(float64); ok {
						fileInfo.CreateTime = int64(createdAt) / 1000
					} else if lCreatedAt, ok := itemMap["l_created_at"].(float64); ok {
						fileInfo.CreateTime = int64(lCreatedAt) / 1000
					}
					if updatedAt, ok := itemMap["updated_at"].(float64); ok {
						fileInfo.ModifyTime = int64(updatedAt) / 1000
					} else if lUpdatedAt, ok := itemMap["l_updated_at"].(float64); ok {
						fileInfo.ModifyTime = int64(lUpdatedAt) / 1000
					}
					if dir, ok := itemMap["dir"].(bool); ok {
						fileInfo.IsDirectory = dir
					} else if file, ok := itemMap["file"].(bool); ok {
						fileInfo.IsDirectory = !file
					}
					fileInfo.DownloadURL = ""
					fileList = append(fileList, fileInfo)
				}
			}
		} else {
			return &StandardResponse{
				Success: false,
				Code:    "INVALID_LIST_FORMAT",
				Message: "list data format is invalid",
				Data:    nil,
			}, nil
		}
	}

	for _, file := range fileList {
		if file.Name == fileName {
			// 找到匹配的文件，构建返回数据
			fileData := map[string]interface{}{
				"fid":          file.Fid,
				"file_name":    file.Name,
				"path":         file.Path,
				"size":         file.Size,
				"dir":          file.IsDirectory,
				"ctime":        file.CreateTime,
				"mtime":        file.ModifyTime,
				"download_url": file.DownloadURL,
			}

			return &StandardResponse{
				Success: true,
				Code:    "OK",
				Message: "获取文件信息成功",
				Data:    fileData,
			}, nil
		}
	}

	// 文件未找到
	return &StandardResponse{
		Success: false,
		Code:    "FILE_NOT_FOUND",
		Message: fmt.Sprintf("file not found: %s", remotePath),
		Data:    nil,
	}, nil
}

// Delete 删除文件或目录
func (qc *QuarkClient) Delete(remotePath string) (*StandardResponse, error) {
	remotePath = normalizePath(remotePath)

	// 获取文件信息以获取文件 ID
	fileInfo, err := qc.GetFileInfo(remotePath)
	if err != nil {
		return &StandardResponse{
			Success: false,
			Code:    "GET_FILE_INFO_ERROR",
			Message: fmt.Sprintf("failed to get file info: %v", err),
			Data:    nil,
		}, nil
	}

	// 检查 GetFileInfo 是否成功
	if !fileInfo.Success {
		return &StandardResponse{
			Success: false,
			Code:    fileInfo.Code,
			Message: fmt.Sprintf("failed to get file info: %s", fileInfo.Message),
			Data:    nil,
		}, nil
	}

	// 安全地获取 fid
	fileFid, ok := fileInfo.Data["fid"].(string)
	if !ok || fileFid == "" {
		return &StandardResponse{
			Success: false,
			Code:    "INVALID_FILE_INFO",
			Message: "file info is invalid: fid not found or empty",
			Data:    nil,
		}, nil
	}

	deleteData := map[string]interface{}{
		"action_type":  1,
		"exclude_fids": []string{},
		"filelist":     []string{fileFid},
	}

	jsonData, err := json.Marshal(deleteData)
	if err != nil {
		return &StandardResponse{
			Success: false,
			Code:    "MARSHAL_DELETE_DATA_ERROR",
			Message: fmt.Sprintf("failed to marshal delete data: %v", err),
			Data:    nil,
		}, nil
	}

	respMap, err := qc.makeRequest("POST", FILE_DELETE, bytes.NewBuffer(jsonData), nil)
	if err != nil {
		return &StandardResponse{
			Success: false,
			Code:    "DELETE_REQUEST_ERROR",
			Message: fmt.Sprintf("delete request failed: %v", err),
			Data:    nil,
		}, nil
	}

	var deleteResp struct {
		Status  int                    `json:"status"`
		Code    int                    `json:"code"`
		Message string                 `json:"message"`
		Data    map[string]interface{} `json:"data"`
	}

	if err := qc.parseResponse(respMap, &deleteResp); err != nil {
		return &StandardResponse{
			Success: false,
			Code:    "DECODE_DELETE_RESPONSE_ERROR",
			Message: fmt.Sprintf("failed to decode delete response: %v", err),
			Data:    nil,
		}, nil
	}

	if deleteResp.Status >= 400 || deleteResp.Code != 0 {
		return &StandardResponse{
			Success: false,
			Code:    "DELETE_FAILED",
			Message: fmt.Sprintf("delete failed: %s (status: %d, code: %d)", deleteResp.Message, deleteResp.Status, deleteResp.Code),
			Data:    nil,
		}, nil
	}

	return &StandardResponse{
		Success: true,
		Code:    "OK",
		Message: "删除成功",
		Data:    map[string]interface{}{"fid": deleteResp.Data["fid"]},
	}, nil
}

// BuildHeaders 实现 RequestHeaderBuilder 接口（OSSPartUploadHeaderBuilder）
func (b *OSSPartUploadHeaderBuilder) BuildHeaders(req *http.Request, qc *QuarkClient) error {
	req.Header.Set("Authorization", b.AuthKey)
	req.Header.Set("Content-Type", b.MimeType)
	req.Header.Set("x-oss-date", b.Timestamp)
	req.Header.Set("x-oss-user-agent", "aliyun-sdk-js/6.6.1 Chrome 98.0.4758.80 on Windows 10 64-bit")
	return nil
}

// BuildHeaders 实现 RequestHeaderBuilder 接口（OSSCommitHeaderBuilder）
func (b *OSSCommitHeaderBuilder) BuildHeaders(req *http.Request, qc *QuarkClient) error {
	req.Header.Set("Authorization", b.AuthKey)
	req.Header.Set("Content-MD5", b.ContentMD5)
	req.Header.Set("Content-Type", "application/xml")
	req.Header.Set("Referer", "https://pan.quark.cn/")
	req.Header.Set("x-oss-callback", b.Callback)
	req.Header.Set("x-oss-date", b.Timestamp)
	req.Header.Set("x-oss-user-agent", "aliyun-sdk-js/6.6.1 Chrome 98.0.4758.80 on Windows 10 64-bit")
	return nil
}

// GetDownloadURL 获取文件的下载链接
// fid: 文件ID
// 返回: 下载链接URL
func (qc *QuarkClient) GetDownloadURL(fid string) (string, error) {
	// 构建请求数据（API 期望 fids 为数组）
	data := map[string]interface{}{
		"fids": []string{fid},
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("failed to marshal download request: %w", err)
	}

	// 发送请求
	respMap, err := qc.makeRequest("POST", FILE_DOWNLOAD, bytes.NewBuffer(jsonData), nil)
	if err != nil {
		return "", fmt.Errorf("download request failed: %w", err)
	}

	// 解析响应
	var downloadResp DownloadResponse
	if err := qc.parseResponse(respMap, &downloadResp); err != nil {
		return "", fmt.Errorf("failed to decode download response: %w", err)
	}

	// 检查响应状态
	if downloadResp.Code != 0 || downloadResp.Status != 200 {
		return "", fmt.Errorf("download failed: code=%d, status=%d", downloadResp.Code, downloadResp.Status)
	}

	// 检查数据数组是否为空
	if len(downloadResp.Data) == 0 {
		return "", fmt.Errorf("download response data is empty")
	}

	// 返回第一个文件的下载链接
	return downloadResp.Data[0].DownloadURL, nil
}
