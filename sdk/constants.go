package sdk

// 域名常量
const (
	PAN_DOMAIN     = "https://pan.quark.cn"      // 主要用于用户信息获取
	DRIVE_DOMAIN   = "https://drive-pc.quark.cn" // 主要用于大部分API请求
	DRIVE_H_DOMAIN = "https://drive-h.quark.cn"  // save_share_file部分请求
)

// 配置相关常量
const (
	DEFAULT_CONFIG_PATH = "config.json" // 默认配置文件路径
)

// 用户信息
const (
	USER_INFO = "/account/info"
)

// 文件上传
const (
	FILE_UPLOAD_PRE    = "/1/clouddrive/file/upload/pre"
	FILE_UPDATE_HASH   = "/1/clouddrive/file/update/hash"
	FILE_UPLOAD_AUTH   = "/1/clouddrive/file/upload/auth"
	FILE_UPLOAD_FINISH = "/1/clouddrive/file/upload/finish"
)

// 文件下载
const (
	FILE_DOWNLOAD = "/1/clouddrive/file/download"
)

// 文件列表
const (
	FILE_SORT = "/1/clouddrive/file/sort"
)

// 文件操作
const (
	FILE_MOVE     = "/1/clouddrive/file/move"
	FILE_COPY     = "/1/clouddrive/file/copy"
	FILE_RENAME   = "/1/clouddrive/file/rename"
	FILE_DELETE   = "/1/clouddrive/file/delete"
	CREATE_FOLDER = "/1/clouddrive/file"
)

// 内容分享
const (
	SHARE          = "/1/clouddrive/share"
	SHARE_PASSWORD = "/1/clouddrive/share/password"
)

// 任务状态
const (
	TASK = "/1/clouddrive/task"
)

// 保存分享内容
const (
	SHARE_SHAREPAGE_TOKEN  = "/1/clouddrive/share/sharepage/token"
	SHARE_SHAREPAGE_DETAIL = "/1/clouddrive/share/sharepage/detail"
	SHARE_SHAREPAGE_SAVE   = "/1/clouddrive/share/sharepage/save"
)
