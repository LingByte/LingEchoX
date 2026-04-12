package task

// Copyright (c) 2026 LingByte
// SPDX-License-Identifier: MIT

import (
	"sort"
	"sync"
)

// 队列内部存储的元素，与泛型 Task 解耦
type queueItem struct {
	taskID   string
	priority int
	taskPtr  any // 存原始任务
}

type PriorityQueue struct {
	mu    sync.Mutex
	items []*queueItem
}

// Push 存储 taskID + priority + 原始任务
func (pq *PriorityQueue) Push(task any, priority int, taskID string) {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	item := &queueItem{
		taskID:   taskID,
		priority: priority,
		taskPtr:  task,
	}

	// 按优先级降序插入
	pos := sort.Search(len(pq.items), func(i int) bool {
		return pq.items[i].priority < priority
	})

	pq.items = append(pq.items[:pos], append([]*queueItem{item}, pq.items[pos:]...)...)
}

// Pop 返回原始任务
func (pq *PriorityQueue) Pop() any {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	if len(pq.items) == 0 {
		return nil
	}
	item := pq.items[0]
	pq.items = pq.items[1:]
	return item.taskPtr
}

func (pq *PriorityQueue) Remove(taskID string) bool {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	for i, item := range pq.items {
		if item.taskID == taskID {
			pq.items = append(pq.items[:i], pq.items[i+1:]...)
			return true
		}
	}
	return false
}

func (pq *PriorityQueue) GetPosition(taskID string) int {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	for i, item := range pq.items {
		if item.taskID == taskID {
			return i
		}
	}
	return -1
}

func (pq *PriorityQueue) Len() int {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	return len(pq.items)
}
