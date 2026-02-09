package sdk

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// FileInfo 文件信息结构（高级接口使用）
type FileInfo struct {
	Name        string    `json:"name"`
	Path        string    `json:"path"`
	Size        int64     `json:"size"`
	ModTime     time.Time `json:"mod_time"`
	IsDirectory bool      `json:"is_directory"`
}

// QuarkClient 夸克网盘 API 客户端
type QuarkClient struct {
	baseURL           string
	accessToken       string            // 当前使用的 access token
	accessTokens      []string          // 所有可用的 access tokens
	currentTokenIdx   int               // 当前使用的 token 索引
	cookies           map[string]string // 解析后的 cookie 字典
	HttpClient        *http.Client
	lastAuthCheck     time.Time     // 上次认证检查时间
	authCheckValid    bool          // 认证检查是否有效
	authCheckMutex    sync.RWMutex  // 认证检查的读写锁
	authCheckTimeout  time.Duration // 认证检查缓存时间（默认5分钟）
	failedTokens      map[int]bool  // 记录已失败的 token 索引
	failedTokensMutex sync.RWMutex  // 失败 token 记录的锁
	Debug             bool          // 调试开关，控制是否输出调试信息
}

// QuarkFileInfo 夸克网盘文件信息
type QuarkFileInfo struct {
	Fid         string `json:"fid"`                    // 文件ID
	Name        string `json:"file_name"`              // 文件名
	Path        string `json:"path"`                   // 文件路径
	Size        int64  `json:"size"`                   // 文件大小
	CreateTime  int64  `json:"ctime"`                  // 创建时间戳（秒），从 created_at 或 l_created_at 转换
	ModifyTime  int64  `json:"mtime"`                  // 修改时间戳（秒），从 updated_at 或 l_updated_at 转换
	IsDirectory bool   `json:"dir"`                    // 是否为目录，从 dir 或 file 字段获取
	DownloadURL string `json:"download_url"`           // 下载链接（列表API中通常不存在）
	CreatedAt   int64  `json:"created_at,omitempty"`   // 创建时间戳（毫秒），API原始字段
	UpdatedAt   int64  `json:"updated_at,omitempty"`   // 修改时间戳（毫秒），API原始字段
	LCreatedAt  int64  `json:"l_created_at,omitempty"` // 创建时间戳（毫秒），API原始字段
	LUpdatedAt  int64  `json:"l_updated_at,omitempty"` // 修改时间戳（毫秒），API原始字段
}

// QuarkListResponse 列表响应
type QuarkListResponse struct {
	Data struct {
		List []QuarkFileInfo `json:"list"`
	} `json:"data"`
	Errno  int    `json:"errno"`
	Errmsg string `json:"errmsg"`
}

// QuarkUploadResponse 上传响应
type QuarkUploadResponse struct {
	Data struct {
		Fid string `json:"fid"`
	} `json:"data"`
	Errno  int    `json:"errno"`
	Errmsg string `json:"errmsg"`
}

// QuarkFileInfoResponse 文件信息响应
type QuarkFileInfoResponse struct {
	Data   QuarkFileInfo `json:"data"`
	Errno  int           `json:"errno"`
	Errmsg string        `json:"errmsg"`
}

// Config 配置结构
type Config struct {
	Quark struct {
		AccessTokens []string `json:"access_tokens"` // Access Token 数组
	}
}

// UserInfo 用户信息结构
type UserInfo struct {
	Cookie string                 `json:"cookie"` // Cookie 字符串
	Data   map[string]interface{} `json:"data"`   // 用户数据
}

// StandardResponse 通用响应结构
type StandardResponse struct {
	Success bool                   `json:"success"` // 是否成功
	Code    string                 `json:"code"`    // 响应代码（"OK" 表示成功）
	Message string                 `json:"message"` // 响应消息
	Data    map[string]interface{} `json:"data"`    // 响应数据
}

// UploadProgress 上传进度信息
type UploadProgress struct {
	Progress     int           `json:"progress"`      // 进度百分比 (0-100)
	Uploaded     int64         `json:"uploaded"`      // 已上传字节数
	Total        int64         `json:"total"`         // 总字节数
	Speed        float64       `json:"speed"`         // 上传速度 (字节/秒)
	SpeedStr     string        `json:"speed_str"`     // 格式化的速度字符串 (如 "25.5 MB/s")
	Remaining    time.Duration `json:"remaining"`     // 剩余时间
	RemainingStr string        `json:"remaining_str"` // 格式化的剩余时间字符串 (如 "2m30s")
	Elapsed      time.Duration `json:"elapsed"`       // 已用时间
}

// UploadState 上传状态（用于断点续传）
type UploadState struct {
	FilePath      string          `json:"file_path"`          // 本地文件路径
	DestPath      string          `json:"dest_path"`          // 目标路径
	FileSize      int64           `json:"file_size"`          // 文件大小
	UploadID      string          `json:"upload_id"`          // OSS UploadID
	TaskID        string          `json:"task_id"`            // 任务ID
	Bucket        string          `json:"bucket"`             // OSS Bucket
	ObjKey        string          `json:"obj_key"`            // OSS Object Key
	UploadURL     string          `json:"upload_url"`         // 上传URL
	PartSize      int64           `json:"part_size"`          // 分片大小
	UploadedParts map[int]string  `json:"uploaded_parts"`     // 已上传的分片：partNumber -> ETag
	MimeType      string          `json:"mime_type"`          // MIME类型
	AuthInfo      json.RawMessage `json:"auth_info"`          // 认证信息
	Callback      json.RawMessage `json:"callback"`           // 回调信息
	HashCtx       *HashCtx        `json:"hash_ctx,omitempty"` // SHA1增量哈希上下文
	CreatedAt     time.Time       `json:"created_at"`         // 创建时间
}

// PreUploadResponse 预上传响应
type PreUploadResponse struct {
	Code   int `json:"code"`
	Status int `json:"status"`
	Data   struct {
		TaskID    string          `json:"task_id"`
		Bucket    string          `json:"bucket"`
		ObjKey    string          `json:"obj_key"`
		UploadID  string          `json:"upload_id"`
		UploadURL string          `json:"upload_url"`
		AuthInfo  json.RawMessage `json:"auth_info"` // 可能是字符串或对象
		Callback  json.RawMessage `json:"callback"`  // 可能是字符串或对象
	} `json:"data"`
	Metadata struct {
		PartSize int64 `json:"part_size"`
	}
}

// HashResponse 哈希验证响应
type HashResponse struct {
	Code   int `json:"code"`
	Status int `json:"status"`
	Data   struct {
		Finish bool `json:"finish"`
	} `json:"data"`
}

// HashCtx SHA1增量哈希上下文
type HashCtx struct {
	HashType string `json:"hash_type"` // "sha1"
	H0       string `json:"h0"`        // SHA1的5个32位整数
	H1       string `json:"h1"`
	H2       string `json:"h2"`
	H3       string `json:"h3"`
	H4       string `json:"h4"`
	Nl       string `json:"Nl"`   // 已处理的字节数
	Nh       string `json:"Nh"`   // 哈希相关计数
	Data     string `json:"data"` // 未处理的数据
	Num      string `json:"num"`  // 分片编号或其他计数
}

// AuthResponse 认证响应
type AuthResponse struct {
	Code   int `json:"code"`
	Status int `json:"status"`
	Data   struct {
		AuthKey string `json:"auth_key"`
	} `json:"data"`
}

// FinishResponse 完成上传响应
type FinishResponse struct {
	Code   int                    `json:"code"`
	Status int                    `json:"status"`
	Data   map[string]interface{} `json:"data"`
}

// CreateFolderResponse 创建文件夹响应
type CreateFolderResponse struct {
	Code   int                    `json:"code"`
	Status int                    `json:"status"`
	Data   map[string]interface{} `json:"data"`
}

// MoveResponse 移动响应
type MoveResponse struct {
	Code   int `json:"code"`
	Status int `json:"status"`
	Data   struct {
		Fid string `json:"fid"`
	} `json:"data"`
}

// CopyResponse 复制响应
type CopyResponse struct {
	Code   int `json:"code"`
	Status int `json:"status"`
	Data   struct {
		Fid string `json:"fid"`
	} `json:"data"`
}

// RenameResponse 重命名响应
type RenameResponse struct {
	Code   int `json:"code"`
	Status int `json:"status"`
	Data   struct {
		Fid string `json:"fid"`
	} `json:"data"`
}

// ShareInfo 分享信息
type ShareInfo struct {
	PwdID    string // 分享链接ID
	Passcode string // 提取码
}

// ShareStokenResponse 分享stoken响应
type ShareStokenResponse struct {
	Code   int                    `json:"code"`
	Status int                    `json:"status"`
	Data   map[string]interface{} `json:"data"`
}

// ShareListResponse 分享列表响应
type ShareListResponse struct {
	Code   int                    `json:"code"`
	Status int                    `json:"status"`
	Data   map[string]interface{} `json:"data"`
}

// SaveShareFileResponse 转存分享文件响应
type SaveShareFileResponse struct {
	Code   int                    `json:"code"`
	Status int                    `json:"status"`
	Data   map[string]interface{} `json:"data"`
}

// CreateShareResponse 创建分享响应
type CreateShareResponse struct {
	Code   int                    `json:"code"`
	Status int                    `json:"status"`
	Data   map[string]interface{} `json:"data"`
}

// DownloadResponse 下载响应（同步：直接返回 data 数组）
type DownloadResponse struct {
	Code   int `json:"code"`
	Status int `json:"status"`
	Data   []struct {
		Fid         string `json:"fid"`          // 文件ID
		DownloadURL string `json:"download_url"` // 下载链接
	} `json:"data"`
}

// DownloadResponseAsync 下载响应（异步：返回 task_id，需轮询任务获取 download_url）
type DownloadResponseAsync struct {
	Code   int `json:"code"`
	Status int `json:"status"`
	Data   struct {
		TaskID   string `json:"task_id"`
		TaskSync bool   `json:"task_sync"`
		TaskResp *struct {
			Data []struct {
				Fid         string `json:"fid"`
				DownloadURL string `json:"download_url"`
			} `json:"data"`
		} `json:"task_resp"`
	} `json:"data"`
}

// ShareLinkInfo 分享链接信息
type ShareLinkInfo struct {
	ShareURL  string // 分享链接
	Passcode  string // 提取码
	PwdID     string // 分享ID
	ExpiresAt int64  // 过期时间（时间戳）
}

// TaskType 任务类型
type TaskType string

const (
	TaskTypeUpload   TaskType = "upload"   // 上传
	TaskTypeDownload TaskType = "download" // 下载
	TaskTypeDelete   TaskType = "delete"   // 删除
	TaskTypeMove     TaskType = "move"     // 移动
	TaskTypeCopy     TaskType = "copy"     // 复制
	TaskTypeWrite    TaskType = "write"    // 写入
)

// TaskStatus 任务状态
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"   // 等待中
	TaskStatusRunning   TaskStatus = "running"   // 执行中
	TaskStatusCompleted TaskStatus = "completed" // 已完成
	TaskStatusFailed    TaskStatus = "failed"    // 失败
	TaskStatusCancelled TaskStatus = "cancelled" // 已取消
)

// Task 任务结构
type Task struct {
	ID          string                 `json:"id"`           // 任务ID
	Type        TaskType               `json:"type"`         // 任务类型
	Status      TaskStatus             `json:"status"`       // 任务状态
	Params      map[string]interface{} `json:"params"`       // 任务参数
	Result      interface{}            `json:"result"`       // 任务结果
	Error       error                  `json:"error"`        // 错误信息
	CreatedAt   time.Time              `json:"created_at"`   // 创建时间
	StartedAt   *time.Time             `json:"started_at"`   // 开始时间
	CompletedAt *time.Time             `json:"completed_at"` // 完成时间
	Progress    float64                `json:"progress"`     // 进度（0-100）
	mu          sync.RWMutex           `json:"-"`            // 读写锁
}

// TaskCallback 任务回调结构
type TaskCallback struct {
	OnProgress func(task *Task, progress float64)   // 进度回调
	OnComplete func(task *Task, result interface{}) // 完成回调
	OnError    func(task *Task, err error)          // 错误回调
}

// TaskQueue 任务队列
type TaskQueue struct {
	maxWorkers int
	tasks      map[string]*Task
	pending    []*Task
	running    []*Task
	completed  []*Task
	mu         sync.RWMutex
	executor   TaskExecutor
	callbacks  map[string]TaskCallback
	stopCh     chan struct{}
	wg         sync.WaitGroup
}

// TaskExecutor 任务执行器接口
type TaskExecutor interface {
	Execute(task *Task) (interface{}, error)
}

// RequestHeaderBuilder 请求头构建器接口
type RequestHeaderBuilder interface {
	BuildHeaders(req *http.Request, qc *QuarkClient) error
}

// OSSPartUploadHeaderBuilder OSS 分片上传头部构建器
type OSSPartUploadHeaderBuilder struct {
	AuthKey   string
	MimeType  string
	Timestamp string
	HashCtx   *HashCtx // SHA1增量哈希上下文（partNumber>=2时需要）
}

// OSSCommitHeaderBuilder OSS 提交上传头部构建器
type OSSCommitHeaderBuilder struct {
	AuthKey    string
	ContentMD5 string
	Callback   string
	Timestamp  string
}
