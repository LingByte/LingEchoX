package task

// Copyright (c) 2026 LingByte
// SPDX-License-Identifier: MIT

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/LingByte/lingoroutine/utils"
	"go.uber.org/zap"
)

type TaskStatus string

const (
	TaskStatusPending TaskStatus = "pending"
	TaskStatusRunning TaskStatus = "running"
	TaskStatusSuccess TaskStatus = "success"
	TaskStatusFailed  TaskStatus = "failed"
	TaskStatusCancel  TaskStatus = "canceled"
)

func (ts TaskStatus) ToString() string {
	return string(ts)
}

type TaskPoolOption struct {
	WorkerCount int
	QueueSize   int
	logger      *zap.Logger
}

// generateTaskID 生成唯一ID：前缀 + 时间戳 + 自增ID
func generateTaskID() string {
	return fmt.Sprintf("task_ling_%s", utils.SnowflakeUtil.GenID())
}

type Task[Params, Result any] struct {
	ID         string
	ctx        context.Context
	cancel     context.CancelFunc
	Priority   int
	Params     Params
	Handler    func(ctx context.Context, params Params) (Result, error)
	Result     chan Result
	Err        chan error
	Status     atomic.Value
	Progress   atomic.Int32
	SubmitTime time.Time
}

type TaskPool[Param any, Result any] struct {
	taskChan chan *Task[Param, Result]
	wg       sync.WaitGroup
	logger   *zap.Logger
	running  atomic.Int32
	workers  int
}

func NewTaskPool[Param any, Result any](options *TaskPoolOption) *TaskPool[Param, Result] {
	if options == nil {
		options = &TaskPoolOption{}
	}
	if options.WorkerCount <= 0 {
		options.WorkerCount = 4
	}
	if options.QueueSize <= 0 {
		options.QueueSize = 1024
	}

	logger := options.logger
	if logger == nil {
		logger, _ = zap.NewProduction()
	}
	pool := &TaskPool[Param, Result]{
		taskChan: make(chan *Task[Param, Result], options.QueueSize),
		logger:   logger,
		workers:  options.WorkerCount,
	}
	for i := 0; i < options.WorkerCount; i++ {
		go pool.worker()
	}
	return pool
}

func (tp *TaskPool[P, R]) WorkerCount() int {
	return tp.workers
}

func (tp *TaskPool[P, R]) worker() {
	for task := range tp.taskChan {
		if task.ctx.Err() != nil {
			tp.logger.Warn("task canceled before run", zap.String("id", task.ID))
			task.Status.Store(TaskStatusCancel)
			task.Err <- task.ctx.Err()
			close(task.Result)
			close(task.Err)
			tp.wg.Done()
			continue
		}
		tp.running.Add(1)
		tp.logger.Info(fmt.Sprintf("start task [%s]", task.ID))
		res, err := task.Handler(task.ctx, task.Params)
		tp.logger.Info(fmt.Sprintf("end task [%s]", task.ID))
		tp.running.Add(-1)
		tp.wg.Done()
		if err != nil {
			if errors.Is(err, context.Canceled) {
				task.Status.Store(TaskStatusCancel)
			} else {
				task.Status.Store(TaskStatusFailed)
			}
		} else {
			task.Status.Store(TaskStatusSuccess)
		}
		task.Result <- res
		task.Err <- err
		close(task.Result)
		close(task.Err)
	}
}

func (tp *TaskPool[P, R]) AddTask(ctx context.Context, param P, handler func(ctx context.Context, p P) (R, error)) (*Task[P, R], error) {
	ctxCancel, cancelFunc := context.WithCancel(ctx)
	task := &Task[P, R]{
		ID:         generateTaskID(),
		ctx:        ctxCancel,
		cancel:     cancelFunc, // 赋值！
		Params:     param,
		Handler:    handler,
		Result:     make(chan R, 1),
		Err:        make(chan error, 1),
		SubmitTime: time.Now(),
	}
	task.Status.Store(TaskStatusPending)
	task.Progress.Store(0)

	tp.wg.Add(1)
	tp.taskChan <- task
	return task, nil
}

func (tp *TaskPool[P, R]) CancelTask(task *Task[P, R]) {
	if task.cancel != nil {
		task.cancel()
		task.Status.Store(TaskStatusCancel)
		tp.logger.Warn("task canceled by user", zap.String("id", task.ID))
	}
}

func (tp *TaskPool[P, R]) QueueLen() int {
	return len(tp.taskChan)
}

func (tp *TaskPool[P, R]) Running() int32 {
	return tp.running.Load()
}

func (tp *TaskPool[P, R]) Wait() {
	close(tp.taskChan)
	tp.wg.Wait()
}
