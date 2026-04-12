package task

// Copyright (c) 2026 LingByte
// SPDX-License-Identifier: MIT

import (
	"context"
	"time"

	"go.uber.org/zap"
)

type Scheduler[P, R any] struct {
	pool  *TaskPool[P, R]
	queue *PriorityQueue
}

func NewScheduler[P, R any](workerCount int, logger *zap.Logger) *Scheduler[P, R] {
	pool := NewTaskPool[P, R](&TaskPoolOption{
		WorkerCount: workerCount,
		logger:      logger,
	})

	s := &Scheduler[P, R]{
		pool:  pool,
		queue: &PriorityQueue{},
	}

	go s.dispatchLoop()
	return s
}

func (s *Scheduler[P, R]) dispatchLoop() {
	for {
		// If all workers are busy, keep tasks in the scheduler queue so that
		// GetTaskPosition reflects waiting tasks.
		if s.pool.Running() >= int32(s.pool.WorkerCount()) {
			time.Sleep(10 * time.Millisecond)
			continue
		}

		item := s.queue.Pop()
		if item == nil {
			time.Sleep(10 * time.Millisecond)
			continue
		}

		task, ok := item.(*Task[P, R])
		if !ok || task == nil {
			continue
		}

		_ = s.pool.addTaskDirectly(task)
	}
}

func (s *Scheduler[P, R]) SubmitTask(
	ctx context.Context,
	priority int,
	param P,
	handler func(ctx context.Context, params P) (R, error),
) *Task[P, R] {
	ctxCancel, cancelFunc := context.WithCancel(ctx)
	task := &Task[P, R]{
		ID:         generateTaskID(),
		ctx:        ctxCancel,
		cancel:     cancelFunc,
		Priority:   priority,
		Params:     param,
		Handler:    handler,
		Result:     make(chan R, 1),
		Err:        make(chan error, 1),
		SubmitTime: time.Now(),
	}
	task.Status.Store(TaskStatusPending)
	task.Progress.Store(0)
	s.queue.Push(task, priority, task.ID)
	return task
}

func (s *Scheduler[P, R]) CancelTaskByID(taskID string) bool {
	return s.queue.Remove(taskID)
}

func (s *Scheduler[P, R]) GetTaskPosition(taskID string) int {
	return s.queue.GetPosition(taskID)
}

func (s *Scheduler[P, R]) QueueLen() int {
	return s.queue.Len()
}

func (s *Scheduler[P, R]) RunningCount() int32 {
	return s.pool.Running()
}

func (tp *TaskPool[P, R]) addTaskDirectly(task *Task[P, R]) error {
	tp.wg.Add(1)
	tp.taskChan <- task
	return nil
}
