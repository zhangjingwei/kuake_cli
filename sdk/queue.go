package sdk

import (
	"fmt"
	"time"
)

// NewTaskQueue 创建新的任务队列
func NewTaskQueue(maxWorkers int) *TaskQueue {
	return &TaskQueue{
		maxWorkers: maxWorkers,
		tasks:      make(map[string]*Task),
		pending:    make([]*Task, 0),
		running:    make([]*Task, 0),
		completed:  make([]*Task, 0),
		callbacks:  make(map[string]TaskCallback),
		stopCh:     make(chan struct{}),
	}
}

// Start 启动任务队列处理器
func (q *TaskQueue) Start(executor TaskExecutor) {
	q.mu.Lock()
	q.executor = executor
	q.mu.Unlock()

	// 启动工作协程
	for i := 0; i < q.maxWorkers; i++ {
		q.wg.Add(1)
		go q.worker()
	}
}

// worker 工作协程，处理任务队列
func (q *TaskQueue) worker() {
	defer q.wg.Done()

	for {
		select {
		case <-q.stopCh:
			return
		default:
			task := q.getNextPendingTask()
			if task == nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}

			// 执行任务
			q.executeTask(task)
		}
	}
}

// getNextPendingTask 获取下一个待处理任务
func (q *TaskQueue) getNextPendingTask() *Task {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.pending) == 0 {
		return nil
	}

	task := q.pending[0]
	q.pending = q.pending[1:]
	q.running = append(q.running, task)

	task.Status = TaskStatusRunning
	now := time.Now()
	task.StartedAt = &now

	return task
}

// executeTask 执行任务
func (q *TaskQueue) executeTask(task *Task) {
	// 获取回调
	q.mu.RLock()
	callback, hasCallback := q.callbacks[task.ID]
	executor := q.executor
	q.mu.RUnlock()

	if executor == nil {
		task.Status = TaskStatusFailed
		task.Error = fmt.Errorf("no executor set")
		q.completeTask(task)
		return
	}

	// 执行任务
	result, err := executor.Execute(task)

	// 更新任务状态
	q.mu.Lock()
	if err != nil {
		task.Status = TaskStatusFailed
		task.Error = err
	} else {
		task.Status = TaskStatusCompleted
		task.Result = result
	}
	now := time.Now()
	task.CompletedAt = &now
	task.Progress = 100.0

	// 从运行中移除
	for i, t := range q.running {
		if t.ID == task.ID {
			q.running = append(q.running[:i], q.running[i+1:]...)
			break
		}
	}

	// 添加到已完成
	q.completed = append(q.completed, task)
	q.mu.Unlock()

	// 调用回调
	if hasCallback {
		if err != nil {
			if callback.OnError != nil {
				callback.OnError(task, err)
			}
		} else {
			if callback.OnComplete != nil {
				callback.OnComplete(task, result)
			}
		}
	}
}

// completeTask 完成任务
func (q *TaskQueue) completeTask(task *Task) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// 从运行中移除
	for i, t := range q.running {
		if t.ID == task.ID {
			q.running = append(q.running[:i], q.running[i+1:]...)
			break
		}
	}

	// 添加到已完成
	q.completed = append(q.completed, task)
}

// AddTask 添加任务到队列
func (q *TaskQueue) AddTask(taskType TaskType, params map[string]interface{}) *Task {
	task := &Task{
		ID:        generateTaskID(),
		Type:      taskType,
		Status:    TaskStatusPending,
		Params:    params,
		CreatedAt: time.Now(),
		Progress:  0.0,
	}

	q.mu.Lock()
	q.tasks[task.ID] = task
	q.pending = append(q.pending, task)
	q.mu.Unlock()

	return task
}

// GetTask 获取任务
func (q *TaskQueue) GetTask(taskID string) (*Task, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	task, ok := q.tasks[taskID]
	return task, ok
}

// GetAllTasks 获取所有任务
func (q *TaskQueue) GetAllTasks() []*Task {
	q.mu.RLock()
	defer q.mu.RUnlock()

	tasks := make([]*Task, 0, len(q.tasks))
	for _, task := range q.tasks {
		tasks = append(tasks, task)
	}
	return tasks
}

// GetPendingTasks 获取等待中的任务
func (q *TaskQueue) GetPendingTasks() []*Task {
	q.mu.RLock()
	defer q.mu.RUnlock()

	tasks := make([]*Task, len(q.pending))
	copy(tasks, q.pending)
	return tasks
}

// GetRunningTasks 获取运行中的任务
func (q *TaskQueue) GetRunningTasks() []*Task {
	q.mu.RLock()
	defer q.mu.RUnlock()

	tasks := make([]*Task, len(q.running))
	copy(tasks, q.running)
	return tasks
}

// GetCompletedTasks 获取已完成的任务
func (q *TaskQueue) GetCompletedTasks() []*Task {
	q.mu.RLock()
	defer q.mu.RUnlock()

	tasks := make([]*Task, len(q.completed))
	copy(tasks, q.completed)
	return tasks
}

// CancelTask 取消任务
func (q *TaskQueue) CancelTask(taskID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	task, ok := q.tasks[taskID]
	if !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}

	if task.Status != TaskStatusPending {
		return fmt.Errorf("task cannot be cancelled: status is %s", task.Status)
	}

	// 从待处理列表中移除
	for i, t := range q.pending {
		if t.ID == taskID {
			q.pending = append(q.pending[:i], q.pending[i+1:]...)
			break
		}
	}

	task.Status = TaskStatusCancelled
	return nil
}

// SetTaskCallback 设置任务回调
func (q *TaskQueue) SetTaskCallback(taskID string, callback TaskCallback) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.callbacks[taskID] = callback
}

// Wait 等待所有任务完成
func (q *TaskQueue) Wait() {
	for {
		q.mu.RLock()
		pendingCount := len(q.pending)
		runningCount := len(q.running)
		q.mu.RUnlock()

		if pendingCount == 0 && runningCount == 0 {
			break
		}

		time.Sleep(100 * time.Millisecond)
	}
}

// Stop 停止队列处理器
func (q *TaskQueue) Stop() {
	close(q.stopCh)
	q.wg.Wait()
}

// generateTaskID 生成任务ID
func generateTaskID() string {
	return fmt.Sprintf("task_%d", time.Now().UnixNano())
}
