package pool

import (
	"container/heap"
	"sync"
)

// PriorityQueue implements a thread-safe priority queue for tasks.
type PriorityQueue struct {
	mu    sync.Mutex
	cond  *sync.Cond
	items taskHeap
	cap   int
}

// NewPriorityQueue creates a new priority queue.
func NewPriorityQueue(capacity int) *PriorityQueue {
	pq := &PriorityQueue{
		items: make(taskHeap, 0, capacity),
		cap:   capacity,
	}
	pq.cond = sync.NewCond(&pq.mu)
	heap.Init(&pq.items)
	return pq
}

// Push adds a task to the queue.
func (pq *PriorityQueue) Push(task Task) bool {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	if len(pq.items) >= pq.cap {
		return false
	}

	heap.Push(&pq.items, task)
	pq.cond.Signal()
	return true
}

// Pop removes and returns the highest priority task.
// Blocks if the queue is empty.
func (pq *PriorityQueue) Pop() (Task, bool) {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	for len(pq.items) == 0 {
		pq.cond.Wait()
	}

	task := heap.Pop(&pq.items).(Task)
	return task, true
}

// TryPop attempts to pop without blocking.
func (pq *PriorityQueue) TryPop() (Task, bool) {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	if len(pq.items) == 0 {
		return Task{}, false
	}

	task := heap.Pop(&pq.items).(Task)
	return task, true
}

// Len returns the current queue length.
func (pq *PriorityQueue) Len() int {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	return len(pq.items)
}

// Signal wakes up one waiting goroutine.
func (pq *PriorityQueue) Signal() {
	pq.cond.Signal()
}

// Broadcast wakes up all waiting goroutines.
func (pq *PriorityQueue) Broadcast() {
	pq.cond.Broadcast()
}

// taskHeap implements heap.Interface for tasks.
type taskHeap []Task

func (h taskHeap) Len() int { return len(h) }

func (h taskHeap) Less(i, j int) bool {
	// Higher priority first
	if h[i].Priority != h[j].Priority {
		return h[i].Priority > h[j].Priority
	}
	// Earlier submission time first (FIFO within same priority)
	return h[i].SubmittedAt.Before(h[j].SubmittedAt)
}

func (h taskHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *taskHeap) Push(x interface{}) {
	*h = append(*h, x.(Task))
}

func (h *taskHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}
