package sdk

import (
	"testing"
)

func TestGetUserInfo(t *testing.T) {
	// 注意：这个测试需要真实的配置和网络连接
	// 在实际测试中，应该使用mock HTTP客户端
	// 这里提供一个基础测试框架

	t.Skip("Skipping test that requires network access. Use integration tests instead.")

	client := createTestClient(t)
	if client == nil {
		t.Fatal("Failed to create test client")
	}

	// 这个测试需要真实的API调用
	// 在实际项目中，应该mock HTTP响应
	response, err := client.GetUserInfo()
	if err != nil {
		t.Logf("GetUserInfo() returned error (expected if no network/config): %v", err)
		return
	}

	if response == nil {
		t.Error("GetUserInfo() returned nil response")
		return
	}

	// 验证响应结构
	if response.Success && response.Data == nil {
		t.Error("GetUserInfo() returned success but no data")
	}
}

// TestGetUserInfo_ErrorHandling 测试错误处理
func TestGetUserInfo_ErrorHandling(t *testing.T) {
	t.Skip("Skipping test that requires network access. Use integration tests instead.")

	client := createTestClient(t)
	if client == nil {
		t.Fatal("Failed to create test client")
	}

	// 注意：GetUserInfo 使用 PAN_DOMAIN 而不是 baseURL
	// 这个测试主要用于验证错误处理逻辑
	// 在实际场景中，错误可能来自网络问题或无效的token
	response, err := client.GetUserInfo()
	if err != nil {
		// 如果有错误，这是预期的（可能因为网络或配置问题）
		t.Logf("GetUserInfo() returned error (expected in test environment): %v", err)
		return
	}

	// 如果没有错误，验证响应结构
	if response == nil {
		t.Error("GetUserInfo() returned nil response")
		return
	}

	// 如果响应不成功，这也是可以接受的（可能是token无效等）
	if !response.Success {
		t.Logf("GetUserInfo() returned unsuccessful response (may be expected): %s", response.Message)
	}
}

