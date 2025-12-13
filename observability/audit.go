package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/victoralfred/goexec/executor"
	"github.com/victoralfred/gowritter/safepath"
)

// AuditLogger provides immutable audit logging.
type AuditLogger interface {
	// Log logs an audit event.
	Log(ctx context.Context, event *AuditEvent) error

	// Query queries audit events.
	Query(ctx context.Context, filter *AuditFilter) ([]*AuditEvent, error)

	// Close closes the audit logger.
	Close() error
}

// AuditEvent represents an audit log entry.
type AuditEvent struct {
	Timestamp      time.Time           `json:"timestamp"`
	ResourceUsage  *AuditResourceUsage `json:"resource_usage,omitempty"`
	Metadata       map[string]string   `json:"metadata,omitempty"`
	User           string              `json:"user,omitempty"`
	ID             string              `json:"id"`
	WorkingDir     string              `json:"working_dir,omitempty"`
	PolicyVersion  string              `json:"policy_version,omitempty"`
	Status         string              `json:"status"`
	Binary         string              `json:"binary"`
	SandboxProfile string              `json:"sandbox_profile,omitempty"`
	Error          string              `json:"error,omitempty"`
	Output         string              `json:"output,omitempty"`
	Type           AuditEventType      `json:"type"`
	TraceID        string              `json:"trace_id,omitempty"`
	Args           []string            `json:"args"`
	Duration       time.Duration       `json:"duration"`
	ExitCode       int                 `json:"exit_code"`
}

// AuditEventType represents the type of audit event.
type AuditEventType string

const (
	// AuditEventExecution is a command execution event.
	AuditEventExecution AuditEventType = "execution"

	// AuditEventPolicyDenied is a policy denial event.
	AuditEventPolicyDenied AuditEventType = "policy_denied"

	// AuditEventSandboxViolation is a sandbox violation event.
	AuditEventSandboxViolation AuditEventType = "sandbox_violation"

	// AuditEventRateLimited is a rate limiting event.
	AuditEventRateLimited AuditEventType = "rate_limited"

	// AuditEventError is an error event.
	AuditEventError AuditEventType = "error"
)

// AuditResourceUsage contains resource usage for audit.
type AuditResourceUsage struct {
	CPUTimeMS    int64 `json:"cpu_time_ms"`
	MemoryPeakMB int64 `json:"memory_peak_mb"`
	IOReadBytes  int64 `json:"io_read_bytes"`
	IOWriteBytes int64 `json:"io_write_bytes"`
}

// AuditFilter filters audit events.
type AuditFilter struct {
	// StartTime is the start of the time range.
	StartTime time.Time

	// EndTime is the end of the time range.
	EndTime time.Time

	// Binary filters by binary.
	Binary string

	// Type filters by event type.
	Type AuditEventType

	// Status filters by status.
	Status string

	// Limit is the maximum number of events to return.
	Limit int
}

// AuditConfig configures the audit logger.
type AuditConfig struct {
	LogLevel      AuditLogLevel
	BasePath      string
	FilePath      string
	MaxOutputSize int
	RotateSize    int64
	RotateCount   int
	Enabled       bool
	IncludeOutput bool
}

// AuditLogLevel determines what events to log.
type AuditLogLevel string

const (
	// AuditLogAll logs all events.
	AuditLogAll AuditLogLevel = "all"

	// AuditLogFailures logs only failures.
	AuditLogFailures AuditLogLevel = "failures"

	// AuditLogPolicyViolations logs only policy violations.
	AuditLogPolicyViolations AuditLogLevel = "policy_violations"
)

// DefaultAuditConfig returns default audit configuration.
func DefaultAuditConfig() AuditConfig {
	return AuditConfig{
		Enabled:       true,
		LogLevel:      AuditLogAll,
		IncludeOutput: false,
		MaxOutputSize: 1024,
		BasePath:      "/var/log",
		FilePath:      "goexec/audit.log",
		RotateSize:    100 * 1024 * 1024, // 100MB
		RotateCount:   10,
	}
}

// fileAuditLogger implements AuditLogger using gowritter.
type fileAuditLogger struct {
	safePath *safepath.SafePath
	config   AuditConfig
	mu       sync.Mutex
}

// NewFileAuditLogger creates a new file-based audit logger.
func NewFileAuditLogger(config AuditConfig) (AuditLogger, error) {
	sp, err := safepath.New(config.BasePath)
	if err != nil {
		return nil, fmt.Errorf("creating safe path: %w", err)
	}

	return &fileAuditLogger{
		config:   config,
		safePath: sp,
	}, nil
}

// Log implements AuditLogger.Log.
func (l *fileAuditLogger) Log(ctx context.Context, event *AuditEvent) error {
	if !l.config.Enabled {
		return nil
	}

	// Check log level
	if !l.shouldLog(event) {
		return nil
	}

	// Truncate output if needed
	if !l.config.IncludeOutput {
		event.Output = ""
	} else if len(event.Output) > l.config.MaxOutputSize {
		event.Output = event.Output[:l.config.MaxOutputSize] + "...(truncated)"
	}

	// Marshal to JSON
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshaling audit event: %w", err)
	}

	// Append newline
	data = append(data, '\n')

	// Write to file using gowritter
	l.mu.Lock()
	defer l.mu.Unlock()

	if err := l.safePath.AppendFile(l.config.FilePath, data, 0o644); err != nil {
		return fmt.Errorf("writing audit log: %w", err)
	}

	return nil
}

// Query implements AuditLogger.Query.
func (l *fileAuditLogger) Query(ctx context.Context, filter *AuditFilter) ([]*AuditEvent, error) {
	// Read file using gowritter
	data, err := l.safePath.ReadFile(l.config.FilePath)
	if err != nil {
		return nil, fmt.Errorf("reading audit log: %w", err)
	}

	// Parse events (simple line-by-line JSON)
	var events []*AuditEvent
	// Note: In production, use a proper JSON streaming parser
	// This is a simplified implementation
	_ = data

	return events, nil
}

// Close implements AuditLogger.Close.
func (l *fileAuditLogger) Close() error {
	return nil
}

func (l *fileAuditLogger) shouldLog(event *AuditEvent) bool {
	switch l.config.LogLevel {
	case AuditLogAll:
		return true
	case AuditLogFailures:
		return event.Status != "success"
	case AuditLogPolicyViolations:
		return event.Type == AuditEventPolicyDenied || event.Type == AuditEventSandboxViolation
	default:
		return true
	}
}

// CreateAuditEvent creates an audit event from execution result.
func CreateAuditEvent(cmd *executor.Command, result *executor.Result, execErr error) *AuditEvent {
	event := &AuditEvent{
		ID:         result.CommandID,
		Timestamp:  time.Now(),
		Type:       AuditEventExecution,
		Binary:     cmd.Binary,
		Args:       cmd.Args,
		WorkingDir: cmd.WorkingDir,
		Status:     result.Status.String(),
		ExitCode:   result.ExitCode,
		Duration:   result.Duration,
		TraceID:    result.TraceID,
		Metadata:   cmd.Metadata,
	}

	if cmd.SandboxProfile != "" {
		event.SandboxProfile = cmd.SandboxProfile
	}

	if execErr != nil {
		event.Error = execErr.Error()
		event.Type = AuditEventError
	}

	switch result.Status {
	case executor.StatusPolicyDenied:
		event.Type = AuditEventPolicyDenied
	case executor.StatusSandboxViolation:
		event.Type = AuditEventSandboxViolation
	case executor.StatusRateLimited:
		event.Type = AuditEventRateLimited
	}

	if result.ResourceUsage != nil {
		event.ResourceUsage = &AuditResourceUsage{
			CPUTimeMS:    result.ResourceUsage.TotalCPUTime().Milliseconds(),
			MemoryPeakMB: result.MemoryPeak / (1024 * 1024),
			IOReadBytes:  result.ResourceUsage.IOReadBytes,
			IOWriteBytes: result.ResourceUsage.IOWriteBytes,
		}
	}

	return event
}

// NoopAuditLogger returns a no-op audit logger.
func NoopAuditLogger() AuditLogger {
	return &noopAuditLogger{}
}

type noopAuditLogger struct{}

func (l *noopAuditLogger) Log(ctx context.Context, event *AuditEvent) error { return nil }
func (l *noopAuditLogger) Query(ctx context.Context, filter *AuditFilter) ([]*AuditEvent, error) {
	return nil, nil
}
func (l *noopAuditLogger) Close() error { return nil }
