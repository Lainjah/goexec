package pool

import (
	"sync"
	"testing"
	"time"
)

func TestNewPriorityQueue(t *testing.T) {
	pq := NewPriorityQueue(10)
	if pq == nil {
		t.Fatal("NewPriorityQueue returned nil")
	}

	if pq.Len() != 0 {
		t.Errorf("Expected empty queue, got length %d", pq.Len())
	}
}

func TestPriorityQueue_Push(t *testing.T) {
	pq := NewPriorityQueue(10)

	success := pq.Push(Task{
		Priority: 3,
	})
	if !success {
		t.Error("Push should succeed when queue has capacity")
	}

	if pq.Len() != 1 {
		t.Errorf("Expected length 1, got %d", pq.Len())
	}
}

func TestPriorityQueue_Push_Full(t *testing.T) {
	pq := NewPriorityQueue(2)

	// Fill queue
	pq.Push(Task{Priority: 1})
	pq.Push(Task{Priority: 2})

	// Should reject when full
	success := pq.Push(Task{Priority: 3})
	if success {
		t.Error("Push should fail when queue is full")
	}
}

func TestPriorityQueue_Pop(t *testing.T) {
	pq := NewPriorityQueue(10)

	// Push tasks with different priorities
	pq.Push(Task{
		Priority:    1,
		SubmittedAt: time.Now(),
	})
	pq.Push(Task{
		Priority:    3,
		SubmittedAt: time.Now().Add(time.Second),
	})
	pq.Push(Task{
		Priority:    2,
		SubmittedAt: time.Now().Add(2 * time.Second),
	})

	// Pop should return highest priority first
	task, ok := pq.Pop()
	if !ok {
		t.Fatal("Pop should succeed")
	}
	if task.Priority != 3 {
		t.Errorf("Expected 3, got %v", task.Priority)
	}

	// Next should be Normal
	task, ok = pq.Pop()
	if !ok {
		t.Fatal("Pop should succeed")
	}
	if task.Priority != 2 {
		t.Errorf("Expected 2, got %v", task.Priority)
	}

	// Last should be Low
	task, ok = pq.Pop()
	if !ok {
		t.Fatal("Pop should succeed")
	}
	if task.Priority != 1 {
		t.Errorf("Expected 1, got %v", task.Priority)
	}
}

func TestPriorityQueue_Pop_Blocks(t *testing.T) {
	pq := NewPriorityQueue(10)

	done := make(chan bool)

	// Start a goroutine that will block on Pop
	go func() {
		_, ok := pq.Pop()
		done <- ok
	}()

	// Give it time to block
	time.Sleep(10 * time.Millisecond)

	// Push a task - should unblock the pop
	pq.Push(Task{Priority: 2})

	select {
	case ok := <-done:
		if !ok {
			t.Error("Pop should succeed after push")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Pop should have unblocked")
	}
}

func TestPriorityQueue_TryPop(t *testing.T) {
	pq := NewPriorityQueue(10)

	// Try pop on empty queue
	_, ok := pq.TryPop()
	if ok {
		t.Error("TryPop should fail on empty queue")
	}

	// Push and try pop
	pq.Push(Task{Priority: 3})
	task, ok := pq.TryPop()
	if !ok {
		t.Error("TryPop should succeed when queue has items")
	}
	if task.Priority != 3 {
		t.Errorf("Expected 3, got %v", task.Priority)
	}
}

func TestPriorityQueue_PriorityOrdering(t *testing.T) {
	pq := NewPriorityQueue(10)

	priorities := []int{
		1,
		4,
		2,
		3,
	}

	for _, p := range priorities {
		pq.Push(Task{Priority: p, SubmittedAt: time.Now()})
	}

	// Should pop in priority order (highest first)
	expectedOrder := []int{
		4,
		3,
		2,
		1,
	}

	for _, expected := range expectedOrder {
		task, ok := pq.TryPop()
		if !ok {
			t.Fatal("TryPop should succeed")
		}
		if task.Priority != expected {
			t.Errorf("Expected %v, got %v", expected, task.Priority)
		}
	}
}

func TestPriorityQueue_FIFO_WithinPriority(t *testing.T) {
	pq := NewPriorityQueue(10)

	now := time.Now()
	pq.Push(Task{
		Priority:    2,
		SubmittedAt: now.Add(1 * time.Second),
	})
	pq.Push(Task{
		Priority:    2,
		SubmittedAt: now,
	})
	pq.Push(Task{
		Priority:    2,
		SubmittedAt: now.Add(2 * time.Second),
	})

	// Should pop in submission order (FIFO within same priority)
	first, _ := pq.TryPop()
	if !first.SubmittedAt.Equal(now) {
		t.Errorf("Expected first submitted task, got %v", first.SubmittedAt)
	}

	second, _ := pq.TryPop()
	if !second.SubmittedAt.Equal(now.Add(1 * time.Second)) {
		t.Errorf("Expected second submitted task, got %v", second.SubmittedAt)
	}

	third, _ := pq.TryPop()
	if !third.SubmittedAt.Equal(now.Add(2 * time.Second)) {
		t.Errorf("Expected third submitted task, got %v", third.SubmittedAt)
	}
}

func TestPriorityQueue_ConcurrentAccess(t *testing.T) {
	pq := NewPriorityQueue(100)

	var wg sync.WaitGroup
	concurrency := 20

	// Concurrent pushes
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			pq.Push(Task{
				Priority:    id % 4,
				SubmittedAt: time.Now(),
			})
		}(i)
	}

	// Concurrent pops
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pq.Pop()
		}()
	}

	wg.Wait()
	// Should not panic
}

func TestPriorityQueue_Signal(t *testing.T) {
	pq := NewPriorityQueue(10)

	// Signal should not panic
	pq.Signal()
}

func TestPriorityQueue_Broadcast(t *testing.T) {
	pq := NewPriorityQueue(10)

	// Broadcast should not panic
	pq.Broadcast()
}

func TestPriorityQueue_Len(t *testing.T) {
	pq := NewPriorityQueue(10)

	if pq.Len() != 0 {
		t.Errorf("Expected length 0, got %d", pq.Len())
	}

	pq.Push(Task{Priority: 2})
	if pq.Len() != 1 {
		t.Errorf("Expected length 1, got %d", pq.Len())
	}

	pq.Push(Task{Priority: 3})
	if pq.Len() != 2 {
		t.Errorf("Expected length 2, got %d", pq.Len())
	}

	pq.TryPop()
	if pq.Len() != 1 {
		t.Errorf("Expected length 1 after pop, got %d", pq.Len())
	}
}

