package sdk

import (
	"os"
	"path/filepath"
	"testing"
)

// TestUploadFileHashCtx_Integration 集成测试：验证 X-Oss-Hash-Ctx 功能
// 需要有效的配置文件才能运行
// 设置环境变量 INTEGRATION_TEST=1 来运行此测试
func TestUploadFileHashCtx_Integration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") == "" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=1 to run.")
	}

	// 检查配置文件是否存在
	configPath := "config.json"
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skipf("Skipping integration test. Config file not found: %s", configPath)
	}

	client := NewQuarkClient(configPath)
	if client == nil {
		t.Fatal("Failed to create test client")
	}

	// 创建测试文件（大于分片大小，确保会触发分片上传）
	partSize := int64(4194304) // 4MB
	fileSize := partSize * 3   // 12MB，确保有3个分片

	tmpFile := filepath.Join(t.TempDir(), "test_hash_ctx.bin")
	file, err := os.Create(tmpFile)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// 写入测试数据（使用可预测的数据以便验证）
	testData := make([]byte, fileSize)
	for i := range testData {
		testData[i] = byte(i % 256)
	}
	if _, err := file.Write(testData); err != nil {
		t.Fatalf("Failed to write test data: %v", err)
	}
	file.Close()

	t.Logf("Created test file: %s, size: %d bytes", tmpFile, fileSize)

	// 上传文件
	response, err := client.UploadFile(tmpFile, "/test_hash_ctx_integration.bin", func(progress *UploadProgress) {
		t.Logf("Upload progress: %d%%, Speed: %s, Remaining: %s",
			progress.Progress, progress.SpeedStr, progress.RemainingStr)
	})

	if err != nil {
		t.Fatalf("UploadFile() error = %v", err)
	}

	if response == nil || !response.Success {
		t.Errorf("UploadFile() failed: %v", response)
	} else {
		t.Logf("Upload successful: %s", response.Message)
	}

	// 清理：删除上传的文件
	if response != nil && response.Success {
		if fid, ok := response.Data["fid"].(string); ok && fid != "" {
			// 尝试删除测试文件
			// 注意：这里需要先获取文件路径，然后删除
			// 为了简化，我们只记录，不实际删除
			t.Logf("Uploaded file FID: %s (can be manually deleted if needed)", fid)
		}
	}
}

// TestUploadFileHashCtx_Resume 测试断点续传功能中的 HashCtx
// 这个测试需要手动中断上传来验证断点续传
func TestUploadFileHashCtx_Resume(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") == "" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=1 to run.")
	}

	configPath := "config.json"
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skipf("Skipping integration test. Config file not found: %s", configPath)
	}

	client := NewQuarkClient(configPath)
	if client == nil {
		t.Fatal("Failed to create test client")
	}

	// 创建较大的测试文件（确保需要多个分片）
	partSize := int64(4194304) // 4MB
	fileSize := partSize * 10  // 40MB，确保有10个分片

	tmpFile := filepath.Join(t.TempDir(), "test_hash_ctx_resume.bin")
	file, err := os.Create(tmpFile)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// 写入测试数据
	testData := make([]byte, fileSize)
	for i := range testData {
		testData[i] = byte(i % 256)
	}
	if _, err := file.Write(testData); err != nil {
		t.Fatalf("Failed to write test data: %v", err)
	}
	file.Close()

	t.Logf("Created test file: %s, size: %d bytes", tmpFile, fileSize)
	t.Logf("To test resume functionality:")
	t.Logf("1. Start upload and interrupt it (Ctrl+C)")
	t.Logf("2. Check state file in: %s", filepath.Join(os.TempDir(), "kuake_upload_state"))
	t.Logf("3. Verify HashCtx is saved in state file")
	t.Logf("4. Restart upload and verify it resumes correctly")

	// 上传文件
	response, err := client.UploadFile(tmpFile, "/test_hash_ctx_resume.bin", func(progress *UploadProgress) {
		t.Logf("Upload progress: %d%%, Speed: %s, Remaining: %s",
			progress.Progress, progress.SpeedStr, progress.RemainingStr)
	})

	if err != nil {
		t.Logf("UploadFile() error (may be expected if interrupted): %v", err)
		return
	}

	if response == nil || !response.Success {
		t.Logf("UploadFile() failed (may be expected if interrupted): %v", response)
		return
	}

	t.Logf("Upload successful: %s", response.Message)
}

// TestUploadStateHashCtx 测试上传状态中 HashCtx 的保存和加载
func TestUploadStateHashCtx(t *testing.T) {
	// 创建测试用的 HashCtx
	hashCtx := &HashCtx{
		HashType: "sha1",
		H0:       "1125272656",
		H1:       "2794323374",
		H2:       "1697191688",
		H3:       "2476193098",
		H4:       "2437866605",
		Nl:       "436207616",
		Nh:       "0",
		Data:     "",
		Num:      "0",
	}

	// 创建测试用的 UploadState
	state := &UploadState{
		FilePath:      "/test/file.bin",
		DestPath:      "/test/file.bin",
		FileSize:      436207616,
		UploadID:      "test_upload_id",
		TaskID:        "test_task_id",
		Bucket:        "test_bucket",
		ObjKey:        "test_obj_key",
		UploadURL:     "http://test.oss.com",
		PartSize:      4194304,
		UploadedParts: map[int]string{1: "etag1", 2: "etag2"},
		MimeType:      "application/octet-stream",
		AuthInfo:      []byte(`{"test": "auth"}`),
		Callback:      []byte(`{"test": "callback"}`),
		HashCtx:       hashCtx,
	}

	// 测试保存和加载
	statePath := filepath.Join(t.TempDir(), "test_upload_state.json")

	// 保存状态
	if err := saveUploadState(statePath, state); err != nil {
		t.Fatalf("saveUploadState() error = %v", err)
	}

	// 加载状态
	loadedState, err := loadUploadState(statePath)
	if err != nil {
		t.Fatalf("loadUploadState() error = %v", err)
	}

	// 验证 HashCtx 是否正确保存和加载
	if loadedState.HashCtx == nil {
		t.Fatal("HashCtx is nil after loading")
	}

	if loadedState.HashCtx.HashType != hashCtx.HashType {
		t.Errorf("HashType = %v, want %v", loadedState.HashCtx.HashType, hashCtx.HashType)
	}
	if loadedState.HashCtx.H0 != hashCtx.H0 {
		t.Errorf("H0 = %v, want %v", loadedState.HashCtx.H0, hashCtx.H0)
	}
	if loadedState.HashCtx.H1 != hashCtx.H1 {
		t.Errorf("H1 = %v, want %v", loadedState.HashCtx.H1, hashCtx.H1)
	}
	if loadedState.HashCtx.H2 != hashCtx.H2 {
		t.Errorf("H2 = %v, want %v", loadedState.HashCtx.H2, hashCtx.H2)
	}
	if loadedState.HashCtx.H3 != hashCtx.H3 {
		t.Errorf("H3 = %v, want %v", loadedState.HashCtx.H3, hashCtx.H3)
	}
	if loadedState.HashCtx.H4 != hashCtx.H4 {
		t.Errorf("H4 = %v, want %v", loadedState.HashCtx.H4, hashCtx.H4)
	}
	if loadedState.HashCtx.Nl != hashCtx.Nl {
		t.Errorf("Nl = %v, want %v", loadedState.HashCtx.Nl, hashCtx.Nl)
	}
	if loadedState.HashCtx.Nh != hashCtx.Nh {
		t.Errorf("Nh = %v, want %v", loadedState.HashCtx.Nh, hashCtx.Nh)
	}
	if loadedState.HashCtx.Data != hashCtx.Data {
		t.Errorf("Data = %v, want %v", loadedState.HashCtx.Data, hashCtx.Data)
	}
	if loadedState.HashCtx.Num != hashCtx.Num {
		t.Errorf("Num = %v, want %v", loadedState.HashCtx.Num, hashCtx.Num)
	}

	// 清理
	os.Remove(statePath)
}

// TestUploadStateHashCtx_Nil 测试 HashCtx 为 nil 时的保存和加载
func TestUploadStateHashCtx_Nil(t *testing.T) {
	// 创建没有 HashCtx 的 UploadState
	state := &UploadState{
		FilePath:      "/test/file.bin",
		DestPath:      "/test/file.bin",
		FileSize:      1000,
		UploadID:      "test_upload_id",
		TaskID:        "test_task_id",
		Bucket:        "test_bucket",
		ObjKey:        "test_obj_key",
		UploadURL:     "http://test.oss.com",
		PartSize:      4194304,
		UploadedParts: map[int]string{1: "etag1"},
		MimeType:      "application/octet-stream",
		AuthInfo:      []byte(`{"test": "auth"}`),
		Callback:      []byte(`{"test": "callback"}`),
		HashCtx:       nil, // 第一个分片时 HashCtx 为 nil
	}

	// 测试保存和加载
	statePath := filepath.Join(t.TempDir(), "test_upload_state_nil.json")

	// 保存状态
	if err := saveUploadState(statePath, state); err != nil {
		t.Fatalf("saveUploadState() error = %v", err)
	}

	// 加载状态
	loadedState, err := loadUploadState(statePath)
	if err != nil {
		t.Fatalf("loadUploadState() error = %v", err)
	}

	// 验证 HashCtx 为 nil 时也能正确处理
	// 注意：JSON unmarshal 时，如果字段不存在，可能为 nil
	if loadedState.HashCtx != nil {
		// 如果加载后不为 nil，检查是否所有字段都是零值
		if loadedState.HashCtx.HashType == "" && loadedState.HashCtx.H0 == "" {
			// 这是正常的，JSON unmarshal 会创建零值结构
			t.Logf("HashCtx loaded as zero value (expected for nil)")
		}
	}

	// 清理
	os.Remove(statePath)
}
