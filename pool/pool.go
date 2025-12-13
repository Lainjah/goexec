// Package pool provides a bounded worker pool with backpressure.
package pool

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// Common errors.
var (
	ErrPoolFull     = errors.New("worker pool is full")
	ErrPoolShutdown = errors.New("worker pool is shutdown")
	ErrTaskTimeout  = errors.New("task timed out waiting for worker")
)

// Task represents a unit of work for the pool.
type Task struct {
	SubmittedAt time.Time
	Fn          func()
	Priority    int
}

// Pool manages a bounded pool of workers.
type Pool interface {
	// Submit submits a task to the pool.
	Submit(ctx context.Context, task Task) error

	// SubmitFunc submits a function to the pool.
	SubmitFunc(ctx context.Context, fn func()) error

	// Stats returns current pool statistics.
	Stats() Stats

	// Resize dynamically adjusts pool size.
	Resize(workers int) error

	// Shutdown gracefully shuts down the pool.
	Shutdown(ctx context.Context) error
}

// Config configures the worker pool.
type Config struct {
	// MinWorkers is the minimum number of workers.
	MinWorkers int

	// MaxWorkers is the maximum number of workers.
	MaxWorkers int

	// QueueSize is the size of the task queue.
	QueueSize int

	// BackpressureStrategy defines behavior when queue is full.
	BackpressureStrategy BackpressureStrategy

	// IdleTimeout is how long an idle worker waits before exiting.
	IdleTimeout time.Duration

	// TaskTimeout is the default timeout for tasks.
	TaskTimeout time.Duration

	// EnablePriorityQueue enables priority-based scheduling.
	EnablePriorityQueue bool
}

// BackpressureStrategy defines how to handle a full queue.
type BackpressureStrategy int

const (
	// StrategyBlock blocks until space is available.
	StrategyBlock BackpressureStrategy = iota

	// StrategyReject immediately rejects new tasks.
	StrategyReject

	// StrategyDropOldest drops the oldest task to make room.
	StrategyDropOldest

	// StrategyCallerRuns executes in the caller's goroutine.
	StrategyCallerRuns
)

// Stats contains pool statistics.
type Stats struct {
	ActiveWorkers  int32
	IdleWorkers    int32
	QueueLength    int32
	QueueCapacity  int32
	TotalSubmitted int64
	TotalCompleted int64
	TotalRejected  int64
	TotalTimeout   int64
	AvgWaitTime    time.Duration
	AvgExecTime    time.Duration
}

// pool is the concrete implementation.
type pool struct {
	taskQueue  chan Task
	stats      *stats
	shutdownCh chan struct{}
	workers    []*worker
	config     Config
	wg         sync.WaitGroup
	workersMu  sync.RWMutex
	shutdown   int32
}

// stats tracks pool statistics.
type stats struct {
	activeWorkers  int32
	idleWorkers    int32
	totalSubmitted int64
	totalCompleted int64
	totalRejected  int64
	totalTimeout   int64
	totalWaitTime  int64
	totalExecTime  int64
}

// DefaultConfig returns default pool configuration.
func DefaultConfig() Config {
	return Config{
		MinWorkers:           4,
		MaxWorkers:           32,
		QueueSize:            1000,
		BackpressureStrategy: StrategyBlock,
		IdleTimeout:          30 * time.Second,
		TaskTimeout:          60 * time.Second,
		EnablePriorityQueue:  false,
	}
}

// New creates a new worker pool.
func New(config Config) (Pool, error) {
	if config.MinWorkers <= 0 {
		config.MinWorkers = 1
	}
	if config.MaxWorkers < config.MinWorkers {
		config.MaxWorkers = config.MinWorkers
	}
	if config.QueueSize <= 0 {
		config.QueueSize = config.MaxWorkers * 10
	}

	p := &pool{
		config:     config,
		taskQueue:  make(chan Task, config.QueueSize),
		workers:    make([]*worker, 0, config.MaxWorkers),
		stats:      &stats{},
		shutdownCh: make(chan struct{}),
	}

	// Start minimum workers
	for i := 0; i < config.MinWorkers; i++ {
		p.startWorker()
	}

	// Start autoscaler
	go p.autoscale()

	return p, nil
}

// Submit implements Pool.Submit.
func (p *pool) Submit(ctx context.Context, task Task) error {
	if atomic.LoadInt32(&p.shutdown) == 1 {
		return ErrPoolShutdown
	}

	task.SubmittedAt = time.Now()
	atomic.AddInt64(&p.stats.totalSubmitted, 1)

	switch p.config.BackpressureStrategy {
	case StrategyBlock:
		return p.submitBlocking(ctx, task)

	case StrategyReject:
		return p.submitNonBlocking(task)

	case StrategyCallerRuns:
		return p.submitCallerRuns(ctx, task)

	case StrategyDropOldest:
		return p.submitDropOldest(task)

	default:
		return p.submitBlocking(ctx, task)
	}
}

// SubmitFunc implements Pool.SubmitFunc.
func (p *pool) SubmitFunc(ctx context.Context, fn func()) error {
	return p.Submit(ctx, Task{Fn: fn})
}

func (p *pool) submitBlocking(ctx context.Context, task Task) error {
	select {
	case p.taskQueue <- task:
		return nil
	case <-ctx.Done():
		atomic.AddInt64(&p.stats.totalTimeout, 1)
		return ctx.Err()
	case <-p.shutdownCh:
		return ErrPoolShutdown
	}
}

func (p *pool) submitNonBlocking(task Task) error {
	select {
	case p.taskQueue <- task:
		return nil
	default:
		atomic.AddInt64(&p.stats.totalRejected, 1)
		return ErrPoolFull
	}
}

func (p *pool) submitCallerRuns(_ context.Context, task Task) error {
	select {
	case p.taskQueue <- task:
		return nil
	default:
		// Execute in caller's goroutine
		p.executeTask(task)
		return nil
	}
}

func (p *pool) submitDropOldest(task Task) error {
	select {
	case p.taskQueue <- task:
		return nil
	default:
		// Try to drop oldest
		select {
		case <-p.taskQueue:
			atomic.AddInt64(&p.stats.totalRejected, 1)
		default:
		}
		// Try again
		select {
		case p.taskQueue <- task:
			return nil
		default:
			atomic.AddInt64(&p.stats.totalRejected, 1)
			return ErrPoolFull
		}
	}
}

// Stats implements Pool.Stats.
func (p *pool) Stats() Stats {
	// Note: len() and cap() on channels are safe to call concurrently
	// They provide an instantaneous snapshot which is acceptable for stats

	// Safely convert int to int32, clamping to max int32 to prevent overflow
	queueLen := len(p.taskQueue)
	queueCap := cap(p.taskQueue)
	const maxInt32 = int32(^uint32(0) >> 1)

	var queueLen32 int32
	var queueCap32 int32
	if queueLen > int(maxInt32) {
		queueLen32 = maxInt32
	} else {
		queueLen32 = int32(queueLen)
	}
	if queueCap > int(maxInt32) {
		queueCap32 = maxInt32
	} else {
		queueCap32 = int32(queueCap)
	}

	return Stats{
		ActiveWorkers:  atomic.LoadInt32(&p.stats.activeWorkers),
		IdleWorkers:    atomic.LoadInt32(&p.stats.idleWorkers),
		QueueLength:    queueLen32,
		QueueCapacity:  queueCap32,
		TotalSubmitted: atomic.LoadInt64(&p.stats.totalSubmitted),
		TotalCompleted: atomic.LoadInt64(&p.stats.totalCompleted),
		TotalRejected:  atomic.LoadInt64(&p.stats.totalRejected),
		TotalTimeout:   atomic.LoadInt64(&p.stats.totalTimeout),
		AvgWaitTime:    p.avgWaitTime(),
		AvgExecTime:    p.avgExecTime(),
	}
}

// Resize implements Pool.Resize.
func (p *pool) Resize(workers int) error {
	if workers < 1 {
		workers = 1
	}

	p.workersMu.Lock()
	defer p.workersMu.Unlock()

	current := len(p.workers)
	if workers > current {
		// Add workers
		for i := current; i < workers && i < p.config.MaxWorkers; i++ {
			p.startWorkerLocked()
		}
	}
	// Note: we don't forcibly remove workers; they'll exit naturally on idle timeout

	return nil
}

// Shutdown implements Pool.Shutdown.
func (p *pool) Shutdown(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&p.shutdown, 0, 1) {
		return nil // Already shutdown
	}

	close(p.shutdownCh)

	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// worker represents a pool worker.
type worker struct {
	pool *pool
	stop chan struct{}
	id   int
}

func (p *pool) startWorker() {
	p.workersMu.Lock()
	defer p.workersMu.Unlock()
	p.startWorkerLocked()
}

func (p *pool) startWorkerLocked() {
	w := &worker{
		id:   len(p.workers),
		pool: p,
		stop: make(chan struct{}),
	}
	p.workers = append(p.workers, w)
	p.wg.Add(1)
	go w.run()
}

func (w *worker) run() {
	defer w.pool.wg.Done()

	idleTimer := time.NewTimer(w.pool.config.IdleTimeout)
	defer idleTimer.Stop()

	// Track idle state to avoid race conditions in counter updates
	isIdle := true

	for {
		if isIdle {
			atomic.AddInt32(&w.pool.stats.idleWorkers, 1)
		}

		select {
		case task, ok := <-w.pool.taskQueue:
			if isIdle {
				atomic.AddInt32(&w.pool.stats.idleWorkers, -1)
			}

			if !ok {
				return
			}

			isIdle = false
			atomic.AddInt32(&w.pool.stats.activeWorkers, 1)
			w.pool.executeTask(task)
			atomic.AddInt32(&w.pool.stats.activeWorkers, -1)

			idleTimer.Reset(w.pool.config.IdleTimeout)
			isIdle = true

		case <-idleTimer.C:
			if isIdle {
				atomic.AddInt32(&w.pool.stats.idleWorkers, -1)
			}

			// Check if we can reduce workers
			if w.pool.canReduceWorkers() {
				return
			}
			idleTimer.Reset(w.pool.config.IdleTimeout)
			isIdle = true

		case <-w.stop:
			if isIdle {
				atomic.AddInt32(&w.pool.stats.idleWorkers, -1)
			}
			return

		case <-w.pool.shutdownCh:
			if isIdle {
				atomic.AddInt32(&w.pool.stats.idleWorkers, -1)
			}

			// Drain remaining tasks
			for {
				select {
				case task, ok := <-w.pool.taskQueue:
					if !ok {
						return
					}
					w.pool.executeTask(task)
				default:
					return
				}
			}
		}
	}
}

func (p *pool) executeTask(task Task) {
	start := time.Now()
	waitTime := start.Sub(task.SubmittedAt)
	atomic.AddInt64(&p.stats.totalWaitTime, int64(waitTime))

	defer func() {
		if r := recover(); r != nil {
			// Log panic but don't crash worker
			_ = r
		}
		execTime := time.Since(start)
		atomic.AddInt64(&p.stats.totalExecTime, int64(execTime))
		atomic.AddInt64(&p.stats.totalCompleted, 1)
	}()

	if task.Fn != nil {
		task.Fn()
	}
}

func (p *pool) canReduceWorkers() bool {
	p.workersMu.RLock()
	defer p.workersMu.RUnlock()
	return len(p.workers) > p.config.MinWorkers
}

func (p *pool) autoscale() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.maybeScale()
		case <-p.shutdownCh:
			return
		}
	}
}

func (p *pool) maybeScale() {
	queueLen := len(p.taskQueue)
	queueCap := cap(p.taskQueue)
	utilization := float64(queueLen) / float64(queueCap)

	p.workersMu.Lock()
	defer p.workersMu.Unlock()

	currentWorkers := len(p.workers)

	// Scale up if queue is > 75% full
	if utilization > 0.75 && currentWorkers < p.config.MaxWorkers {
		toAdd := (p.config.MaxWorkers - currentWorkers) / 2
		if toAdd < 1 {
			toAdd = 1
		}
		for i := 0; i < toAdd; i++ {
			p.startWorkerLocked()
		}
	}
}

func (p *pool) avgWaitTime() time.Duration {
	completed := atomic.LoadInt64(&p.stats.totalCompleted)
	if completed == 0 {
		return 0
	}
	waitTime := atomic.LoadInt64(&p.stats.totalWaitTime)
	// Check for overflow: ensure result fits in time.Duration (int64)
	// time.Duration max value is about 292 years, so we check against max int64
	const maxDurationNs = int64(^uint64(0) >> 1) // Max int64
	if waitTime > maxDurationNs || completed > maxDurationNs {
		// Avoid overflow by using floating point division
		avg := float64(waitTime) / float64(completed)
		// Clamp to max time.Duration
		if avg > float64(maxDurationNs) {
			return time.Duration(maxDurationNs)
		}
		return time.Duration(avg)
	}
	return time.Duration(waitTime / completed)
}

func (p *pool) avgExecTime() time.Duration {
	completed := atomic.LoadInt64(&p.stats.totalCompleted)
	if completed == 0 {
		return 0
	}
	execTime := atomic.LoadInt64(&p.stats.totalExecTime)
	// Check for overflow: ensure result fits in time.Duration (int64)
	const maxDurationNs = int64(^uint64(0) >> 1) // Max int64
	if execTime > maxDurationNs || completed > maxDurationNs {
		// Avoid overflow by using floating point division
		avg := float64(execTime) / float64(completed)
		// Clamp to max time.Duration
		if avg > float64(maxDurationNs) {
			return time.Duration(maxDurationNs)
		}
		return time.Duration(avg)
	}
	return time.Duration(execTime / completed)
}
