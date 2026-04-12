package task

// Copyright (c) 2026 LingByte
// SPDX-License-Identifier: MIT

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

type pqMock struct {
	id string
}

func TestPriorityQueue_PushPopOrder(t *testing.T) {
	pq := &PriorityQueue{}

	pq.Push(&pqMock{id: "low"}, 10, "low")
	pq.Push(&pqMock{id: "high"}, 100, "high")
	pq.Push(&pqMock{id: "mid"}, 50, "mid")

	assert.Equal(t, 3, pq.Len())
	assert.Equal(t, 0, pq.GetPosition("high"))
	assert.Equal(t, 1, pq.GetPosition("mid"))
	assert.Equal(t, 2, pq.GetPosition("low"))

	v1 := pq.Pop().(*pqMock)
	v2 := pq.Pop().(*pqMock)
	v3 := pq.Pop().(*pqMock)

	assert.Equal(t, "high", v1.id)
	assert.Equal(t, "mid", v2.id)
	assert.Equal(t, "low", v3.id)

	assert.Equal(t, 0, pq.Len())
	assert.Nil(t, pq.Pop())
}

func TestPriorityQueue_RemoveAndPosition(t *testing.T) {
	pq := &PriorityQueue{}

	pq.Push(&pqMock{id: "a"}, 10, "a")
	pq.Push(&pqMock{id: "b"}, 20, "b")
	pq.Push(&pqMock{id: "c"}, 30, "c")

	assert.Equal(t, 3, pq.Len())
	assert.Equal(t, 0, pq.GetPosition("c"))
	assert.Equal(t, 1, pq.GetPosition("b"))
	assert.Equal(t, 2, pq.GetPosition("a"))
	assert.Equal(t, -1, pq.GetPosition("missing"))

	ok := pq.Remove("b")
	assert.True(t, ok)
	assert.Equal(t, 2, pq.Len())
	assert.Equal(t, -1, pq.GetPosition("b"))
	assert.Equal(t, 0, pq.GetPosition("c"))
	assert.Equal(t, 1, pq.GetPosition("a"))

	ok = pq.Remove("missing")
	assert.False(t, ok)
}

func TestPriorityQueue_ConcurrencySmoke(t *testing.T) {
	pq := &PriorityQueue{}

	const goroutines = 8
	const perG = 200

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		g := g
		go func() {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				id := fmt.Sprintf("g%d", g)
				taskID := fmt.Sprintf("%s_%d", id, i)
				pq.Push(&pqMock{id: id}, i%10, taskID)
				_ = pq.Len()
			}
		}()
	}
	wg.Wait()

	assert.Greater(t, pq.Len(), 0)

	// Pop a few items; not asserting strict order in concurrent case.
	for i := 0; i < 10; i++ {
		_ = pq.Pop()
	}
}

func TestPriorityQueue_Empty(t *testing.T) {
	pq := &PriorityQueue{}
	assert.Equal(t, 0, pq.Len())
	assert.Nil(t, pq.Pop())
	assert.Equal(t, -1, pq.GetPosition("any"))
	assert.False(t, pq.Remove("any"))
}

// 相同 priority 时按插入顺序排在队尾，Pop 为 FIFO。
func TestPriorityQueue_SamePriorityFIFO(t *testing.T) {
	pq := &PriorityQueue{}
	pq.Push(&pqMock{id: "first"}, 1, "t1")
	pq.Push(&pqMock{id: "second"}, 1, "t2")
	pq.Push(&pqMock{id: "third"}, 1, "t3")

	assert.Equal(t, 0, pq.GetPosition("t1"))
	assert.Equal(t, 1, pq.GetPosition("t2"))
	assert.Equal(t, 2, pq.GetPosition("t3"))

	assert.Equal(t, "first", pq.Pop().(*pqMock).id)
	assert.Equal(t, "second", pq.Pop().(*pqMock).id)
	assert.Equal(t, "third", pq.Pop().(*pqMock).id)
}

func TestPriorityQueue_RemoveHeadThenPop(t *testing.T) {
	pq := &PriorityQueue{}
	pq.Push(&pqMock{id: "x"}, 100, "head")
	pq.Push(&pqMock{id: "y"}, 50, "tail")
	assert.True(t, pq.Remove("head"))
	assert.Equal(t, 1, pq.Len())
	assert.Equal(t, 0, pq.GetPosition("tail"))
	v := pq.Pop().(*pqMock)
	assert.Equal(t, "y", v.id)
	assert.Nil(t, pq.Pop())
}
