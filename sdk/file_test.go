package sdk

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeRootDir(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "normalize root slash",
			path: "/",
			want: "0",
		},
		{
			name: "normalize empty string",
			path: "",
			want: "0",
		},
		{
			name: "normalize dot",
			path: ".",
			want: "0",
		},
		{
			name: "keep normal path",
			path: "/test/path",
			want: "/test/path",
		},
		{
			name: "keep fid",
			path: "1234567890",
			want: "1234567890",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeRootDir(tt.path)
			if got != tt.want {
				t.Errorf("normalizeRootDir() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCreateFolder(t *testing.T) {
	t.Skip("Skipping test that requires network access. Use integration tests instead.")

	client := createTestClient(t)
	if client == nil {
		t.Fatal("Failed to create test client")
	}

	tests := []struct {
		name      string
		folderName string
		pdirFid   string
		wantErr   bool
	}{
		{
			name:      "create folder in root",
			folderName: "test_folder",
			pdirFid:   "/",
			wantErr:   false,
		},
		{
			name:      "create folder with empty name",
			folderName: "",
			pdirFid:   "/",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := client.CreateFolder(tt.folderName, tt.pdirFid)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateFolder() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && response != nil && !response.Success {
				t.Errorf("CreateFolder() returned unsuccessful response: %s", response.Message)
			}
		})
	}
}

func TestList(t *testing.T) {
	t.Skip("Skipping test that requires network access. Use integration tests instead.")

	client := createTestClient(t)
	if client == nil {
		t.Fatal("Failed to create test client")
	}

	tests := []struct {
		name    string
		dirPath string
		wantErr bool
	}{
		{
			name:    "list root directory",
			dirPath: "/",
			wantErr: false,
		},
		{
			name:    "list subdirectory",
			dirPath: "/test",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := client.List(tt.dirPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("List() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && response != nil && !response.Success {
				t.Logf("List() returned unsuccessful response (may be expected): %s", response.Message)
			}
		})
	}
}

func TestGetFileInfo(t *testing.T) {
	t.Skip("Skipping test that requires network access. Use integration tests instead.")

	client := createTestClient(t)
	if client == nil {
		t.Fatal("Failed to create test client")
	}

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "get file info for root",
			path:    "/",
			wantErr: false,
		},
		{
			name:    "get file info for non-existent file",
			path:    "/nonexistent_file.txt",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := client.GetFileInfo(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetFileInfo() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && response != nil && !response.Success {
				t.Logf("GetFileInfo() returned unsuccessful response (may be expected): %s", response.Message)
			}
		})
	}
}

func TestDelete(t *testing.T) {
	t.Skip("Skipping test that requires network access. Use integration tests instead.")

	client := createTestClient(t)
	if client == nil {
		t.Fatal("Failed to create test client")
	}

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "delete file",
			path:    "/test_file.txt",
			wantErr: false,
		},
		{
			name:    "delete non-existent file",
			path:    "/nonexistent_file.txt",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := client.Delete(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("Delete() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && response != nil && !response.Success {
				t.Logf("Delete() returned unsuccessful response (may be expected): %s", response.Message)
			}
		})
	}
}

func TestMove(t *testing.T) {
	t.Skip("Skipping test that requires network access. Use integration tests instead.")

	client := createTestClient(t)
	if client == nil {
		t.Fatal("Failed to create test client")
	}

	tests := []struct {
		name    string
		srcPath string
		destPath string
		wantErr bool
	}{
		{
			name:    "move file",
			srcPath: "/source.txt",
			destPath: "/dest/",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := client.Move(tt.srcPath, tt.destPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("Move() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && response != nil && !response.Success {
				t.Logf("Move() returned unsuccessful response (may be expected): %s", response.Message)
			}
		})
	}
}

func TestCopy(t *testing.T) {
	t.Skip("Skipping test that requires network access. Use integration tests instead.")

	client := createTestClient(t)
	if client == nil {
		t.Fatal("Failed to create test client")
	}

	tests := []struct {
		name    string
		srcPath string
		destPath string
		wantErr bool
	}{
		{
			name:    "copy file",
			srcPath: "/source.txt",
			destPath: "/dest/",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := client.Copy(tt.srcPath, tt.destPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("Copy() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && response != nil && !response.Success {
				t.Logf("Copy() returned unsuccessful response (may be expected): %s", response.Message)
			}
		})
	}
}

func TestRename(t *testing.T) {
	t.Skip("Skipping test that requires network access. Use integration tests instead.")

	client := createTestClient(t)
	if client == nil {
		t.Fatal("Failed to create test client")
	}

	tests := []struct {
		name    string
		oldPath string
		newName string
		wantErr bool
	}{
		{
			name:    "rename file",
			oldPath: "/old_name.txt",
			newName: "new_name.txt",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := client.Rename(tt.oldPath, tt.newName)
			if (err != nil) != tt.wantErr {
				t.Errorf("Rename() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && response != nil && !response.Success {
				t.Logf("Rename() returned unsuccessful response (may be expected): %s", response.Message)
			}
		})
	}
}

func TestUploadFile(t *testing.T) {
	t.Skip("Skipping test that requires network access. Use integration tests instead.")

	client := createTestClient(t)
	if client == nil {
		t.Fatal("Failed to create test client")
	}

	// 创建临时测试文件
	tmpFile := filepath.Join(t.TempDir(), "test_upload.txt")
	if err := os.WriteFile(tmpFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	tests := []struct {
		name    string
		filePath string
		destPath string
		wantErr bool
	}{
		{
			name:    "upload file",
			filePath: tmpFile,
			destPath: "/test_upload.txt",
			wantErr: false,
		},
		{
			name:    "upload non-existent file",
			filePath: "/nonexistent/file.txt",
			destPath: "/test.txt",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := client.UploadFile(tt.filePath, tt.destPath, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("UploadFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && response != nil && !response.Success {
				t.Logf("UploadFile() returned unsuccessful response (may be expected): %s", response.Message)
			}
		})
	}
}

