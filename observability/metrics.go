package observability

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/victoralfred/goexec/executor"
)

// Metrics provides execution metrics.
type Metrics struct {
	binaryStats        map[string]*BinaryStats
	totalDuration      int64
	minDuration        int64
	timeoutExec        int64
	policyDenied       int64
	sandboxViolations  int64
	rateLimited        int64
	failedExec         int64
	circuitBreakerOpen int64
	durationCount      int64
	totalExecutions    int64
	maxDuration        int64
	totalCPUTime       int64
	totalMemoryPeak    int64
	memoryCount        int64
	successfulExec     int64
	mu                 sync.RWMutex
}

// BinaryStats contains per-binary statistics.
type BinaryStats struct {
	LastExecutionAt time.Time
	Binary          string
	LastStatus      string
	TotalExecutions int64
	SuccessfulExec  int64
	FailedExec      int64
	TotalDuration   int64
	AvgDuration     int64
}

// NewMetrics creates a new metrics collector.
func NewMetrics() *Metrics {
	return &Metrics{
		binaryStats: make(map[string]*BinaryStats),
		minDuration: -1,
	}
}

// RecordExecution records an execution result.
func (m *Metrics) RecordExecution(cmd *executor.Command, result *executor.Result, err error) {
	atomic.AddInt64(&m.totalExecutions, 1)

	// Record status
	switch result.Status {
	case executor.StatusSuccess:
		atomic.AddInt64(&m.successfulExec, 1)
	case executor.StatusTimeout:
		atomic.AddInt64(&m.timeoutExec, 1)
		atomic.AddInt64(&m.failedExec, 1)
	case executor.StatusPolicyDenied:
		atomic.AddInt64(&m.policyDenied, 1)
		atomic.AddInt64(&m.failedExec, 1)
	case executor.StatusSandboxViolation:
		atomic.AddInt64(&m.sandboxViolations, 1)
		atomic.AddInt64(&m.failedExec, 1)
	case executor.StatusRateLimited:
		atomic.AddInt64(&m.rateLimited, 1)
		atomic.AddInt64(&m.failedExec, 1)
	case executor.StatusCircuitOpen:
		atomic.AddInt64(&m.circuitBreakerOpen, 1)
		atomic.AddInt64(&m.failedExec, 1)
	default:
		if err != nil || result.ExitCode != 0 {
			atomic.AddInt64(&m.failedExec, 1)
		} else {
			atomic.AddInt64(&m.successfulExec, 1)
		}
	}

	// Record duration
	duration := result.Duration.Nanoseconds()
	atomic.AddInt64(&m.totalDuration, duration)
	atomic.AddInt64(&m.durationCount, 1)

	// Update min/max
	for {
		old := atomic.LoadInt64(&m.minDuration)
		if old >= 0 && duration >= old {
			break
		}
		if atomic.CompareAndSwapInt64(&m.minDuration, old, duration) {
			break
		}
	}

	for {
		old := atomic.LoadInt64(&m.maxDuration)
		if duration <= old {
			break
		}
		if atomic.CompareAndSwapInt64(&m.maxDuration, old, duration) {
			break
		}
	}

	// Record resource usage
	if result.CPUTime > 0 {
		atomic.AddInt64(&m.totalCPUTime, result.CPUTime.Nanoseconds())
	}
	if result.MemoryPeak > 0 {
		atomic.AddInt64(&m.totalMemoryPeak, result.MemoryPeak)
		atomic.AddInt64(&m.memoryCount, 1)
	}

	// Update per-binary stats
	m.updateBinaryStats(cmd.Binary, result)
}

func (m *Metrics) updateBinaryStats(binary string, result *executor.Result) {
	m.mu.Lock()
	defer m.mu.Unlock()

	stats, ok := m.binaryStats[binary]
	if !ok {
		stats = &BinaryStats{Binary: binary}
		m.binaryStats[binary] = stats
	}

	stats.TotalExecutions++
	stats.TotalDuration += result.Duration.Nanoseconds()
	stats.AvgDuration = stats.TotalDuration / stats.TotalExecutions
	stats.LastExecutionAt = time.Now()
	stats.LastStatus = result.Status.String()

	if result.Status == executor.StatusSuccess {
		stats.SuccessfulExec++
	} else {
		stats.FailedExec++
	}
}

// Snapshot returns a snapshot of current metrics.
func (m *Metrics) Snapshot() MetricsSnapshot {
	return MetricsSnapshot{
		TotalExecutions:    atomic.LoadInt64(&m.totalExecutions),
		SuccessfulExec:     atomic.LoadInt64(&m.successfulExec),
		FailedExec:         atomic.LoadInt64(&m.failedExec),
		TimeoutExec:        atomic.LoadInt64(&m.timeoutExec),
		PolicyDenied:       atomic.LoadInt64(&m.policyDenied),
		SandboxViolations:  atomic.LoadInt64(&m.sandboxViolations),
		RateLimited:        atomic.LoadInt64(&m.rateLimited),
		CircuitBreakerOpen: atomic.LoadInt64(&m.circuitBreakerOpen),
		AvgDuration:        m.avgDuration(),
		MinDuration:        time.Duration(atomic.LoadInt64(&m.minDuration)),
		MaxDuration:        time.Duration(atomic.LoadInt64(&m.maxDuration)),
		AvgCPUTime:         m.avgCPUTime(),
		AvgMemoryPeak:      m.avgMemoryPeak(),
		BinaryStats:        m.getBinaryStats(),
	}
}

// MetricsSnapshot is a point-in-time snapshot of metrics.
type MetricsSnapshot struct {
	BinaryStats        map[string]*BinaryStats
	RateLimited        int64
	FailedExec         int64
	TimeoutExec        int64
	PolicyDenied       int64
	SandboxViolations  int64
	TotalExecutions    int64
	CircuitBreakerOpen int64
	AvgDuration        time.Duration
	MinDuration        time.Duration
	MaxDuration        time.Duration
	AvgCPUTime         time.Duration
	AvgMemoryPeak      int64
	SuccessfulExec     int64
}

// SuccessRate returns the success rate as a percentage.
func (s MetricsSnapshot) SuccessRate() float64 {
	if s.TotalExecutions == 0 {
		return 0
	}
	return float64(s.SuccessfulExec) / float64(s.TotalExecutions) * 100
}

// ErrorRate returns the error rate as a percentage.
func (s MetricsSnapshot) ErrorRate() float64 {
	if s.TotalExecutions == 0 {
		return 0
	}
	return float64(s.FailedExec) / float64(s.TotalExecutions) * 100
}

func (m *Metrics) avgDuration() time.Duration {
	count := atomic.LoadInt64(&m.durationCount)
	if count == 0 {
		return 0
	}
	return time.Duration(atomic.LoadInt64(&m.totalDuration) / count)
}

func (m *Metrics) avgCPUTime() time.Duration {
	count := atomic.LoadInt64(&m.durationCount)
	if count == 0 {
		return 0
	}
	return time.Duration(atomic.LoadInt64(&m.totalCPUTime) / count)
}

func (m *Metrics) avgMemoryPeak() int64 {
	count := atomic.LoadInt64(&m.memoryCount)
	if count == 0 {
		return 0
	}
	return atomic.LoadInt64(&m.totalMemoryPeak) / count
}

func (m *Metrics) getBinaryStats() map[string]*BinaryStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*BinaryStats, len(m.binaryStats))
	for k, v := range m.binaryStats {
		// Copy stats
		copied := *v
		result[k] = &copied
	}
	return result
}

// Reset resets all metrics.
func (m *Metrics) Reset() {
	atomic.StoreInt64(&m.totalExecutions, 0)
	atomic.StoreInt64(&m.successfulExec, 0)
	atomic.StoreInt64(&m.failedExec, 0)
	atomic.StoreInt64(&m.timeoutExec, 0)
	atomic.StoreInt64(&m.policyDenied, 0)
	atomic.StoreInt64(&m.sandboxViolations, 0)
	atomic.StoreInt64(&m.rateLimited, 0)
	atomic.StoreInt64(&m.circuitBreakerOpen, 0)
	atomic.StoreInt64(&m.totalDuration, 0)
	atomic.StoreInt64(&m.durationCount, 0)
	atomic.StoreInt64(&m.minDuration, -1)
	atomic.StoreInt64(&m.maxDuration, 0)
	atomic.StoreInt64(&m.totalCPUTime, 0)
	atomic.StoreInt64(&m.totalMemoryPeak, 0)
	atomic.StoreInt64(&m.memoryCount, 0)

	m.mu.Lock()
	m.binaryStats = make(map[string]*BinaryStats)
	m.mu.Unlock()
}
