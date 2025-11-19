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
	if !ok || !success {
		return &StandardResponse{
			Success: false,
			Code:    "INVALID_SUCCESS",
			Message: fmt.Sprintf("API returned error: success=%v", success),
			Data:    nil,
		}, nil
	}

	// 检查 code 字段
	code, _ := jsonResp["code"].(string)
	if code != "OK" {
		return &StandardResponse{
			Success: false,
			Code:    code,
			Message: fmt.Sprintf("API returned error: code=%s", code),
			Data:    nil,
		}, nil
	}

	// 检查 data 字段
	data, ok := jsonResp["data"].(map[string]interface{})
	if !ok {
		return &StandardResponse{
			Success: false,
			Code:    "INVALID_DATA_FORMAT",
			Message: "invalid data format in response",
			Data:    nil,
		}, nil
	}

	// 检查数据是否为空
	if len(data) == 0 {
		return &StandardResponse{
			Success: false,
			Code:    "EMPTY_DATA",
			Message: "用户数据为空，获取用户信息失败",
			Data:    nil,
		}, nil
	}

	// 所有检查通过，返回成功响应
	return &StandardResponse{
		Success: true,
		Code:    "OK",
		Message: "获取用户信息成功",
		Data:    data,
	}, nil
}
