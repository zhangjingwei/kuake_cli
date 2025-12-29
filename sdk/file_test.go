package sdk

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "normalize Windows path",
			path: "d:\\a\\b\\c",
			want: "d:/a/b/c",
		},
		{
			name: "normalize mixed separators",
			path: "d:/a\\b/c",
			want: "d:/a/b/c",
		},
		{
			name: "normalize multiple slashes",
			path: "//a//b//c",
			want: "/a/b/c",
		},
		{
			name: "remove trailing slash",
			path: "/a/b/",
			want: "/a/b",
		},
		{
			name: "keep root slash",
			path: "/",
			want: "/",
		},
		{
			name: "normalize empty string",
			path: "",
			want: "",
		},
		{
			name: "normalize single backslash",
			path: "\\",
			want: "/",
		},
		{
			name: "normalize Windows path with file",
			path: "d:\\a.mkv",
			want: "d:/a.mkv",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizePath(tt.path)
			if got != tt.want {
				t.Errorf("normalizePath() = %v, want %v", got, tt.want)
			}
		})
	}
}

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
			name: "normalize Windows backslash root",
			path: "\\",
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
		{
			name: "normalize Windows path to Unix style",
			path: "d:\\a\\b",
			want: "d:/a/b",
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
				t.Errorf("List() returned unsuccessful response: %s", response.Message)
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
			name:    "get file info",
			path:    "/test_file.txt",
			wantErr: false,
		},
		{
			name:    "get non-existent file info",
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

// TestPathNormalizationInFunctions 测试各个函数中的路径标准化处理
// 这些测试验证函数是否正确处理 Windows 风格的路径
func TestPathNormalizationInFunctions(t *testing.T) {
	tests := []struct {
		name     string
		testFunc func(*testing.T)
	}{
		{
			name: "test Copy function path normalization",
			testFunc: func(t *testing.T) {
				testPaths := []struct {
					input  string
					expect string
				}{
					{"d:\\source\\file.txt", "d:/source/file.txt"},
					{"d:/source/file.txt", "d:/source/file.txt"},
					{"/source/file.txt", "/source/file.txt"},
					{"\\source\\file.txt", "/source/file.txt"},
				}
				
				for _, tt := range testPaths {
					normalized := normalizePath(tt.input)
					if normalized != tt.expect {
						t.Errorf("normalizePath(%q) = %q, want %q", tt.input, normalized, tt.expect)
					}
				}
			},
		},
		{
			name: "test Move function path normalization",
			testFunc: func(t *testing.T) {
				testPaths := []struct {
					src    string
					dest   string
					srcExp string
					destExp string
				}{
					{"d:\\source\\file.txt", "d:\\dest\\", "d:/source/file.txt", "d:/dest"},
					{"d:/source/file.txt", "d:/dest/", "d:/source/file.txt", "d:/dest"},
					{"/source/file.txt", "/dest/", "/source/file.txt", "/dest"},
					{"\\source\\file.txt", "\\dest\\", "/source/file.txt", "/dest"},
				}
				
				for _, tt := range testPaths {
					srcNorm := normalizePath(tt.src)
					destNorm := normalizePath(tt.dest)
					if srcNorm != tt.srcExp {
						t.Errorf("normalizePath(src=%q) = %q, want %q", tt.src, srcNorm, tt.srcExp)
					}
					if destNorm != tt.destExp {
						t.Errorf("normalizePath(dest=%q) = %q, want %q", tt.dest, destNorm, tt.destExp)
					}
				}
			},
		},
		{
			name: "test Rename function path normalization",
			testFunc: func(t *testing.T) {
				testPaths := []struct {
					oldPath string
					expect  string
				}{
					{"d:\\old\\file.txt", "d:/old/file.txt"},
					{"d:/old/file.txt", "d:/old/file.txt"},
					{"/old/file.txt", "/old/file.txt"},
					{"\\old\\file.txt", "/old/file.txt"},
				}
				
				for _, tt := range testPaths {
					normalized := normalizePath(tt.oldPath)
					if normalized != tt.expect {
						t.Errorf("normalizePath(%q) = %q, want %q", tt.oldPath, normalized, tt.expect)
					}
				}
			},
		},
		{
			name: "test Delete function path normalization",
			testFunc: func(t *testing.T) {
				testPaths := []struct {
					path   string
					expect string
				}{
					{"d:\\file.txt", "d:/file.txt"},
					{"d:/file.txt", "d:/file.txt"},
					{"/file.txt", "/file.txt"},
					{"\\file.txt", "/file.txt"},
					{"d:\\folder\\subfolder\\file.txt", "d:/folder/subfolder/file.txt"},
				}
				
				for _, tt := range testPaths {
					normalized := normalizePath(tt.path)
					if normalized != tt.expect {
						t.Errorf("normalizePath(%q) = %q, want %q", tt.path, normalized, tt.expect)
					}
				}
			},
		},
		{
			name: "test filepath.Join result normalization",
			testFunc: func(t *testing.T) {
				testCases := []struct {
					base   string
					name   string
					expect string
				}{
					{"/parent", "file.txt", "/parent/file.txt"},
					{"d:/parent", "file.txt", "d:/parent/file.txt"},
					{"parent", "file.txt", "parent/file.txt"},
					{"/", "file.txt", "/file.txt"},
				}
				
				for _, tt := range testCases {
					joined := filepath.Join(tt.base, tt.name)
					normalized := normalizePath(joined)
					if normalized != tt.expect {
						t.Errorf("normalizePath(filepath.Join(%q, %q)) = %q, want %q", 
							tt.base, tt.name, normalized, tt.expect)
					}
				}
			},
		},
		{
			name: "test parent path extraction",
			testFunc: func(t *testing.T) {
				testCases := []struct {
					path     string
					expected string
				}{
					{"/a/b/c.txt", "/a/b"},
					{"/a/b/", "/a"}, // 标准化后是 /a/b，父目录是 /a // 标准化后是 /a/b，父目录是 /a
					{"/a.txt", "/"},
					{"/", "/"},
					{"a.txt", "/"},
					{"d:/a/b/c.txt", "d:/a/b"},
					{"d:\\a\\b\\c.txt", "d:/a/b"},
				}
				
				for _, tt := range testCases {
					normalized := normalizePath(tt.path)
					parentPath := normalized
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
					
					if parentPath != tt.expected {
						t.Errorf("extractParentPath(%q) = %q, want %q (normalized: %q)", 
							tt.path, parentPath, tt.expected, normalized)
					}
				}
			},
		},
		{
			name: "test mixed path separators",
			testFunc: func(t *testing.T) {
				testCases := []struct {
					input  string
					expect string
				}{
					{"d:/a\\b/c", "d:/a/b/c"},
					{"d:\\a/b\\c", "d:/a/b/c"},
					{"/a\\b/c", "/a/b/c"},
					{"\\a/b\\c", "/a/b/c"},
				}
				
				for _, tt := range testCases {
					normalized := normalizePath(tt.input)
					if normalized != tt.expect {
						t.Errorf("normalizePath(%q) = %q, want %q", tt.input, normalized, tt.expect)
					}
				}
			},
		},
		{
			name: "test multiple consecutive slashes",
			testFunc: func(t *testing.T) {
				testCases := []struct {
					input  string
					expect string
				}{
					{"//a//b//c", "/a/b/c"},
					{"d://a//b//c", "d:/a/b/c"},
					{"d:\\\\a\\\\b\\\\c", "d:/a/b/c"},
					{"/a//b//c/", "/a/b/c"},
				}
				
				for _, tt := range testCases {
					normalized := normalizePath(tt.input)
					if normalized != tt.expect {
						t.Errorf("normalizePath(%q) = %q, want %q", tt.input, normalized, tt.expect)
					}
				}
			},
		},
		{
			name: "test filepath.Base with normalized paths",
			testFunc: func(t *testing.T) {
				// 测试 filepath.Base 在标准化后的路径上的行为
				// 在 Windows 上，filepath.Base 应该能正确处理标准化后的 Unix 风格路径
				testCases := []struct {
					path     string
					expected string
				}{
					{"/a/b/c.txt", "c.txt"},
					{"/a/b/", "b"},
					{"/a.txt", "a.txt"},
					{"/", "/"},
					{"d:/a/b/c.txt", "c.txt"},
					{"d:/a/b/", "b"},
					{"a.txt", "a.txt"},
				}
				
				for _, tt := range testCases {
					normalized := normalizePath(tt.path)
					baseName := filepath.Base(normalized)
					// filepath.Base 在 Windows 上可能返回 Windows 风格的路径，需要再次标准化
					baseName = normalizePath(baseName)
					
					// 对于标准化后的路径，filepath.Base 应该返回正确的文件名
					// 但如果路径是 "/"，filepath.Base 在 Windows 上可能返回 "\"
					if normalized == "/" {
						if baseName != "/" && baseName != "\\" {
							t.Errorf("filepath.Base(%q) = %q, expected \"/\" or \"\\\"", normalized, baseName)
						}
					} else {
						// 手动提取文件名作为期望值
						expected := tt.expected
						if normalized != "/" {
							parts := strings.Split(strings.Trim(normalized, "/"), "/")
							if len(parts) > 0 {
								expected = parts[len(parts)-1]
							}
						}
						if baseName != expected && baseName != normalizePath(expected) {
							t.Errorf("filepath.Base(normalizePath(%q)) = %q, want %q (normalized: %q)", 
								tt.path, baseName, expected, normalized)
						}
					}
				}
			},
		},
		{
			name: "test UploadFile local file path handling",
			testFunc: func(t *testing.T) {
				// 测试 UploadFile 函数中本地文件路径的处理
				// 本地文件路径在 Windows 上可能是 d:\a.mkv，这个路径直接传给 os.Open
				// os.Open 应该能正确处理 Windows 路径，但我们需要确保不会影响远程路径的处理
				testCases := []struct {
					localPath string
					destPath  string
					destExp   string
				}{
					{"d:\\a.mkv", "/a.mkv", "/a.mkv"},
					{"d:/a.mkv", "/a.mkv", "/a.mkv"},
					{"d:\\a.mkv", "d:\\dest\\a.mkv", "d:/dest/a.mkv"},
					{"C:\\Users\\test\\file.txt", "/file.txt", "/file.txt"},
				}
				
				for _, tt := range testCases {
					destNormalized := normalizePath(tt.destPath)
					if destNormalized != tt.destExp {
						t.Errorf("normalizePath(dest=%q) = %q, want %q", tt.destPath, destNormalized, tt.destExp)
					}
					// 本地路径不需要标准化，因为它是本地文件系统路径
					// 但我们可以验证它不会影响远程路径的标准化
				}
			},
		},
		{
			name: "test List function path normalization",
			testFunc: func(t *testing.T) {
				testCases := []struct {
					dirPath string
					expect  string
				}{
					{"d:\\folder", "d:/folder"},
					{"d:/folder", "d:/folder"},
					{"/folder", "/folder"},
					{"\\folder", "/folder"},
					{"", ""},
					{"/", "/"},
				}
				
				for _, tt := range testCases {
					normalized := normalizePath(tt.dirPath)
					if normalized != tt.expect {
						t.Errorf("normalizePath(dirPath=%q) = %q, want %q", tt.dirPath, normalized, tt.expect)
					}
				}
			},
		},
		{
			name: "test CreateFolder parent directory path normalization",
			testFunc: func(t *testing.T) {
				testCases := []struct {
					pdirArg string
					expect  string
				}{
					{"d:\\parent", "d:/parent"},
					{"d:/parent", "d:/parent"},
					{"/parent", "/parent"},
					{"\\parent", "/parent"},
					{"/", "/"},
					{"", ""},
				}
				
				for _, tt := range testCases {
					normalized := normalizePath(tt.pdirArg)
					if normalized != tt.expect {
						t.Errorf("normalizePath(pdirArg=%q) = %q, want %q", tt.pdirArg, normalized, tt.expect)
					}
				}
			},
		},
		{
			name: "test edge cases with Windows drive letters",
			testFunc: func(t *testing.T) {
				testCases := []struct {
					path   string
					expect string
				}{
					{"C:\\", "C:"}, // normalizePath 会移除尾部斜杠
					{"C:/", "C:"}, // normalizePath 会移除尾部斜杠
					{"C:\\a", "C:/a"},
					{"C:/a", "C:/a"},
					{"D:\\a\\b", "D:/a/b"},
					{"Z:\\deep\\path\\to\\file.txt", "Z:/deep/path/to/file.txt"},
				}
				
				for _, tt := range testCases {
					normalized := normalizePath(tt.path)
					if normalized != tt.expect {
						t.Errorf("normalizePath(%q) = %q, want %q", tt.path, normalized, tt.expect)
					}
				}
			},
		},
		{
			name: "test UNC paths normalization",
			testFunc: func(t *testing.T) {
				// UNC 路径如 \\server\share\file.txt
				// normalizePath 会将多个连续斜杠合并为单个，所以 \\ 会变成 /
				// 虽然夸克网盘 API 不使用 UNC 路径，但我们应该确保 normalizePath 能正确处理
				testCases := []struct {
					path   string
					expect string
				}{
					{"\\\\server\\share\\file.txt", "/server/share/file.txt"},
					{"\\\\server\\share", "/server/share"},
				}
				
				for _, tt := range testCases {
					normalized := normalizePath(tt.path)
					if normalized != tt.expect {
						t.Errorf("normalizePath(%q) = %q, want %q", tt.path, normalized, tt.expect)
					}
				}
			},
		},
		{
			name: "test Unix path compatibility",
			testFunc: func(t *testing.T) {
				// 确保原有的 Unix 路径场景仍然正常工作
				testCases := []struct {
					path   string
					expect string
					desc   string
				}{
					{"/", "/", "根目录保持不变"},
					{"/a", "/a", "单层路径保持不变"},
					{"/a/b", "/a/b", "多层路径保持不变"},
					{"/a/b/c", "/a/b/c", "深层路径保持不变"},
					{"/a/b/", "/a/b", "尾部斜杠被移除"},
					{"/a/b/c.txt", "/a/b/c.txt", "文件路径保持不变"},
					{"/home/user/file.txt", "/home/user/file.txt", "完整 Unix 路径保持不变"},
					{"/var/log/app.log", "/var/log/app.log", "系统路径保持不变"},
					{"//a//b//c", "/a/b/c", "多个连续斜杠被合并"},
					{"/a//b//c", "/a/b/c", "路径中多个斜杠被合并"},
					{"", "", "空字符串保持不变"},
					{".", ".", "当前目录表示保持不变"},
					{"/.", "/.", "根目录下的点保持不变"},
				}
				
				for _, tt := range testCases {
					normalized := normalizePath(tt.path)
					if normalized != tt.expect {
						t.Errorf("normalizePath(%q) = %q, want %q (%s)", tt.path, normalized, tt.expect, tt.desc)
					}
				}
			},
		},
		{
			name: "test Unix path in all functions",
			testFunc: func(t *testing.T) {
				// 测试所有函数对 Unix 路径的处理
				unixPaths := []string{
					"/",
					"/a",
					"/a/b",
					"/a/b/c.txt",
					"/home/user/file.txt",
					"/var/log/app.log",
				}
				
				for _, path := range unixPaths {
					normalized := normalizePath(path)
					// Unix 路径应该保持不变（除了合并斜杠和移除尾部斜杠）
					expected := path
					if strings.Contains(path, "//") {
						// 如果有多个斜杠，会被合并
						expected = strings.ReplaceAll(expected, "//", "/")
					}
					if len(expected) > 1 && strings.HasSuffix(expected, "/") {
						expected = strings.TrimSuffix(expected, "/")
					}
					
					if normalized != expected {
						t.Errorf("normalizePath(%q) = %q, want %q (Unix path should remain unchanged)", 
							path, normalized, expected)
					}
					
					// 测试 normalizeRootDir
					if path == "/" || path == "" || path == "." {
						rootDir := normalizeRootDir(path)
						if rootDir != "0" {
							t.Errorf("normalizeRootDir(%q) = %q, want \"0\"", path, rootDir)
						}
					} else {
						rootDir := normalizeRootDir(path)
						if rootDir != normalized {
							t.Errorf("normalizeRootDir(%q) = %q, want %q", path, rootDir, normalized)
						}
					}
				}
			},
		},
		{
			name: "test backward compatibility with original Unix behavior",
			testFunc: func(t *testing.T) {
				// 确保原有的 Unix 路径行为没有被破坏
				// 这些是典型的 Unix/Linux 路径格式
				testCases := []struct {
					original string
					normalized string
					shouldChange bool
				}{
					{"/", "/", false}, // 根目录不变
					{"/a", "/a", false}, // 单层路径不变
					{"/a/b", "/a/b", false}, // 多层路径不变
					{"/a/b/", "/a/b", true}, // 尾部斜杠被移除（这是预期的）
					{"/a//b", "/a/b", true}, // 多个斜杠被合并（这是预期的）
					{"/home/user", "/home/user", false}, // 标准 Unix 路径不变
					{"/usr/local/bin", "/usr/local/bin", false}, // 系统路径不变
					{"/tmp/file.txt", "/tmp/file.txt", false}, // 文件路径不变
				}
				
				for _, tt := range testCases {
					normalized := normalizePath(tt.original)
					if normalized != tt.normalized {
						t.Errorf("normalizePath(%q) = %q, want %q", tt.original, normalized, tt.normalized)
					}
					
					// 验证行为是否符合预期
					changed := normalized != tt.original
					if changed != tt.shouldChange {
						t.Errorf("normalizePath(%q) changed=%v, want changed=%v", 
							tt.original, changed, tt.shouldChange)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.testFunc)
	}
}
