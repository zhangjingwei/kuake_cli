package sdk

import (
	"fmt"
	"net/url"
)

// GetUserInfo 获取用户信息
// 返回标准响应结构
func (qc *QuarkClient) GetUserInfo() (*StandardResponse, error) {
	// 构建完整 URL（使用 PAN_DOMAIN，不是 baseURL）
	reqURL := PAN_DOMAIN + USER_INFO

	// 解析 URL 并添加查询参数
	parsedURL, err := url.Parse(reqURL)
	if err != nil {
		return &StandardResponse{
			Success: false,
			Code:    "URL_PARSE_ERROR",
			Message: fmt.Sprintf("failed to parse URL: %v", err),
			Data:    nil,
		}, nil
	}

	// 添加查询参数
	query := parsedURL.Query()
	query.Set("fr", "pc")
	query.Set("platform", "pc")
	parsedURL.RawQuery = query.Encode()
	reqURL = parsedURL.String()

	// 使用 makeRequest 发起请求，跳过认证检查（避免死锁，因为 checkAuth 会调用 GetUserInfo）
	jsonResp, err := qc.makeRequest("GET", reqURL, nil, nil, true)

	if err != nil {
		return &StandardResponse{
			Success: false,
			Code:    "REQUEST_ERROR",
			Message: fmt.Sprintf("request failed: %v", err),
			Data:    nil,
		}, nil
	}

	// 检查 success 字段
	success, ok := jsonResp["success"].(bool)
	message, ok := jsonResp["msg"].(string)
	code, _ := jsonResp["code"].(string)
	if ok && !success {
		return &StandardResponse{
			Success: success,
			Code:    code,
			Message: fmt.Sprintf("API returned: %s", message),
		}, nil
	} else {
		return &StandardResponse{
			Success: success,
			Code:    code,
			Message: "get user info success",
			Data:    jsonResp["data"].(map[string]interface{}),
		}, nil
	}
}
