package sdk

import (
	"fmt"
	"net/url"
)

// GetUserInfo 获取用户信息（含容量）
// 先调用 /account/info 获取昵称头像，再调用 /1/clouddrive/member 获取容量和会员信息，
// 两者合并后返回。
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
	success, _ := jsonResp["success"].(bool)
	message, _ := jsonResp["msg"].(string)
	code, _ := jsonResp["code"].(string)
	if !success {
		return &StandardResponse{
			Success: success,
			Code:    code,
			Message: fmt.Sprintf("API returned: %s", message),
		}, nil
	}

	data, _ := jsonResp["data"].(map[string]interface{})
	if data == nil {
		data = make(map[string]interface{})
	}

	// 额外请求 member API 补全容量和会员信息
	memberData, memberErr := qc.getMemberInfo()
	if memberErr == nil && memberData != nil {
		// 将容量和会员字段合并到 data 中
		if v, ok := memberData["use_capacity"]; ok {
			data["use_capacity"] = v
		}
		if v, ok := memberData["total_capacity"]; ok {
			data["total_capacity"] = v
		}
		if v, ok := memberData["member_type"]; ok {
			data["member_type"] = v
		}
		if v, ok := memberData["super_vip_exp_at"]; ok {
			data["super_vip_exp_at"] = v
		}
	}

	return &StandardResponse{
		Success: success,
		Code:    code,
		Message: "get user info success",
		Data:    data,
	}, nil
}

// getMemberInfo 获取会员和容量信息
// 调用 DRIVE_DOMAIN + MEMBER_INFO（/1/clouddrive/member）
// 返回包含 use_capacity、total_capacity、member_type 等字段的 data map
func (qc *QuarkClient) getMemberInfo() (map[string]interface{}, error) {
	reqURL := DRIVE_DOMAIN + MEMBER_INFO

	parsedURL, err := url.Parse(reqURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse member URL: %w", err)
	}

	query := parsedURL.Query()
	query.Set("pr", "ucpro")
	query.Set("fr", "pc")
	query.Set("fetch_subscribe", "true")
	query.Set("fetch_identity", "true")
	parsedURL.RawQuery = query.Encode()
	reqURL = parsedURL.String()

	// 跳过认证检查（此函数可能在 checkAuth 链路中被调用）
	jsonResp, err := qc.makeRequest("GET", reqURL, nil, nil, true)
	if err != nil {
		return nil, fmt.Errorf("member request failed: %w", err)
	}

	// 检查响应状态
	respCode, _ := jsonResp["code"].(float64)
	if respCode != 0 {
		return nil, fmt.Errorf("member API error: code=%.0f", respCode)
	}

	data, ok := jsonResp["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("member API: no data field")
	}

	return data, nil
}
