package sdk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// NewQuarkClient 创建夸克网盘客户端（支持多个 token）
// configPath: 配置文件路径，如果为空则使用默认路径 DEFAULT_CONFIG_PATH
func NewQuarkClient(configPath string) *QuarkClient {
	// 加载配置文件
	config, err := LoadConfig(configPath)
	if err != nil {
		panic("failed to load config file")
	}

	accessTokens := config.Quark.AccessTokens

	if len(accessTokens) == 0 {
		panic("at least one access token is required")
	}

	// 随机选择一个 token 作为初始 token
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	initialIdx := rng.Intn(len(accessTokens))
	initialToken := accessTokens[initialIdx]

	// 从环境变量读取调试开关
	// 0 关闭，1 开启
	debugEnv := os.Getenv("KUake_DEBUG")
	isDebugEnv := debugEnv == "1"

	client := &QuarkClient{
		baseURL:          DRIVE_DOMAIN,    // 使用 DRIVE_DOMAIN 常量
		accessToken:      initialToken,    // 当前使用的 token
		accessTokens:     accessTokens,    // 所有可用的 tokens
		currentTokenIdx:  initialIdx,      // 当前 token 索引
		authCheckTimeout: 5 * time.Minute, // 默认5分钟内缓存认证检查结果
		failedTokens:     make(map[int]bool),
		Debug:            isDebugEnv, // 从环境变量读取，默认关闭
		HttpClient: &http.Client{
			Timeout: 30 * time.Second, // 普通 API 请求的超时时间，上传请求使用动态超时
		},
	}
	// 解析 cookie
	client.cookies = client.parseCookie(initialToken)
	return client
}

// SetBaseURL 设置自定义 API 基础 URL
func (qc *QuarkClient) SetBaseURL(baseURL string) {
	qc.baseURL = baseURL
}

// GetCookies 获取解析后的 cookie 字典
func (qc *QuarkClient) GetCookies() map[string]string {
	return qc.cookies
}

// parseCookie 解析 cookie 字符串为字典
// 参考 Python 的 SimpleCookie 实现
func (qc *QuarkClient) parseCookie(cookieStr string) map[string]string {
	cookies := make(map[string]string)

	// 按分号分割 cookie
	cookieParts := splitCookieString(cookieStr)

	for _, part := range cookieParts {
		part = trimSpace(part)
		if part == "" {
			continue
		}

		// 查找等号位置
		eqIndex := -1
		for i, char := range part {
			if char == '=' {
				eqIndex = i
				break
			}
		}

		if eqIndex == -1 {
			continue
		}

		key := trimSpace(part[:eqIndex])
		value := trimSpace(part[eqIndex+1:])

		if key != "" {
			cookies[key] = value
		}
	}

	return cookies
}

// splitCookieString 分割 cookie 字符串，处理引号内的分号
func splitCookieString(s string) []string {
	var parts []string
	var current strings.Builder
	inQuotes := false

	for _, char := range s {
		if char == '"' {
			inQuotes = !inQuotes
			current.WriteRune(char)
		} else if char == ';' && !inQuotes {
			parts = append(parts, current.String())
			current.Reset()
		} else {
			current.WriteRune(char)
		}
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

// trimSpace 去除字符串两端的空白字符
func trimSpace(s string) string {
	return strings.TrimSpace(s)
}

// switchToNextToken 切换到下一个可用的 token
func (qc *QuarkClient) switchToNextToken() error {
	qc.failedTokensMutex.Lock()
	defer qc.failedTokensMutex.Unlock()

	// 标记当前 token 为失败
	qc.failedTokens[qc.currentTokenIdx] = true

	// 查找下一个可用的 token
	for i := 0; i < len(qc.accessTokens); i++ {
		nextIdx := (qc.currentTokenIdx + 1 + i) % len(qc.accessTokens)
		if !qc.failedTokens[nextIdx] {
			// 找到可用的 token，切换
			qc.currentTokenIdx = nextIdx
			qc.accessToken = qc.accessTokens[nextIdx]
			qc.cookies = qc.parseCookie(qc.accessToken)
			// 重置认证缓存
			qc.authCheckValid = false
			return nil
		}
	}

	// 所有 token 都已失败
	return fmt.Errorf("all access tokens have failed")
}

// checkAuth 检查用户登录状态
// 如果缓存有效则直接返回，否则调用 GetUserInfo 检查
// 如果认证失败，自动切换到下一个 token
func (qc *QuarkClient) checkAuth() error {
	qc.authCheckMutex.RLock()
	// 检查缓存是否有效
	if qc.authCheckValid && time.Since(qc.lastAuthCheck) < qc.authCheckTimeout {
		qc.authCheckMutex.RUnlock()
		return nil // 缓存有效，直接返回
	}
	qc.authCheckMutex.RUnlock()

	// 缓存无效或过期，重新检查
	qc.authCheckMutex.Lock()
	defer qc.authCheckMutex.Unlock()

	// 双重检查，避免并发时重复请求
	if qc.authCheckValid && time.Since(qc.lastAuthCheck) < qc.authCheckTimeout {
		return nil
	}

	// 调用 GetUserInfo 检查登录状态
	userInfoResp, err := qc.GetUserInfo()
	if err != nil {
		qc.authCheckValid = false
		return err
	}

	// 检查 StandardResponse 的 Success 字段
	if !userInfoResp.Success {
		qc.authCheckValid = false

		// 如果有多个 token，尝试切换到下一个
		if len(qc.accessTokens) > 1 {
			if switchErr := qc.switchToNextToken(); switchErr != nil {
				return fmt.Errorf("authentication failed: all tokens invalid")
			}
			// 切换成功，重新尝试认证
			retryResp, retryErr := qc.GetUserInfo()
			if retryErr != nil {
				return retryErr
			}
			// 检查重试后的 StandardResponse
			if !retryResp.Success {
				return fmt.Errorf("authentication failed after token switch")
			}
			// 重新认证成功，更新缓存
			qc.authCheckValid = true
			qc.lastAuthCheck = time.Now()
			return nil
		}

		return fmt.Errorf("authentication failed")
	}

	// 更新缓存
	qc.authCheckValid = true
	qc.lastAuthCheck = time.Now()
	return nil
}

// makeRequest 发起 HTTP 请求并解析 JSON 响应
// urlOrEndpoint: 可以是完整 URL（以 http:// 或 https:// 开头）或相对路径 endpoint
// 如果是完整 URL，直接使用；如果是相对路径，会拼接 baseURL 并添加查询参数 pr=ucpro&fr=pc
// skipAuth: 是否跳过认证检查（用于避免死锁，当 checkAuth 调用 GetUserInfo 时使用）
// 返回解析后的 JSON 数据（map[string]interface{}）和错误
func (qc *QuarkClient) makeRequest(method, urlOrEndpoint string, body io.Reader, headers map[string]string, skipAuth ...bool) (map[string]interface{}, error) {
	// 在请求前检查用户登录状态（除非明确跳过）
	shouldSkipAuth := len(skipAuth) > 0 && skipAuth[0]
	if !shouldSkipAuth {
		if err := qc.checkAuth(); err != nil {
			return nil, err
		}
	}

	var reqURL string
	// 判断是完整 URL 还是相对路径
	if strings.HasPrefix(urlOrEndpoint, "http://") || strings.HasPrefix(urlOrEndpoint, "https://") {
		// 完整 URL，直接使用
		reqURL = urlOrEndpoint
	} else {
		// 相对路径，拼接 baseURL
		reqURL = qc.baseURL + urlOrEndpoint

		// 添加基础查询参数
		parsedURL, err := url.Parse(reqURL)
		if err != nil {
			return nil, fmt.Errorf("invalid URL: %w", err)
		}

		query := parsedURL.Query()
		query.Set("pr", "ucpro")
		query.Set("fr", "pc")
		parsedURL.RawQuery = query.Encode()
		reqURL = parsedURL.String()
	}

	req, err := http.NewRequest(method, reqURL, body)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	// 设置默认 headers（参考浏览器实际请求）
	// 将 cookie map 转换为字符串格式: "key1=value1; key2=value2"
	cookieParts := make([]string, 0, len(qc.cookies))
	for k, v := range qc.cookies {
		cookieParts = append(cookieParts, fmt.Sprintf("%s=%s", k, v))
	}
	req.Header.Set("Cookie", strings.Join(cookieParts, "; "))
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Priority", "u=1, i")
	req.Header.Set("Referer", "https://pan.quark.cn/list")
	req.Header.Set("Sec-Ch-Ua", `"Chromium";v="142", "Google Chrome";v="142", "Not_A Brand";v="99"`)
	req.Header.Set("Sec-Ch-Ua-Arch", `"x86"`)
	req.Header.Set("Sec-Ch-Ua-Bitness", `"64"`)
	req.Header.Set("Sec-Ch-Ua-Full-Version", `"142.0.7444.163"`)
	req.Header.Set("Sec-Ch-Ua-Full-Version-List", `"Chromium";v="142.0.7444.163", "Google Chrome";v="142.0.7444.163", "Not_A Brand";v="99.0.0.0"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Model", `""`)
	req.Header.Set("Sec-Ch-Ua-Platform", `"Windows"`)
	req.Header.Set("Sec-Ch-Ua-Platform-Version", `"19.0.0"`)
	req.Header.Set("Sec-Ch-Ua-Wow64", "?0")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36")
	req.Header.Set("Origin", "https://pan.quark.cn")

	// 只在有 body 时设置 Content-Type
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// 设置自定义 headers
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := qc.HttpClient.Do(req)
	if err != nil {
		// 检查是否是超时错误
		if strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "deadline exceeded") {
			return nil, fmt.Errorf("request timeout")
		}
		// 检查是否是 DNS 解析错误
		if strings.Contains(err.Error(), "no such host") || strings.Contains(err.Error(), "lookup") {
			return nil, fmt.Errorf("DNS resolution failed")
		}
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应体
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response failed: %w", err)
	}

	// 如果开启调试，输出请求和响应信息
	if qc.Debug {
		fmt.Printf("\n[调试] 请求: %s %s\n", method, reqURL)
		fmt.Printf("[调试] 状态码: %d\n", resp.StatusCode)
		fmt.Printf("[调试] 响应内容: %s\n\n", string(bodyBytes))
	}

	// 检查HTTP状态码，如果>=400表示请求失败
	// 尝试解析响应体获取具体错误信息
	if resp.StatusCode >= 400 {
		// 尝试解析响应体为JSON，提取错误消息
		var errorResp map[string]interface{}
		if err := json.Unmarshal(bodyBytes, &errorResp); err == nil {
			// 成功解析JSON，尝试提取message字段
			if msg, ok := errorResp["message"].(string); ok && msg != "" {
				return nil, fmt.Errorf("status %d: %s", resp.StatusCode, msg)
			}
			// 如果没有message字段，尝试提取errmsg字段
			if msg, ok := errorResp["errmsg"].(string); ok && msg != "" {
				return nil, fmt.Errorf("status %d: %s", resp.StatusCode, msg)
			}
			// 如果都没有，尝试提取code字段
			if code, ok := errorResp["code"].(float64); ok {
				return nil, fmt.Errorf("status %d, code %.0f", resp.StatusCode, code)
			}
		}
		// 如果无法解析JSON或没有找到错误消息，返回原始响应体（限制长度）
		bodyStr := string(bodyBytes)
		if len(bodyStr) > 500 {
			bodyStr = bodyStr[:500] + "..."
		}
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, bodyStr)
	}

	// 解析JSON响应体
	var jsonResp map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &jsonResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return jsonResp, nil
}

// parseResponse 将 map[string]interface{} 转换为指定的结构体
func (qc *QuarkClient) parseResponse(respMap map[string]interface{}, target interface{}) error {
	jsonData, err := json.Marshal(respMap)
	if err != nil {
		return fmt.Errorf("marshal failed: %w", err)
	}
	if err := json.Unmarshal(jsonData, target); err != nil {
		return fmt.Errorf("unmarshal failed: %w", err)
	}
	return nil
}

// ConvertToFileInfo 将 QuarkFileInfo 转换为 FileInfo
func (qc *QuarkClient) ConvertToFileInfo(qf QuarkFileInfo) *FileInfo {
	return &FileInfo{
		Name:        qf.Name,
		Path:        qf.Path,
		Size:        qf.Size,
		ModTime:     time.Unix(qf.ModifyTime, 0),
		IsDirectory: qf.IsDirectory,
	}
}

// getOSSAuthKey 获取 OSS 请求的 Authorization Key
// authMeta: OSS 请求的认证元数据字符串
// authInfo: 从预上传响应中获取的 auth_info
// taskID: 任务ID
// 返回 Authorization Key 和错误
func (qc *QuarkClient) getOSSAuthKey(authMeta string, authInfo json.RawMessage, taskID string) (string, error) {
	authData := map[string]interface{}{
		"auth_info": authInfo,
		"auth_meta": authMeta,
		"task_id":   taskID,
	}

	jsonData, err := json.Marshal(authData)
	if err != nil {
		return "", fmt.Errorf("marshal auth data failed: %w", err)
	}

	respMap, err := qc.makeRequest("POST", FILE_UPLOAD_AUTH, bytes.NewBuffer(jsonData), nil)
	if err != nil {
		return "", err
	}

	var authResp AuthResponse
	if err := qc.parseResponse(respMap, &authResp); err != nil {
		return "", err
	}

	if authResp.Code != 0 || authResp.Status != 200 {
		return "", fmt.Errorf("auth failed: code=%d", authResp.Code)
	}

	return authResp.Data.AuthKey, nil
}

// newRequestWithHeaders 创建请求并设置头部（通过多态处理）
// method: HTTP 方法
// url: 请求 URL
// body: 请求体
// headerBuilder: 头部构建器，如果为 nil 则使用默认的 API 请求头部
// 返回 *http.Request 和错误
func (qc *QuarkClient) newRequestWithHeaders(method, url string, body io.Reader, headerBuilder RequestHeaderBuilder) (*http.Request, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	// 如果提供了自定义头部构建器，使用它；否则使用默认的 API 请求头部
	if headerBuilder != nil {
		if err := headerBuilder.BuildHeaders(req, qc); err != nil {
			return nil, fmt.Errorf("build headers failed: %w", err)
		}
	} else {
		// 默认的 API 请求头部
		qc.setDefaultAPIHeaders(req)
	}

	return req, nil
}

// setDefaultAPIHeaders 设置默认的 API 请求头部
func (qc *QuarkClient) setDefaultAPIHeaders(req *http.Request) {
	// 将 cookie map 转换为字符串格式
	cookieParts := make([]string, 0, len(qc.cookies))
	for k, v := range qc.cookies {
		cookieParts = append(cookieParts, fmt.Sprintf("%s=%s", k, v))
	}
	req.Header.Set("Cookie", strings.Join(cookieParts, "; "))
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Priority", "u=1, i")
	req.Header.Set("Referer", "https://pan.quark.cn/list")
	req.Header.Set("Sec-Ch-Ua", `"Chromium";v="142", "Google Chrome";v="142", "Not_A Brand";v="99"`)
	req.Header.Set("Sec-Ch-Ua-Arch", `"x86"`)
	req.Header.Set("Sec-Ch-Ua-Bitness", `"64"`)
	req.Header.Set("Sec-Ch-Ua-Full-Version", `"142.0.7444.163"`)
	req.Header.Set("Sec-Ch-Ua-Full-Version-List", `"Chromium";v="142.0.7444.163", "Google Chrome";v="142.0.7444.163", "Not_A Brand";v="99.0.0.0"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Model", `""`)
	req.Header.Set("Sec-Ch-Ua-Platform", `"Windows"`)
	req.Header.Set("Sec-Ch-Ua-Platform-Version", `"19.0.0"`)
	req.Header.Set("Sec-Ch-Ua-Wow64", "?0")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36")
	req.Header.Set("Origin", "https://pan.quark.cn")

	if req.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
}
