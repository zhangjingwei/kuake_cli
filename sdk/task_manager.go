package sdk

// TaskManager 任务管理器 - 负责管理异步任务队列
type TaskManager struct {
	queue *TaskQueue // 任务队列
}

// NewTaskManager 创建新的任务管理器实例
// maxWorkers: 最大并发数，如果为 0 则使用默认值 3
func NewTaskManager(maxWorkers ...int) *TaskManager {
	// 如果未指定 maxWorkers 或为 0，使用默认值 3
	workers := 3
	if len(maxWorkers) > 0 && maxWorkers[0] > 0 {
		workers = maxWorkers[0]
	}

	tm := &TaskManager{
		queue: NewTaskQueue(workers),
	}

	// 启动队列处理器（需要外部提供 TaskExecutor 实现）
	// tm.queue.Start(executor) // 由外部调用

	return tm
}

// Start 启动任务队列处理器
// executor: 任务执行器，实现 TaskExecutor 接口
func (tm *TaskManager) Start(executor TaskExecutor) {
	tm.queue.Start(executor)
}

// AddTask 添加任务到队列
func (tm *TaskManager) AddTask(taskType TaskType, params map[string]interface{}) *Task {
	return tm.queue.AddTask(taskType, params)
}

// GetTask 获取任务
func (tm *TaskManager) GetTask(taskID string) (*Task, bool) {
	return tm.queue.GetTask(taskID)
}

// GetAllTasks 获取所有任务
func (tm *TaskManager) GetAllTasks() []*Task {
	return tm.queue.GetAllTasks()
}

// GetPendingTasks 获取等待中的任务
func (tm *TaskManager) GetPendingTasks() []*Task {
	return tm.queue.GetPendingTasks()
}

// GetRunningTasks 获取运行中的任务
func (tm *TaskManager) GetRunningTasks() []*Task {
	return tm.queue.GetRunningTasks()
}

// GetCompletedTasks 获取已完成的任务
func (tm *TaskManager) GetCompletedTasks() []*Task {
	return tm.queue.GetCompletedTasks()
}

// CancelTask 取消任务
func (tm *TaskManager) CancelTask(taskID string) error {
	return tm.queue.CancelTask(taskID)
}

// SetTaskCallback 设置任务回调
func (tm *TaskManager) SetTaskCallback(taskID string, callback TaskCallback) {
	tm.queue.SetTaskCallback(taskID, callback)
}

// WaitAllTasks 等待所有任务完成
func (tm *TaskManager) WaitAllTasks() {
	tm.queue.Wait()
}

// StopQueue 停止队列处理器
func (tm *TaskManager) StopQueue() {
	tm.queue.Stop()
}
