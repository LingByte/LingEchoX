package task

// Copyright (c) 2026 LingByte
// SPDX-License-Identifier: MIT

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestScheduler_SubmitAndWait(t *testing.T) {
	scheduler := NewScheduler[string, string](2, nil)

	task := scheduler.SubmitTask(
		context.Background(),
		10,
		"test",
		func(ctx context.Context, param string) (string, error) {
			return "ok_" + param, nil
		},
	)

	res := <-task.Result
	err := <-task.Err

	assert.NoError(t, err)
	assert.Equal(t, "ok_test", res)
	assert.Equal(t, TaskStatusSuccess, task.Status.Load().(TaskStatus))
}

func TestScheduler_Priority(t *testing.T) {
	scheduler := NewScheduler[int, int](1, nil)
	var log []int

	// 低优先级
	scheduler.SubmitTask(context.Background(), 10, 1, func(ctx context.Context, p int) (int, error) {
		log = append(log, p)
		return 0, nil
	})

	// VIP 高优先级
	scheduler.SubmitTask(context.Background(), 100, 999, func(ctx context.Context, p int) (int, error) {
		log = append(log, p)
		return 0, nil
	})

	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, []int{999, 1}, log)
}
func TestScheduler_TaskPosition(t *testing.T) {
	scheduler := NewScheduler[int, int](1, nil)

	// -------------- 关键：创建一个永远阻塞的任务，占死 worker --------------
	blockChan := make(chan struct{})
	blockTask := scheduler.SubmitTask(
		context.Background(),
		10,
		9999,
		func(ctx context.Context, p int) (int, error) {
			<-blockChan // 永远阻塞
			return 0, nil
		},
	)

	// 等待阻塞任务开始运行
	time.Sleep(50 * time.Millisecond)

	// -------------- 提交两个任务，一定会进入队列 --------------
	task2 := scheduler.SubmitTask(context.Background(), 10, 2, nilHandler[int, int])
	task3 := scheduler.SubmitTask(context.Background(), 10, 3, nilHandler[int, int])

	// 等待入队
	time.Sleep(50 * time.Millisecond)

	// -------------- 断言队列位置 --------------
	// task2 排在第 0 位（前面有 0 个）
	assert.Equal(t, 0, scheduler.GetTaskPosition(task2.ID))
	// task3 排在第 1 位（前面有 1 个）
	assert.Equal(t, 1, scheduler.GetTaskPosition(task3.ID))
	// 不存在的 ID 返回 -1
	assert.Equal(t, -1, scheduler.GetTaskPosition("fake-id"))

	// 释放阻塞任务
	close(blockChan)
	<-blockTask.Result
	<-blockTask.Err
}

func TestScheduler_CancelTaskByID(t *testing.T) {
	scheduler := NewScheduler[int, int](1, nil)

	block := make(chan struct{})
	blockTask := scheduler.SubmitTask(context.Background(), 10, 0, func(ctx context.Context, p int) (int, error) {
		<-block
		return 0, nil
	})

	task := scheduler.SubmitTask(context.Background(), 10, 100, nilHandler[int, int])
	ok := scheduler.CancelTaskByID(task.ID)
	assert.True(t, ok)

	close(block)
	<-blockTask.Result
	<-blockTask.Err
}

func nilHandler[P, R any](ctx context.Context, p P) (R, error) {
	var zero R
	return zero, nil
}
