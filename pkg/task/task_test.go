package task

// Copyright (c) 2026 LingByte
// SPDX-License-Identifier: MIT

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewNewTaskPool(t *testing.T) {
	pool := NewTaskPool[string, string](&TaskPoolOption{
		WorkerCount: 10,
	})
	assert.NotNil(t, pool)
	assert.IsType(t, &TaskPool[string, string]{}, pool)
	assert.Equal(t, pool, pool)
	assert.NotNil(t, pool.logger)
}

func TestTaskPoolRun(t *testing.T) {
	pool := NewTaskPool[string, string](&TaskPoolOption{
		WorkerCount: 10,
	})
	defer pool.Wait()
	// 1. 提交一个正常任务
	task, err := pool.AddTask(
		context.Background(),
		"test_param",
		func(ctx context.Context, param string) (string, error) {
			return "hello_" + param, nil
		},
	)
	assert.NoError(t, err)
	assert.NotEmpty(t, task.ID)
	assert.NotNil(t, task.Result)
	assert.NotNil(t, task.Err)
	// 2. 获取结果
	res := <-task.Result
	err = <-task.Err
	assert.NoError(t, err)
	assert.Equal(t, "hello_test_param", res)
}

// TestTaskPoolError 任务返回错误测试
func TestTaskPoolError(t *testing.T) {
	pool := NewTaskPool[int, string](&TaskPoolOption{
		WorkerCount: 10,
	})
	defer pool.Wait()

	task, err := pool.AddTask(
		context.Background(),
		100,
		func(ctx context.Context, param int) (string, error) {
			return "", errors.New("mock error")
		},
	)

	assert.NoError(t, err)

	<-task.Result
	errRes := <-task.Err

	assert.ErrorContains(t, errRes, "mock error")
}

// TestTaskPoolConcurrent 并发提交任务测试
func TestTaskPoolConcurrent(t *testing.T) {
	pool := NewTaskPool[int, int](&TaskPoolOption{
		WorkerCount: 10,
	})
	defer pool.Wait()

	taskCount := 10
	var tasks []*Task[int, int]

	// 并发提交 10 个任务
	for i := 0; i < taskCount; i++ {
		n := i
		task, _ := pool.AddTask(
			context.Background(),
			n,
			func(ctx context.Context, param int) (int, error) {
				time.Sleep(10 * time.Millisecond)
				return param * 2, nil
			},
		)
		tasks = append(tasks, task)
	}

	// 确保所有任务返回正确
	for _, task := range tasks {
		res := <-task.Result
		<-task.Err
		assert.True(t, res%2 == 0)
	}
}

// TestTaskPoolTaskID_Unique 测试任务ID唯一
func TestTaskPoolTaskID_Unique(t *testing.T) {
	pool := NewTaskPool[int, int](&TaskPoolOption{
		WorkerCount: 10,
	})
	defer pool.Wait()

	idMap := make(map[string]bool)
	count := 100

	for i := 0; i < count; i++ {
		task, _ := pool.AddTask(
			context.Background(),
			i,
			func(ctx context.Context, param int) (int, error) {
				return 0, nil
			},
		)
		// 确保不重复
		assert.False(t, idMap[task.ID])
		idMap[task.ID] = true
	}

	assert.Equal(t, count, len(idMap))
}

// TestTaskPoolQueueAndRunning 测试排队数 & 运行中计数
func TestTaskPoolQueueAndRunning(t *testing.T) {
	pool := NewTaskPool[int, int](&TaskPoolOption{
		WorkerCount: 2,
	})
	defer pool.Wait()
	block := make(chan struct{})
	for i := 0; i < 5; i++ {
		pool.AddTask(
			context.Background(),
			i,
			func(ctx context.Context, param int) (int, error) {
				<-block // 阻塞
				return 0, nil
			},
		)
	}
	time.Sleep(30 * time.Millisecond)
	// 协程池大小=2，所以运行中=2，排队=3
	assert.Equal(t, int32(2), pool.Running())
	assert.Equal(t, 3, pool.QueueLen())
	close(block) // 释放
}

// TestTaskPoolWait 测试 Wait 能正确等待所有任务完成
func TestTaskPoolWait(t *testing.T) {
	pool := NewTaskPool[int, int](&TaskPoolOption{
		WorkerCount: 10,
	})
	var count int32
	for i := 0; i < 5; i++ {
		pool.AddTask(
			context.Background(),
			i,
			func(ctx context.Context, param int) (int, error) {
				atomic.AddInt32(&count, 1)
				time.Sleep(10 * time.Millisecond)
				return 0, nil
			},
		)
	}
	pool.Wait()
	assert.Equal(t, int32(5), atomic.LoadInt32(&count))
}

// TestTaskPool_Context_Value 测试任务使用 context 传值
func TestTaskPool_Context_Value(t *testing.T) {
	pool := NewTaskPool[string, string](&TaskPoolOption{
		WorkerCount: 10,
	})
	defer pool.Wait()
	// 1. 往 context 里放值
	ctx := context.WithValue(context.Background(), "user_id", "ling_123456")
	// 2. 提交任务，任务内部从 ctx 取值
	task, _ := pool.AddTask(
		ctx,
		"test_param",
		func(ctx context.Context, param string) (string, error) {
			// 从 ctx 取出用户ID
			userID, ok := ctx.Value("user_id").(string)
			if !ok {
				return "", errors.New("user_id not found")
			}
			return "user:" + userID + ", param:" + param, nil
		},
	)
	// 3. 获取结果
	res := <-task.Result
	err := <-task.Err
	assert.NoError(t, err)
	assert.Equal(t, "user:ling_123456, param:test_param", res)
}

// TestTaskPool_Context_Cancel 测试 context 取消任务
func TestTaskPool_Context_Cancel(t *testing.T) {
	pool := NewTaskPool[int, string](&TaskPoolOption{
		WorkerCount: 10,
	})
	defer pool.Wait()

	// 1. 创建可取消的 ctx
	ctx, cancel := context.WithCancel(context.Background())

	// 2. 提交任务，内部检查 ctx 是否取消
	task, _ := pool.AddTask(
		ctx,
		100,
		func(ctx context.Context, param int) (string, error) {
			// 模拟耗时操作
			time.Sleep(1 * time.Second)
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			return "success", nil
		},
	)
	// 3. 立刻取消（任务还没执行完）
	cancel()
	// 4. 应该收到  context canceled 错误
	err := <-task.Err
	<-task.Result
	assert.ErrorIs(t, err, context.Canceled)
}

// TestTaskPoolContext_Timeout 测试 context 超时
func TestTaskPoolContext_Timeout(t *testing.T) {
	pool := NewTaskPool[int, string](&TaskPoolOption{
		WorkerCount: 10,
	})
	defer pool.Wait()
	// 1. 创建 500ms 超时的 ctx
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	// 2. 任务执行 1 秒 → 一定会超时
	task, _ := pool.AddTask(
		ctx,
		100,
		func(ctx context.Context, param int) (string, error) {
			time.Sleep(1 * time.Second) // 超时

			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			return "ok", nil
		},
	)
	// 3. 预期超时错误
	err := <-task.Err
	<-task.Result
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

// TestTaskPool_CancelTask 测试：取消正在排队/执行的任务
func TestTaskPool_CancelTask(t *testing.T) {
	// 1. 创建协程池（worker=1，方便控制排队）
	pool := NewTaskPool[string, string](&TaskPoolOption{
		WorkerCount: 1,
	})
	defer pool.Wait()

	// 2. 先提交一个阻塞任务，占满 worker
	blockChan := make(chan struct{})
	blockTask, _ := pool.AddTask(
		context.Background(),
		"block",
		func(ctx context.Context, s string) (string, error) {
			<-blockChan // 一直阻塞
			return "ok", nil
		},
	)

	// 3. 再提交第二个任务 → 这个任务会进入排队
	task, _ := pool.AddTask(
		context.Background(),
		"test-cancel",
		func(ctx context.Context, s string) (string, error) {
			// 模拟任务执行
			time.Sleep(1 * time.Second)
			return "done", nil
		},
	)

	// 4. 直接取消第二个任务
	pool.CancelTask(task)

	// 5. 等待一下，让协程池处理取消
	time.Sleep(5 * time.Millisecond)

	// 6. 断言：任务状态 = canceled
	assert.Equal(t, TaskStatusCancel, task.Status.Load().(TaskStatus))

	// 7. 释放阻塞任务，让 worker 有机会处理队列里的取消任务
	close(blockChan)
	<-blockTask.Err
	<-blockTask.Result

	// 8. 断言：错误是 context canceled
	select {
	case err := <-task.Err:
		<-task.Result
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for canceled task result")
	}
}

// TestTaskPool_CancelRunningTask 测试：取消正在运行中的任务
func TestTaskPool_CancelRunningTask(t *testing.T) {
	pool := NewTaskPool[int, int](&TaskPoolOption{
		WorkerCount: 1,
	})
	defer pool.Wait()

	// 提交一个会执行很久的任务
	task, _ := pool.AddTask(
		context.Background(),
		100,
		func(ctx context.Context, i int) (int, error) {
			// 循环，并且检查 ctx 是否取消
			for j := 0; j < 10; j++ {
				time.Sleep(200 * time.Millisecond)
				if ctx.Err() != nil {
					return 0, ctx.Err()
				}
			}
			return 200, nil
		},
	)

	// 等待任务开始运行
	time.Sleep(300 * time.Millisecond)

	// 取消正在运行的任务
	pool.CancelTask(task)

	// 等待结果
	err := <-task.Err
	<-task.Result

	// 断言取消成功
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, TaskStatusCancel, task.Status.Load().(TaskStatus))
}
