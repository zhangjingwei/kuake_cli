package sdk

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/tls"
	"encoding"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"hash"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// isRetryableError 判断错误是否为可重试的瞬时网络故障
// EOF、连接重置、超时等属于可重试错误；业务逻辑错误（如 auth failed）不可重试
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	retryablePatterns := []string{
		"EOF",
		"connection reset",
		"connection refused",
		"broken pipe",
		"i/o timeout",
		"TLS handshake timeout",
	}
	for _, pattern := range retryablePatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}
	return false
}

type uploadPartJob struct {
	partNumber int
	chunkData  []byte
	hashCtx    *HashCtx
}

type uploadPartResult struct {
	partNumber int
	size       int64
	etag       string
	err        error
}

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

func cloneHashCtx(hashCtx *HashCtx) *HashCtx {
	if hashCtx == nil {
		return nil
	}
	cloned := *hashCtx
	return &cloned
}

func buildUploadProgressInfo(
	uploaded int64,
	total int64,
	startTime time.Time,
	lastUpdateTime *time.Time,
	lastUploaded *int64,
) *UploadProgress {
	if uploaded < 0 {
		uploaded = 0
	}
	if uploaded > total {
		uploaded = total
	}

	progress := 0
	if total > 0 {
		progress = int(float64(uploaded) / float64(total) * 100)
		if progress > 100 {
			progress = 100
		}
	}

	now := time.Now()
	elapsed := now.Sub(startTime)

	var speed float64
	var remaining time.Duration

	if !lastUpdateTime.IsZero() && elapsed > 0 {
		deltaTime := now.Sub(*lastUpdateTime)
		deltaUploaded := uploaded - *lastUploaded
		if deltaTime > 0 {
			speed = float64(deltaUploaded) / deltaTime.Seconds()
		} else {
			speed = float64(uploaded) / elapsed.Seconds()
		}
		if speed > 0 {
			remainingBytes := total - uploaded
			remaining = time.Duration(float64(remainingBytes)/speed) * time.Second
		}
	} else if elapsed > 0 {
		speed = float64(uploaded) / elapsed.Seconds()
		if speed > 0 {
			remainingBytes := total - uploaded
			remaining = time.Duration(float64(remainingBytes)/speed) * time.Second
		}
	}

	*lastUpdateTime = now
	*lastUploaded = uploaded

	return &UploadProgress{
		Progress:     progress,
		Uploaded:     uploaded,
		Total:        total,
		Speed:        speed,
		SpeedStr:     formatSpeed(speed),
		Remaining:    remaining,
		RemainingStr: formatDuration(remaining),
		Elapsed:      elapsed,
	}
}

func (qc *QuarkClient) uploadPartsParallel(
	file *os.File,
	pre *PreUploadResponse,
	mimeType string,
	partSize int64,
	fileSize int64,
	statePath string,
	savedState *UploadState,
	startTime time.Time,
	progressCallback func(*UploadProgress),
	uploadParallel int,
	alreadyUploaded map[int]string, // 断点续传：已上传分片（partNumber -> etag），为空则全新上传
	hashMD5 hash.Hash, // 嵌入式哈希：生产者累积计算 MD5（用于 upHash）
	hashSHA1ForUpHash hash.Hash, // 嵌入式哈希：生产者累积计算 SHA1（用于 upHash）
) (map[int]string, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	jobCh := make(chan uploadPartJob, uploadParallel*2)
	resultCh := make(chan uploadPartResult, uploadParallel*2)

	var workerWG sync.WaitGroup
	for i := 0; i < uploadParallel; i++ {
		workerWG.Add(1)
		go func() {
			defer workerWG.Done()
			for job := range jobCh {
				if ctx.Err() != nil {
					return
				}
				// 分片级重试：最多重试 3 次，指数退避（1s, 2s, 4s）
				const maxRetries = 3
				var etag string
				var lastErr error
				for attempt := 0; attempt <= maxRetries; attempt++ {
					if ctx.Err() != nil {
						return
					}
					var uploadErr error
					etag, _, uploadErr = qc.upPart(pre, mimeType, job.partNumber, job.chunkData, job.hashCtx) // 【Round 20.5】恢复传递 HashCtx。虽然是并行模式，但服务端仍要求每个分片携带 Context，最终在 commit 阶段做链式跨分片校验。
					if uploadErr == nil {
						lastErr = nil
						break
					}
					lastErr = uploadErr
					// 仅对可重试的网络错误进行重试
					if !isRetryableError(uploadErr) {
						break
					}
					if attempt < maxRetries {
						backoff := time.Duration(1<<uint(attempt)) * time.Second
						fmt.Printf("[重试] 分片 %d 上传失败 (第 %d/%d 次): %v, %.0f秒后重试...\n",
							job.partNumber, attempt+1, maxRetries, uploadErr, backoff.Seconds())
						time.Sleep(backoff)
					}
				}
				if lastErr != nil {
					resultCh <- uploadPartResult{
						partNumber: job.partNumber,
						err:        fmt.Errorf("failed to upload part %d (after %d retries): %w", job.partNumber, maxRetries, lastErr),
					}
					cancel()
					return
				}
				select {
				case resultCh <- uploadPartResult{
					partNumber: job.partNumber,
					size:       int64(len(job.chunkData)),
					etag:       etag,
				}:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	go func() {
		defer close(jobCh)

		partNumber := 1
		cumulativeHash := sha1.New()
		var hashCtx *HashCtx
		var processedBytes int64

		for {
			if ctx.Err() != nil {
				return
			}

			chunk := make([]byte, partSize)
			n, err := file.Read(chunk)
			if err == io.EOF {
				return
			}
			if err != nil {
				resultCh <- uploadPartResult{
					partNumber: partNumber,
					err:        fmt.Errorf("failed to read file chunk: %w", err),
				}
				cancel()
				return
			}
			if n == 0 {
				return
			}

			chunk = chunk[:n]

			// 嵌入式哈希：读取分片后同时写入 MD5+SHA1，消除第二个文件句柄的 14GB 冗余读取
			if hashMD5 != nil {
				hashMD5.Write(chunk)
			}
			if hashSHA1ForUpHash != nil {
				hashSHA1ForUpHash.Write(chunk)
			}

			var currentHashCtx *HashCtx
			if partNumber >= 2 {
				currentHashCtx = cloneHashCtx(hashCtx)
			}

			hashCtx, _ = updateHashCtxFromHash(cumulativeHash, chunk, processedBytes)
			processedBytes += int64(len(chunk))

			// 断点续传：跳过已上传的分片（仍需读文件和计算哈希以维持后续分片 HashCtx 一致性）
			if _, ok := alreadyUploaded[partNumber]; ok {
				partNumber++
				continue
			}

			job := uploadPartJob{
				partNumber: partNumber,
				chunkData:  chunk,
				hashCtx:    currentHashCtx,
			}

			select {
			case jobCh <- job:
			case <-ctx.Done():
				return
			}
			partNumber++
		}
	}()

	go func() {
		workerWG.Wait()
		close(resultCh)
	}()

	totalParts := int((fileSize + partSize - 1) / partSize)
	uploadedPartMap := make(map[int]string, totalParts)
	var uploadedBytes int64
	var lastUpdateTime time.Time
	var lastUploaded int64
	var firstErr error

	// 断点续传：预填充已上传分片信息，计算已传字节数
	for pn, etag := range alreadyUploaded {
		uploadedPartMap[pn] = etag
		if int64(pn) < int64(totalParts) {
			uploadedBytes += partSize
		} else {
			remainder := fileSize % partSize
			if remainder > 0 {
				uploadedBytes += remainder
			} else {
				uploadedBytes += partSize
			}
		}
	}
	if len(alreadyUploaded) > 0 {
		lastUpdateTime = time.Now()
		lastUploaded = uploadedBytes
	}

	for result := range resultCh {
		if result.err != nil {
			if firstErr == nil {
				firstErr = result.err
				cancel()
			}
			continue
		}

		uploadedPartMap[result.partNumber] = result.etag
		savedState.UploadedParts[result.partNumber] = result.etag
		_ = saveUploadState(statePath, savedState)

		uploadedBytes += result.size
		if progressCallback != nil {
			progressInfo := buildUploadProgressInfo(
				uploadedBytes,
				fileSize,
				startTime,
				&lastUpdateTime,
				&lastUploaded,
			)
			progressCallback(progressInfo)
		}
	}

	if firstErr != nil {
		return nil, firstErr
	}

	if len(uploadedPartMap) != totalParts {
		return nil, fmt.Errorf("parallel upload incomplete: expected %d parts, got %d parts", totalParts, len(uploadedPartMap))
	}

	return uploadedPartMap, nil
}

// stripQuotes 去掉路径参数中可能存在的首尾引号（处理 Git Bash 等特殊情况）
// 某些 shell 或调用方式可能会保留引号，此函数用于统一处理
func stripQuotes(path string) string {
	path = strings.TrimSpace(path)
	if len(path) >= 2 && path[0] == '"' && path[len(path)-1] == '"' {
		path = path[1 : len(path)-1]
	}
	return path
}

// normalizePath 将路径标准化为 Unix 风格（使用 / 作为分隔符）
func normalizePath(path string) string {
	path = stripQuotes(path)
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
		"parallel_upload": true,
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

// updateHashCtxFromHash 更新SHA1增量哈希上下文
// 使用 MarshalBinary 提取 SHA1 的真正内部中间状态（h0-h4），而非 Sum() 的 finalized 摘要。
// 【Round 20 关键修复】原版使用 hash.Sum(nil) 获取的是经过 padding+finalization 的最终摘要，
// 与 OSS 要求的内部中间状态完全不同，导致 X-Oss-Hash-Ctx 校验失败、多分片上传被限速到 1.3 MB/s。
//
// Go sha1 MarshalBinary 布局：magic(4B) + h[0..4](20B, 大端序 uint32) + x_buffer(64B) + len(8B)
//
// hash: 累积的SHA1哈希对象（已处理所有前面的分片）
// chunkData: 当前分片数据
// totalBytes: 已处理的总字节数（该参数现已不用，改为从 MarshalBinary 中直接提取）
func updateHashCtxFromHash(h hash.Hash, chunkData []byte, totalBytes int64) (*HashCtx, error) {
	// 1. 把当前分片数据写入哈希对象（累积更新）
	h.Write(chunkData)

	// 2. 通过 MarshalBinary 获取 SHA1 的真正内部状态
	marshaler, ok := h.(encoding.BinaryMarshaler)
	if !ok {
		return nil, fmt.Errorf("hash does not support BinaryMarshaler interface")
	}
	state, err := marshaler.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal hash state: %w", err)
	}

	// 3. 从二进制状态中提取 h0-h4（偏移 4 开始，每个 4 字节，大端序 uint32）
	h0 := binary.BigEndian.Uint32(state[4:8])
	h1 := binary.BigEndian.Uint32(state[8:12])
	h2 := binary.BigEndian.Uint32(state[12:16])
	h3 := binary.BigEndian.Uint32(state[16:20])
	h4 := binary.BigEndian.Uint32(state[20:24])

	// 4. 从末尾 8 字节提取已处理字节数，转换为比特数（× 8）
	//   【Round 20.5 修复】Nl 和 Nh 分别代表低 32 位和高 32 位的比特数。
	//   超过 ~536MB 的文件，总比特数会大于 4.29 亿（uint32 溢出）。
	//   我们必须将其正确拆分为高低 32 位，否则服务端解析溢出会导致 commit 阶段 ContextCompareFailed。
	totalBytesProcessed := binary.BigEndian.Uint64(state[len(state)-8:])
	totalBits := totalBytesProcessed * 8
	nl := uint32(totalBits & 0xFFFFFFFF)
	nh := uint32(totalBits >> 32)

	return &HashCtx{
		HashType: "sha1",
		H0:       fmt.Sprintf("%d", h0),
		H1:       fmt.Sprintf("%d", h1),
		H2:       fmt.Sprintf("%d", h2),
		H3:       fmt.Sprintf("%d", h3),
		H4:       fmt.Sprintf("%d", h4),
		Nl:       fmt.Sprintf("%d", nl),
		Nh:       fmt.Sprintf("%d", nh),
		Data:     "",
		Num:      "0",
	}, nil
}

// encodeHashCtx 将 HashCtx 编码为 base64 字符串
func encodeHashCtx(ctx *HashCtx) (string, error) {
	if ctx == nil {
		return "", nil
	}
	jsonData, err := json.Marshal(ctx)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(jsonData), nil
}

// upPart 上传文件分片
func (qc *QuarkClient) upPart(pre *PreUploadResponse, mimeType string, partNumber int, chunkData []byte, hashCtx *HashCtx) (string, *HashCtx, error) {
	now := time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")

	// 构建 authMeta，如果 partNumber >= 2，需要包含 X-Oss-Hash-Ctx
	authMeta := fmt.Sprintf("PUT\n\n%s\n%s\n", mimeType, now)

	// partNumber >= 2 时需要包含 X-Oss-Hash-Ctx
	if partNumber >= 2 && hashCtx != nil {
		hashCtxStr, err := encodeHashCtx(hashCtx)
		if err != nil {
			return "", nil, fmt.Errorf("failed to encode hash ctx: %w", err)
		}
		authMeta += fmt.Sprintf("X-Oss-Hash-Ctx:%s\n", hashCtxStr)
	}

	authMeta += fmt.Sprintf("x-oss-date:%s\nx-oss-user-agent:aliyun-sdk-js/1.0.0 Chrome 145.0.0.0 on Windows 10 64-bit\n/%s/%s?partNumber=%d&uploadId=%s",
		now, pre.Data.Bucket, pre.Data.ObjKey, partNumber, pre.Data.UploadID)

	// 使用 client 方法获取 Authorization
	authKey, err := qc.getOSSAuthKey(authMeta, pre.Data.AuthInfo, pre.Data.TaskID)
	if err != nil {
		return "", nil, err
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
		HashCtx:   hashCtx, // partNumber >= 2 时需要
	}
	req, err := qc.newRequestWithHeaders("PUT", uploadURL, bytes.NewReader(chunkData), headerBuilder)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create upload request: %w", err)
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
		return "", nil, fmt.Errorf("failed to upload chunk: %w", err)
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
				// 注意：哈希上下文在上传循环中通过累积的SHA1哈希对象更新
				return etag, nil, nil
			}
		}

		return "", nil, fmt.Errorf("upload chunk failed with status %d: %s", resp.StatusCode, bodyStr)
	}

	// 从响应头获取 ETag
	etag := resp.Header.Get("ETag")
	if etag == "" {
		return "", nil, fmt.Errorf("no ETag in response")
	}

	// 注意：哈希上下文在上传循环中通过累积的SHA1哈希对象更新
	// 这里返回 nil，因为实际的更新在 UploadFile 函数中进行
	return etag, nil, nil
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
	authMeta := fmt.Sprintf("POST\n%s\napplication/xml\n%s\nx-oss-callback:%s\nx-oss-date:%s\nx-oss-user-agent:aliyun-sdk-js/1.0.0 Chrome 145.0.0.0 on Windows 10 64-bit\n/%s/%s?uploadId=%s",
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
// opts: 上传选项（可为 nil，使用默认行为）
func (qc *QuarkClient) UploadFile(filePath, destPath string, progressCallback func(*UploadProgress), opts *UploadOptions) (*StandardResponse, error) {
	// 解析选项，nil 安全
	var policy UploadPolicy
	if opts != nil {
		policy = opts.Policy
	}
	filePath = stripQuotes(filePath)
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

	// 去重策略检查：在 upPre 之前检查目标路径是否已存在同名文件
	if policy == UploadPolicySkip || policy == UploadPolicyRsync {
		existingInfo, existErr := qc.GetFileInfo(destPath)
		if existErr == nil && existingInfo != nil && existingInfo.Success {
			// 文件已存在
			switch policy {
			case UploadPolicySkip:
				return &StandardResponse{
					Success: true,
					Code:    "SKIPPED",
					Message: fmt.Sprintf("文件已存在，跳过上传: %s", destPath),
					Data:    existingInfo.Data,
				}, nil
			case UploadPolicyRsync:
				// 检查文件大小是否一致
				if existingInfo.Data != nil {
					var existingSize int64
					switch v := existingInfo.Data["size"].(type) {
					case float64:
						existingSize = int64(v)
					case int64:
						existingSize = v
					}
					if existingSize == fileSize {
						return &StandardResponse{
							Success: true,
							Code:    "SKIPPED",
							Message: fmt.Sprintf("文件大小相同，跳过上传: %s (%d bytes)", destPath, existingSize),
							Data:    existingInfo.Data,
						}, nil
					}
					// 大小不同，继续上传（覆盖）
				}
			}
		}
		// policy == UploadPolicyOverwrite 或文件不存在：继续上传
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
			pre.Metadata.PartThread = state.PartThread // 恢复并发线程数，确保 parallel 模式续传不退化

			// 验证 uploadId 是否仍然有效：尝试上传一个空分片或查询分片列表
			// 由于没有查询 API，我们直接尝试使用，如果失败再重新获取
			useSavedState = true
			savedState = state
			// HashCtx 会在后续的上传循环中从 savedState 恢复
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

	// upHash 确认上传会话：通过嵌入式哈希策略，在分片读取过程中同步计算 MD5+SHA1，
	// 之后调用 upHash 确认服务端上传生命周期（upPre → upHash → upCommit）。
	//
	// 【核心】upHash 必须被调用，否则服务端会清理未确认的 OSS 上传会话，
	// 导致 upCommit 时返回 404 NoSuchUpload。
	//
	// 【嵌入式策略（Round 17）】将 MD5+SHA1 计算嵌入分片读取流程（生产者/顺序路径），
	// 消除第二个文件句柄的 14GB 冗余读取，NFS 场景下断点续传启动延迟大幅降低。

	// parallelHashResult 在主线程内传递 upHash 结果
	type parallelHashResult struct {
		isRapid bool
		err     error
	}
	parallelHashCh := make(chan parallelHashResult, 1)

	// 嵌入式哈希对象：在分片读取过程中累积计算，所有分片处理完毕后提交 upHash
	embeddedMD5 := md5.New()
	embeddedSHA1 := sha1.New()

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

	// 并发数完全由服务端 part_thread 控制。
	// 当 upPre 请求含 parallel_upload=true 时，服务端启用并行 OSS 模式，
	// 返回 metadata.part_thread 作为并发数（通常为 3）。
	totalParts := int((fileSize + partSize - 1) / partSize)
	uploadParallel := pre.Metadata.PartThread
	if uploadParallel <= 0 {
		uploadParallel = 1 // 服务端未返回时退回单线程
	}
	if uploadParallel > totalParts {
		uploadParallel = totalParts
	}
	canUseParallel := uploadParallel > 1

	buildUploadState := func(currentHashCtx *HashCtx) *UploadState {
		return &UploadState{
			FilePath:      filePath,
			DestPath:      destPath,
			FileSize:      fileSize,
			UploadID:      pre.Data.UploadID,
			TaskID:        pre.Data.TaskID,
			Bucket:        pre.Data.Bucket,
			ObjKey:        pre.Data.ObjKey,
			UploadURL:     pre.Data.UploadURL,
			PartSize:      partSize,
			PartThread:    pre.Metadata.PartThread, // 保存并发线程数，断点续传恢复时需要
			UploadedParts: make(map[int]string),
			MimeType:      mimeType,
			AuthInfo:      pre.Data.AuthInfo,
			Callback:      pre.Data.Callback,
			HashCtx:       currentHashCtx,
		}
	}

	if canUseParallel {
		// === 并发上传路径（支持断点续传）===
		// 并发模式始终从文件头开始读，由生产者统一顺序读取并跳过已上传分片
		file.Seek(0, 0)

		// 提取已上传分片（断点续传场景）
		alreadyUploaded := make(map[int]string)
		if useSavedState && savedState != nil && len(savedState.UploadedParts) > 0 {
			for pn, etag := range savedState.UploadedParts {
				alreadyUploaded[pn] = etag
			}
		}

		// 构建新的上传状态，并预填充已上传分片
		savedState = buildUploadState(nil)
		for pn, etag := range alreadyUploaded {
			savedState.UploadedParts[pn] = etag
		}

		uploadedPartMap, uploadErr := qc.uploadPartsParallel(
			file,
			pre,
			mimeType,
			partSize,
			fileSize,
			statePath,
			savedState,
			startTime,
			progressCallback,
			uploadParallel,
			alreadyUploaded,
			embeddedMD5,
			embeddedSHA1,
		)
		if uploadErr != nil {
			return &StandardResponse{
				Success: false,
				Code:    "UPLOAD_PART_ERROR",
				Message: uploadErr.Error(),
				Data:    nil,
			}, nil
		}

		// 嵌入式哈希：所有分片已由生产者读取并累积哈希，提交 upHash
		md5Sum := fmt.Sprintf("%x", embeddedMD5.Sum(nil))
		sha1Sum := fmt.Sprintf("%x", embeddedSHA1.Sum(nil))
		hashResp, hashErr := qc.upHash(md5Sum, sha1Sum, pre.Data.TaskID)
		if hashErr != nil {
			parallelHashCh <- parallelHashResult{err: hashErr}
		} else {
			parallelHashCh <- parallelHashResult{isRapid: hashResp.Data.Finish}
		}

		etags = make([]string, totalParts)
		for i := 1; i <= totalParts; i++ {
			etag, ok := uploadedPartMap[i]
			if !ok {
				return &StandardResponse{
					Success: false,
					Code:    "UPLOAD_PART_ERROR",
					Message: fmt.Sprintf("parallel upload missing part %d", i),
					Data:    nil,
				}, nil
			}
			etags[i-1] = etag
		}
	} else {
		// === 顺序上传路径（后备逻辑，仅在 totalParts==1 或 uploadParallel==1 时触发）===

		// 用于计算速度和剩余时间
		var lastUpdateTime time.Time
		var lastUploaded int64

		// 如果从断点续传，计算已上传的字节数并跳过已上传的分片
		if useSavedState && startPartNumber > 1 {
			lastUploaded = int64(startPartNumber-1) * partSize
			if lastUploaded > fileSize {
				lastUploaded = fileSize
			}
			lastUpdateTime = time.Now()
			skipBytes := int64(startPartNumber-1) * partSize
			if skipBytes > 0 {
				file.Seek(skipBytes, 0)
			}
		}

		// 初始化累积的SHA1哈希对象
		var cumulativeHash hash.Hash
		var hashCtx *HashCtx
		var processedBytes int64

		if useSavedState && startPartNumber > 1 {
			if savedState.HashCtx != nil {
				hashCtx = savedState.HashCtx
				file.Seek(0, 0)
				cumulativeHash = sha1.New()
				processedBytes = 0
				for i := 1; i < startPartNumber; i++ {
					chunk := make([]byte, partSize)
					n, err := file.Read(chunk)
					if err != nil && err != io.EOF {
						return &StandardResponse{
							Success: false,
							Code:    "READ_FILE_ERROR",
							Message: fmt.Sprintf("failed to read file chunk for hash calculation: %v", err),
							Data:    nil,
						}, nil
					}
					if n > 0 {
						cumulativeHash.Write(chunk[:n])
						processedBytes += int64(n)
					}
				}
				file.Seek(processedBytes, 0)
			} else {
				file.Seek(0, 0)
				cumulativeHash = sha1.New()
				processedBytes = 0
				for i := 1; i < startPartNumber; i++ {
					chunk := make([]byte, partSize)
					n, err := file.Read(chunk)
					if err != nil && err != io.EOF {
						return &StandardResponse{
							Success: false,
							Code:    "READ_FILE_ERROR",
							Message: fmt.Sprintf("failed to read file chunk for hash calculation: %v", err),
							Data:    nil,
						}, nil
					}
					if n > 0 {
						cumulativeHash.Write(chunk[:n])
						processedBytes += int64(n)
					}
				}
				hashCtx, _ = updateHashCtxFromHash(cumulativeHash, []byte{}, processedBytes)
				file.Seek(processedBytes, 0)
			}
		} else {
			cumulativeHash = sha1.New()
			processedBytes = 0
			hashCtx = nil
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
				if savedState == nil {
					savedState = buildUploadState(hashCtx)
				} else {
					savedState.HashCtx = hashCtx
					if savedState.UploadedParts == nil {
						savedState.UploadedParts = make(map[int]string)
					}
				}
				// 保存已上传的分片
				for i, etag := range etags {
					savedState.UploadedParts[i+1] = etag
				}
				_ = saveUploadState(statePath, savedState)

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

			// 嵌入式哈希：顺序路径也在每个分片读取后累积 MD5+SHA1
			embeddedMD5.Write(chunk)
			embeddedSHA1.Write(chunk)

			// 上传分片（partNumber >= 2 时需要传递 hashCtx）
			var currentHashCtx *HashCtx
			if partNumber >= 2 {
				currentHashCtx = hashCtx
			}

			etag, _, err := qc.upPart(pre, mimeType, partNumber, chunk, currentHashCtx)
			if err != nil {
				// 上传失败，保存当前状态以便断点续传
				if savedState == nil {
					savedState = buildUploadState(hashCtx)
				} else {
					savedState.HashCtx = hashCtx
					if savedState.UploadedParts == nil {
						savedState.UploadedParts = make(map[int]string)
					}
				}
				// 保存已上传的分片
				for i, uploadedEtag := range etags {
					savedState.UploadedParts[i+1] = uploadedEtag
				}
				_ = saveUploadState(statePath, savedState)

				return &StandardResponse{
					Success: false,
					Code:    "UPLOAD_PART_ERROR",
					Message: fmt.Sprintf("failed to upload part %d: %v", partNumber, err),
					Data:    nil,
				}, nil
			}

			etags = append(etags, etag)

			// 更新累积的SHA1哈希对象和HashCtx（为下一个分片准备）
			if cumulativeHash != nil {
				hashCtx, _ = updateHashCtxFromHash(cumulativeHash, chunk, processedBytes)
				processedBytes += int64(len(chunk))
			}

			// 更新上传状态
			if savedState == nil {
				savedState = buildUploadState(hashCtx)
			} else {
				savedState.HashCtx = hashCtx
				if savedState.UploadedParts == nil {
					savedState.UploadedParts = make(map[int]string)
				}
			}
			savedState.UploadedParts[partNumber] = etag
			// 每上传一个分片后保存状态
			_ = saveUploadState(statePath, savedState)

			// 更新进度
			if progressCallback != nil {
				uploaded := int64(len(etags)) * partSize
				progressInfo := buildUploadProgressInfo(
					uploaded,
					fileSize,
					startTime,
					&lastUpdateTime,
					&lastUploaded,
				)
				progressCallback(progressInfo)
			}

			partNumber++
		}

		// 嵌入式哈希：顺序路径所有分片读取完毕，提交 upHash
		md5Sum := fmt.Sprintf("%x", embeddedMD5.Sum(nil))
		sha1Sum := fmt.Sprintf("%x", embeddedSHA1.Sum(nil))
		hashResp, hashErr := qc.upHash(md5Sum, sha1Sum, pre.Data.TaskID)
		if hashErr != nil {
			parallelHashCh <- parallelHashResult{err: hashErr}
		} else {
			parallelHashCh <- parallelHashResult{isRapid: hashResp.Data.Finish}
		}
	}

	// 10. 嵌入式哈希完成后，检查 upHash 结果
	// 目的：确保 upHash 在 upCommit 之前被调用（服务端协议要求）
	if parallelHashCh != nil {
		hashResult := <-parallelHashCh
		if hashResult.err != nil {
			// upHash 失败：记录但不中断（降级处理，继续 commit 尝试）
			// 在正常网络条件下不应进入此分支
			_ = hashResult.err
		} else if hashResult.isRapid {
			// 秒传：upHash 告知服务端文件已存在，直接走 upFinish 跳过 commit
			deleteUploadState(statePath)
			finishResp, err := qc.upFinish(pre)
			if err != nil {
				return &StandardResponse{
					Success: false,
					Code:    "FINISH_UPLOAD_ERROR",
					Message: fmt.Sprintf("finish upload failed: %v", err),
					Data:    nil,
				}, nil
			}
			responseData := make(map[string]interface{})
			for k, v := range finishResp.Data {
				if k != "preview_url" {
					responseData[k] = v
				}
			}
			return &StandardResponse{
				Success: true,
				Code:    "OK",
				Message: "上传完成（秒传）",
				Data:    responseData,
			}, nil
		}
		// isRapid=false：服务端确认需要正常上传，继续走 commit 流程
	}

	// 10. 提交上传
	finish, err := qc.upCommit(pre, etags)
	if err != nil {
		// NoSuchUpload 说明 OSS 端的 uploadId 已失效（可能已过期或被清理），
		// 必须删除断点续传状态文件，避免重试时反复使用同一个过期 uploadId 导致死循环
		if strings.Contains(err.Error(), "NoSuchUpload") {
			deleteUploadState(statePath)
		}
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
	folderName = stripQuotes(folderName)
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
	newName = stripQuotes(newName)

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
	dirPath = normalizePath(dirPath)
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
	req.Header.Set("x-oss-user-agent", "aliyun-sdk-js/1.0.0 Chrome 145.0.0.0 on Windows 10 64-bit")

	// 如果存在 HashCtx，设置 X-Oss-Hash-Ctx header
	if b.HashCtx != nil {
		hashCtxStr, err := encodeHashCtx(b.HashCtx)
		if err == nil {
			req.Header.Set("X-Oss-Hash-Ctx", hashCtxStr)
		}
	}

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
	req.Header.Set("x-oss-user-agent", "aliyun-sdk-js/1.0.0 Chrome 145.0.0.0 on Windows 10 64-bit")
	return nil
}

// GetDownloadURL 获取文件的下载链接（支持同步与异步，大文件为异步任务会轮询直到拿到 URL）
// fid: 文件ID
// 返回: 下载链接URL
func (qc *QuarkClient) GetDownloadURL(fid string) (string, error) {
	data := map[string]interface{}{
		"fids": []string{fid},
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("failed to marshal download request: %w", err)
	}
	respMap, err := qc.makeRequest("POST", FILE_DOWNLOAD, bytes.NewBuffer(jsonData), nil)
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "23018") || strings.Contains(errStr, "download file size limit") {
			return "", fmt.Errorf("超过文件下载大小限制，请使用客户端下载")
		}
		return "", fmt.Errorf("download request failed: %w", err)
	}
	code, _ := respMap["code"].(float64)
	status, _ := respMap["status"].(float64)
	if int(code) != 0 || int(status) != 200 {
		return "", fmt.Errorf("download failed: code=%v, status=%v", code, status)
	}
	rawData := respMap["data"]
	if rawData == nil {
		return "", fmt.Errorf("download response data is empty")
	}
	// 同步：data 为数组，直接带 download_url
	if arr, ok := rawData.([]interface{}); ok && len(arr) > 0 {
		if first, ok := arr[0].(map[string]interface{}); ok {
			if u, _ := first["download_url"].(string); u != "" {
				return u, nil
			}
		}
	}
	// 异步：data 为对象，含 task_id / task_sync / task_resp
	if obj, ok := rawData.(map[string]interface{}); ok {
		taskID, _ := obj["task_id"].(string)
		_, _ = obj["task_sync"].(bool)
		if taskResp, _ := obj["task_resp"].(map[string]interface{}); taskResp != nil {
			if dataArr, _ := taskResp["data"].([]interface{}); len(dataArr) > 0 {
				if first, _ := dataArr[0].(map[string]interface{}); first != nil {
					if u, _ := first["download_url"].(string); u != "" {
						return u, nil
					}
				}
			}
		}
		if taskID != "" {
			return qc.waitForDownloadTaskComplete(taskID)
		}
	}
	return "", fmt.Errorf("download response data is empty or invalid")
}

// waitForDownloadTaskComplete 轮询下载任务直到完成，返回 download_url
func (qc *QuarkClient) waitForDownloadTaskComplete(taskID string) (string, error) {
	const maxRetries = 60
	retryInterval := 2 * time.Second
	for i := 0; i < maxRetries; i++ {
		time.Sleep(retryInterval)
		queryParams := url.Values{}
		queryParams.Set("task_id", taskID)
		queryParams.Set("retry_index", "0")
		reqURL := qc.baseURL + TASK + "?" + queryParams.Encode()
		respMap, err := qc.makeRequest("GET", reqURL, nil, nil)
		if err != nil {
			return "", fmt.Errorf("query download task failed: %w", err)
		}
		rawData := respMap["data"]
		if rawData == nil {
			continue
		}
		data, ok := rawData.(map[string]interface{})
		if !ok {
			continue
		}
		status, _ := data["status"].(float64)
		if status == 3 {
			return "", fmt.Errorf("download task failed")
		}
		if status == 2 {
			if u, _ := data["download_url"].(string); u != "" {
				return u, nil
			}
			if arr, _ := data["data"].([]interface{}); len(arr) > 0 {
				if first, _ := arr[0].(map[string]interface{}); first != nil {
					if u, _ := first["download_url"].(string); u != "" {
						return u, nil
					}
				}
			}
		}
	}
	return "", fmt.Errorf("download task timeout after %d retries", maxRetries)
}

// DownloadProgress 下载进度回调参数
type DownloadProgress struct {
	Downloaded int64 // 已下载字节数
	Total      int64 // 总字节数，-1 表示未知
}

// DownloadFile 将文件下载到本地
// fid: 文件ID；destPath: 本地路径（文件或目录，为目录时使用 fileName 作为文件名）；fileName: 远程文件名（当 destPath 为目录时使用）
// progressCallback: 进度回调，可为 nil
func (qc *QuarkClient) DownloadFile(fid, destPath, fileName string, progressCallback func(*DownloadProgress)) error {
	downloadURL, err := qc.GetDownloadURL(fid)
	if err != nil {
		return err
	}
	// 若目标为目录或以分隔符结尾，则保存为 destPath/fileName
	path := destPath
	if path == "" || path == "." {
		path = fileName
	} else if info, err := os.Stat(path); err == nil && info.IsDir() {
		path = filepath.Join(path, fileName)
	} else if strings.HasSuffix(path, "/") || strings.HasSuffix(path, string(filepath.Separator)) {
		path = filepath.Join(path, fileName)
	}
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create local dir: %w", err)
		}
	}
	out, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create local file: %w", err)
	}
	defer out.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36")
	cookieParts := make([]string, 0, len(qc.cookies))
	for k, v := range qc.cookies {
		cookieParts = append(cookieParts, fmt.Sprintf("%s=%s", k, v))
	}
	if len(cookieParts) > 0 {
		req.Header.Set("Cookie", strings.Join(cookieParts, "; "))
	}

	client := &http.Client{
		Timeout: 2 * time.Hour,
		Transport: &http.Transport{
			// 禁用 HTTP/2，与主客户端保持一致
			TLSNextProto: make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("download failed: status %d, body: %s", resp.StatusCode, string(body))
	}
	var total int64 = -1
	if resp.ContentLength >= 0 {
		total = resp.ContentLength
	}
	var written int64
	buf := make([]byte, 32*1024)
	for {
		nr, errRead := resp.Body.Read(buf)
		if nr > 0 {
			nw, errWrite := out.Write(buf[:nr])
			written += int64(nw)
			if errWrite != nil {
				return fmt.Errorf("write file: %w", errWrite)
			}
			if progressCallback != nil {
				progressCallback(&DownloadProgress{Downloaded: written, Total: total})
			}
		}
		if errRead == io.EOF {
			break
		}
		if errRead != nil {
			return fmt.Errorf("read body: %w", errRead)
		}
	}
	return nil
}
