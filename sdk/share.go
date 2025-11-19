package sdk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// GetShareInfo 从文本中提取分享ID和提取码
// text: 包含分享链接和/或提取码的文本
// 返回分享信息和错误
func (qc *QuarkClient) GetShareInfo(text string) (*ShareInfo, error) {
	// 提取pwd_id
	// 匹配格式: /s/(\w+)(#/list/share.*/(\w+))?
	re := regexp.MustCompile(`/s/(\w+)(#/list/share.*/(\w+))?`)
	match := re.FindStringSubmatch(text)
	if len(match) < 2 {
		return nil, fmt.Errorf("链接格式错误")
	}

	pwdID := match[1]

	// 提取提取码
	// 匹配格式: 提取码[:：](\S+\d{1,4}\S*)
	reCode := regexp.MustCompile(`提取码[:：](\S+\d{1,4}\S*)`)
	matchCode := reCode.FindStringSubmatch(text)
	passcode := ""
	if len(matchCode) >= 2 {
		passcode = matchCode[1]
	}

	return &ShareInfo{
		PwdID:    pwdID,
		Passcode: passcode,
	}, nil
}

// GetShareStoken 获取分享stoken
// pwdID: 分享链接ID
// passcode: 提取码，默认空
// 返回stoken数据和错误
func (qc *QuarkClient) GetShareStoken(pwdID, passcode string) (map[string]interface{}, error) {
	// 生成随机数和时间戳
	rand.Seed(time.Now().UnixNano())
	dt := rand.Intn(900) + 100 // 100-999
	t := time.Now().UnixMilli()

	queryParams := url.Values{}
	queryParams.Set("pr", "ucpro")
	queryParams.Set("fr", "pc")
	queryParams.Set("uc_param_str", "")
	queryParams.Set("__dt", fmt.Sprintf("%d", dt))
	queryParams.Set("__t", fmt.Sprintf("%d", t))

	data := map[string]interface{}{
		"pwd_id":                            pwdID,
		"passcode":                          passcode,
		"support_visit_limit_private_share": true,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request data: %w", err)
	}

	// 使用 DRIVE_H_DOMAIN 作为 baseURL
	reqURL := DRIVE_H_DOMAIN + SHARE_SHAREPAGE_TOKEN + "?" + queryParams.Encode()
	respMap, err := qc.makeRequest("POST", reqURL, bytes.NewBuffer(jsonData), nil)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	var stokenResp ShareStokenResponse
	if err := qc.parseResponse(respMap, &stokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if stokenResp.Code != 0 || stokenResp.Status != 200 {
		return nil, fmt.Errorf("get share stoken failed: code=%d, status=%d", stokenResp.Code, stokenResp.Status)
	}

	return stokenResp.Data, nil
}

// GetShareList 获取分享列表
// pwdID: 分享链接ID
// stoken: 分享stoken
// pdirFid: 目录ID，默认"0"（根目录）
// page: 页码，默认1
// size: 每页数量，默认50
// sortBy: 排序字段，"file_name" 或 "updated_at"，默认"file_name"
// sortOrder: 排序方式，"asc" 或 "desc"，默认"asc"
// 返回分享列表数据和错误
func (qc *QuarkClient) GetShareList(pwdID, stoken, pdirFid string, page, size int, sortBy, sortOrder string) (map[string]interface{}, error) {
	// 验证排序字段
	if sortBy != "file_name" && sortBy != "updated_at" {
		return nil, fmt.Errorf("sort_by 只能为 'file_name' 或 'updated_at'")
	}

	// 构建排序字符串
	sort := fmt.Sprintf("file_type:asc,%s:%s", sortBy, sortOrder)

	// 生成随机数和时间戳
	rand.Seed(time.Now().UnixNano())
	dt := rand.Intn(900) + 100 // 100-999
	t := time.Now().UnixMilli()

	queryParams := url.Values{}
	queryParams.Set("pr", "ucpro")
	queryParams.Set("fr", "pc")
	queryParams.Set("uc_param_str", "")
	queryParams.Set("pwd_id", pwdID)
	queryParams.Set("stoken", stoken)
	queryParams.Set("pdir_fid", pdirFid)
	queryParams.Set("force", "0")
	queryParams.Set("_page", fmt.Sprintf("%d", page))
	queryParams.Set("_size", fmt.Sprintf("%d", size))
	queryParams.Set("_fetch_banner", "1")
	queryParams.Set("_fetch_share", "1")
	queryParams.Set("_fetch_total", "1")
	queryParams.Set("_sort", sort)
	queryParams.Set("__dt", fmt.Sprintf("%d", dt))
	queryParams.Set("__t", fmt.Sprintf("%d", t))

	reqURL := DRIVE_H_DOMAIN + SHARE_SHAREPAGE_DETAIL + "?" + queryParams.Encode()
	respMap, err := qc.makeRequest("GET", reqURL, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	var listResp ShareListResponse
	if err := qc.parseResponse(respMap, &listResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if listResp.Code != 0 || listResp.Status != 200 {
		return nil, fmt.Errorf("get share list failed: code=%d, status=%d", listResp.Code, listResp.Status)
	}

	return listResp.Data, nil
}

// SaveShareFile 转存指定文件
// pwdID: 分享链接ID
// stoken: 分享stoken
// fidList: 要转存的文件fid列表，全部保存则为空列表
// shareTokenList: 与fidList对应的share_fid_token列表，全部保存则为空列表
// toPdirFid: 目标父目录fid，默认为"0"（根目录）
// pdirSaveAll: 是否全部保存，默认true
// 返回转存结果数据和错误
func (qc *QuarkClient) SaveShareFile(pwdID, stoken string, fidList, shareTokenList []string, toPdirFid string, pdirSaveAll bool) (map[string]interface{}, error) {
	// 生成随机数和时间戳
	rand.Seed(time.Now().UnixNano())
	dt := rand.Intn(900) + 100 // 100-999
	t := time.Now().UnixMilli()

	queryParams := url.Values{}
	queryParams.Set("pr", "ucpro")
	queryParams.Set("fr", "pc")
	queryParams.Set("uc_param_str", "")
	queryParams.Set("__dt", fmt.Sprintf("%d", dt))
	queryParams.Set("__t", fmt.Sprintf("%d", t))

	data := map[string]interface{}{
		"fid_list":         fidList,
		"share_token_list": shareTokenList,
		"to_pdir_fid":      toPdirFid,
		"pwd_id":           pwdID,
		"stoken":           stoken,
		"pdir_fid":         "0",
		"pdir_save_all":    pdirSaveAll,
		"exclude_fids":     []string{},
		"scene":            "link",
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request data: %w", err)
	}

	reqURL := DRIVE_DOMAIN + SHARE_SHAREPAGE_SAVE + "?" + queryParams.Encode()
	respMap, err := qc.makeRequest("POST", reqURL, bytes.NewBuffer(jsonData), nil)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	var saveResp SaveShareFileResponse
	if err := qc.parseResponse(respMap, &saveResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if saveResp.Code != 0 || saveResp.Status != 200 {
		return nil, fmt.Errorf("save share file failed: code=%d, status=%d", saveResp.Code, saveResp.Status)
	}

	return saveResp.Data, nil
}

// CreateShare 创建文件/文件夹分享链接
// filePath: 文件或文件夹路径
// expireDays: 有效期天数，0=永久有效，1=1天，7=7天，30=30天
// needPasscode: 是否需要提取码，true表示需要（服务端自动生成），false表示不需要
// 返回分享链接信息和错误
func (qc *QuarkClient) CreateShare(filePath string, expireDays int, needPasscode bool) (*ShareLinkInfo, error) {
	// 获取文件信息
	fileInfo, err := qc.GetFileInfo(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	// 构建请求数据
	// 根据实际API，参数名是 expired_type
	// expired_type值：1=永久有效，2=1天，3=7天，4=30天
	// url_type值：1=不需要提取码，2=需要提取码
	data := map[string]interface{}{
		"fid_list": []string{fileInfo.Data["fid"].(string)},
		"title":    fileInfo.Data["file_name"].(string),
		"url_type": 1, // 默认不需要提取码
	}

	// 根据needPasscode设置url_type
	if needPasscode {
		data["url_type"] = 2 // 需要提取码
	} else {
		data["url_type"] = 1 // 不需要提取码
	}

	// 设置有效期类型
	// 1=永久有效，2=1天，3=7天，4=30天
	switch expireDays {
	case 0:
		// 永久有效
		data["expired_type"] = 1
	case 1:
		// 1天
		data["expired_type"] = 2
	case 7:
		// 7天
		data["expired_type"] = 3
	case 30:
		// 30天
		data["expired_type"] = 4
	default:
		// 其他天数，根据范围选择
		if expireDays <= 7 {
			data["expired_type"] = 3 // 7天
		} else {
			data["expired_type"] = 4 // 30天
		}
	}
	// 如果需要提取码，生成一个4位随机提取码
	// 注意：只有当url_type=2时才需要传递passcode参数
	var generatedPasscode string
	if needPasscode {
		rand.Seed(time.Now().UnixNano())
		chars := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
		var code strings.Builder
		for i := 0; i < 4; i++ {
			code.WriteByte(chars[rand.Intn(len(chars))])
		}
		generatedPasscode = code.String()
		data["passcode"] = generatedPasscode
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request data: %w", err)
	}

	respMap, err := qc.makeRequest("POST", SHARE, bytes.NewBuffer(jsonData), nil)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	var shareResp struct {
		Code   int `json:"code"`
		Status int `json:"status"`
		Data   struct {
			TaskID   string `json:"task_id"`
			TaskSync bool   `json:"task_sync"`
			TaskResp struct {
				Data struct {
					ShareID string `json:"share_id"`
				} `json:"data"`
			} `json:"task_resp"`
		} `json:"data"`
	}

	if err := qc.parseResponse(respMap, &shareResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if shareResp.Code != 0 || shareResp.Status != 200 {
		return nil, fmt.Errorf("create share failed: code=%d, status=%d", shareResp.Code, shareResp.Status)
	}

	// 提取 task_id 和 share_id
	taskID := shareResp.Data.TaskID
	shareID := shareResp.Data.TaskResp.Data.ShareID

	// 如果 task_sync 为 false 或者 share_id 为空，需要轮询任务状态
	if !shareResp.Data.TaskSync || shareID == "" {
		// 轮询任务状态直到完成
		var err error
		shareID, err = qc.waitForTaskComplete(taskID)
		if err != nil {
			return nil, fmt.Errorf("wait for task complete failed: %w", err)
		}
	}

	// 调用 /share/password 接口获取分享链接和提取码
	shareLinkInfo, err := qc.GetShareLink(shareID)
	if err != nil {
		return nil, err
	}

	// 如果生成了提取码，验证password接口返回的提取码
	// 注意：password接口返回的提取码就是我们提交给share接口的提取码
	if needPasscode && generatedPasscode != "" {
		if shareLinkInfo.Passcode == "" {
			return nil, fmt.Errorf("提取码异常：已生成提取码(%s)但password接口未返回提取码", generatedPasscode)
		}
		if shareLinkInfo.Passcode != generatedPasscode {
			return nil, fmt.Errorf("提取码异常：password接口返回的提取码(%s)与生成的提取码(%s)不一致", shareLinkInfo.Passcode, generatedPasscode)
		}
	}

	return shareLinkInfo, nil
}

// waitForTaskComplete 轮询任务状态直到完成
// taskID: 任务ID
// 返回share_id和错误
func (qc *QuarkClient) waitForTaskComplete(taskID string) (string, error) {
	maxRetries := 10
	retryInterval := 500 * time.Millisecond

	for i := 0; i < maxRetries; i++ {
		time.Sleep(retryInterval)

		queryParams := url.Values{}
		queryParams.Set("task_id", taskID)
		queryParams.Set("retry_index", "0")

		reqURL := qc.baseURL + TASK + "?" + queryParams.Encode()
		respMap, err := qc.makeRequest("GET", reqURL, nil, nil)
		if err != nil {
			return "", fmt.Errorf("query task status failed: %w", err)
		}

		var taskResp struct {
			Code   int `json:"code"`
			Status int `json:"status"`
			Data   struct {
				Status  int    `json:"status"` // 2表示完成
				ShareID string `json:"share_id"`
			} `json:"data"`
		}

		if err := qc.parseResponse(respMap, &taskResp); err != nil {
			return "", fmt.Errorf("failed to decode task response: %w", err)
		}

		if taskResp.Data.Status == 2 && taskResp.Data.ShareID != "" {
			return taskResp.Data.ShareID, nil
		}

		// 如果任务还在进行中，继续等待
		if taskResp.Data.Status == 1 {
			continue
		}

		// 如果任务失败
		if taskResp.Data.Status == 3 {
			return "", fmt.Errorf("task failed")
		}
	}

	return "", fmt.Errorf("task timeout after %d retries", maxRetries)
}

// GetShareLink 通过share_id获取分享链接
// shareID: 分享ID（从CreateShare返回）
// 返回分享链接信息和错误
func (qc *QuarkClient) GetShareLink(shareID string) (*ShareLinkInfo, error) {
	data := map[string]interface{}{
		"share_id": shareID,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request data: %w", err)
	}

	respMap, err := qc.makeRequest("POST", SHARE_PASSWORD, bytes.NewBuffer(jsonData), nil)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	var linkResp struct {
		Code   int `json:"code"`
		Status int `json:"status"`
		Data   struct {
			ShareURL  string      `json:"share_url"`
			PwdID     string      `json:"pwd_id"`
			Passcode  interface{} `json:"passcode"`   // 可能是字符串或不存在
			ExpiredAt interface{} `json:"expired_at"` // 可能是int64或float64（毫秒时间戳）
		} `json:"data"`
	}

	if err := qc.parseResponse(respMap, &linkResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if linkResp.Code != 0 || linkResp.Status != 200 {
		return nil, fmt.Errorf("get share link failed: code=%d, status=%d", linkResp.Code, linkResp.Status)
	}

	shareLinkInfo := &ShareLinkInfo{
		ShareURL: linkResp.Data.ShareURL,
		PwdID:    linkResp.Data.PwdID,
	}

	// 提取过期时间（可能是毫秒时间戳）
	if expiredAt, ok := linkResp.Data.ExpiredAt.(float64); ok {
		shareLinkInfo.ExpiresAt = int64(expiredAt)
	} else if expiredAt, ok := linkResp.Data.ExpiredAt.(int64); ok {
		shareLinkInfo.ExpiresAt = expiredAt
	}

	// 提取码可能不存在（如果没有设置提取码）
	// 使用类型断言来安全地获取 passcode
	if passcode, ok := linkResp.Data.Passcode.(string); ok && passcode != "" {
		shareLinkInfo.Passcode = passcode
	}
	// 注意：如果 password 接口不返回提取码，会在 CreateShare 方法中验证并返回错误

	return shareLinkInfo, nil
}

// SetSharePassword 设置分享提取码
// pwdID: 分享ID
// passcode: 提取码
// 返回错误
func (qc *QuarkClient) SetSharePassword(pwdID, passcode string) error {
	data := map[string]interface{}{
		"pwd_id":   pwdID,
		"passcode": passcode,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal request data: %w", err)
	}

	respMap, err := qc.makeRequest("POST", SHARE_PASSWORD, bytes.NewBuffer(jsonData), nil)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}

	var passwordResp struct {
		Code   int `json:"code"`
		Status int `json:"status"`
	}

	if err := qc.parseResponse(respMap, &passwordResp); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if passwordResp.Code != 0 || passwordResp.Status != 200 {
		return fmt.Errorf("set share password failed: code=%d, status=%d", passwordResp.Code, passwordResp.Status)
	}

	return nil
}
