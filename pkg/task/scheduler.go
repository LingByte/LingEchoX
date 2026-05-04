package task

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// Stats exposes scheduler visibility for dashboards.
type Stats struct {
	Queued     int   // priority wait queue length
	ChannelLen int   // legacy: always 0 (no worker channel backlog)
	Running    int32 // handlers executing
	Unfinished int   // Queued + Running
}

// Scheduler multiplexes prioritized submissions onto a fixed-size worker pool.
// Waiting work lives only in the priority queue (no secondary task channel).
type Scheduler[Params, Result any] struct {
	mu          sync.Mutex
	cond        *sync.Cond
	queue       *PriorityQueue
	workerCount int
	running     int32
	stopping    atomic.Bool
	lg          *zap.Logger
	wg          sync.WaitGroup
}

// NewScheduler builds a scheduler with workerCount concurrent handlers.
func NewScheduler[Params, Result any](workerCount int, lg *zap.Logger) *Scheduler[Params, Result] {
	if workerCount <= 0 {
		workerCount = 4
	}
	if lg == nil {
		lg = zap.NewNop()
	}
	s := &Scheduler[Params, Result]{
		queue:       &PriorityQueue{},
		workerCount: workerCount,
		lg:          lg,
	}
	s.cond = sync.NewCond(&s.mu)
	s.wg.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go s.workerLoop()
	}
	return s
}

func (s *Scheduler[Params, Result]) relabelQueuedLocked() {
	n := s.queue.Len()
	rn := atomic.LoadInt32(&s.running)
	s.queue.RelabelQueued(func(pos int, _ string, ptr any) {
		t, ok := ptr.(*Task[Params, Result])
		if !ok || t == nil {
			return
		}
		t.applyQueueLabels(pos, n, rn)
	})
}

func (s *Scheduler[Params, Result]) workerLoop() {
	defer s.wg.Done()
	for {
		task := s.acquireTask()
		if task == nil {
			return
		}
		if task.ctx.Err() != nil {
			var zero Result
			task.deliver(zero, task.ctx.Err())
			atomic.AddInt32(&s.running, -1)
			s.afterTaskLocked()
			continue
		}
		task.Status.Store(TaskStatusRunning)
		wait := time.Since(task.SubmitTime)
		s.lg.Info("task running",
			zap.String("task_id", task.ID),
			zap.Int("priority", task.Priority),
			zap.Duration("queue_wait", wait),
			zap.Int("ahead_at_submit", task.QueueAhead()),
			zap.Int("queued_depth_at_submit", task.QueuedTotal()),
			zap.Int("unfinished_est_at_submit", task.UnfinishedEstimate()),
		)
		res, err := task.Handler(task.ctx, task.Params)
		task.deliver(res, err)
		atomic.AddInt32(&s.running, -1)
		if err != nil {
			s.lg.Warn("task finished with error",
				zap.String("task_id", task.ID),
				zap.Error(err),
			)
		} else {
			s.lg.Info("task finished ok",
				zap.String("task_id", task.ID),
			)
		}
		s.afterTaskLocked()
	}
}

func (s *Scheduler[Params, Result]) acquireTask() *Task[Params, Result] {
	s.mu.Lock()
	for s.queue.Len() == 0 {
		if s.stopping.Load() {
			s.mu.Unlock()
			return nil
		}
		s.cond.Wait()
	}
	if s.stopping.Load() && s.queue.Len() == 0 {
		s.mu.Unlock()
		return nil
	}
	item := s.queue.Pop()
	t, ok := item.(*Task[Params, Result])
	if !ok || t == nil {
		s.mu.Unlock()
		return s.acquireTask()
	}
	atomic.AddInt32(&s.running, 1)
	s.relabelQueuedLocked()
	s.mu.Unlock()
	return t
}

func (s *Scheduler[Params, Result]) afterTaskLocked() {
	s.mu.Lock()
	s.relabelQueuedLocked()
	s.cond.Signal()
	s.mu.Unlock()
}

// SubmitTask enqueues by priority (higher runs earlier among waiting tasks).
func (s *Scheduler[Params, Result]) SubmitTask(
	ctx context.Context,
	priority int,
	param Params,
	handler func(ctx context.Context, params Params) (Result, error),
) *Task[Params, Result] {
	task := newTask(ctx, priority, param, handler)
	if s.stopping.Load() {
		var z Result
		task.deliver(z, fmt.Errorf("scheduler stopped"))
		return task
	}
	s.mu.Lock()
	if s.stopping.Load() {
		s.mu.Unlock()
		var z Result
		task.deliver(z, fmt.Errorf("scheduler stopped"))
		return task
	}
	s.queue.Push(task, priority, task.ID)
	s.relabelQueuedLocked()
	qlen := s.queue.Len()
	rn := atomic.LoadInt32(&s.running)
	s.mu.Unlock()
	s.cond.Signal()
	s.lg.Info("task queued",
		zap.String("task_id", task.ID),
		zap.Int("priority", priority),
		zap.Int("ahead_in_queue", task.QueueAhead()),
		zap.Int("queued_depth", qlen),
		zap.Int32("running_now", rn),
		zap.Int("unfinished_estimate", task.UnfinishedEstimate()),
	)
	return task
}

// CancelTaskByID removes a pending task from the priority queue.
func (s *Scheduler[Params, Result]) CancelTaskByID(taskID string) bool {
	s.mu.Lock()
	ptr, ok := s.queue.Remove(taskID)
	if ok {
		s.relabelQueuedLocked()
	}
	s.mu.Unlock()
	if ok {
		s.cond.Signal()
		if t, ok := ptr.(*Task[Params, Result]); ok && t != nil {
			t.Cancel()
			var z Result
			t.deliver(z, context.Canceled)
		}
	}
	return ok
}

// GetTaskPosition returns queue position (0 = next) or -1.
func (s *Scheduler[Params, Result]) GetTaskPosition(taskID string) int {
	return s.queue.GetPosition(taskID)
}

// QueueLen is tasks waiting in the priority queue.
func (s *Scheduler[Params, Result]) QueueLen() int {
	return s.queue.Len()
}

// RunningCount is handlers currently executing.
func (s *Scheduler[Params, Result]) RunningCount() int32 {
	return atomic.LoadInt32(&s.running)
}

// Stats returns queued, running, and unfinished totals.
func (s *Scheduler[Params, Result]) Stats() Stats {
	s.mu.Lock()
	ql := s.queue.Len()
	s.mu.Unlock()
	rn := atomic.LoadInt32(&s.running)
	return Stats{
		Queued:     ql,
		ChannelLen: 0,
		Running:    rn,
		Unfinished: ql + int(rn),
	}
}

// PendingSnapshot returns pending task IDs and priorities in queue order.
func (s *Scheduler[Params, Result]) PendingSnapshot() []QueueItemSnapshot {
	return s.queue.Snapshot()
}

// Stop stops accepting new tasks, drains the wait queue with errors, and waits for workers to exit.
func (s *Scheduler[Params, Result]) Stop() error {
	if s == nil {
		return nil
	}
	if !s.stopping.CompareAndSwap(false, true) {
		return errors.New("scheduler already stopped")
	}
	s.mu.Lock()
	for s.queue.Len() > 0 {
		item := s.queue.Pop()
		t, ok := item.(*Task[Params, Result])
		if !ok || t == nil {
			continue
		}
		var z Result
		t.deliver(z, fmt.Errorf("scheduler stopped"))
	}
	s.mu.Unlock()
	s.cond.Broadcast()
	s.wg.Wait()
	return nil
}
