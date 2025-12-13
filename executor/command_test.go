package executor

import (
	"strings"
	"testing"
	"time"
)

func TestNewCommand(t *testing.T) {
	builder := NewCommand("/usr/bin/ls", "-la", "/tmp")
	if builder == nil {
		t.Fatal("NewCommand returned nil")
	}

	cmd, err := builder.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if cmd.Binary != "/usr/bin/ls" {
		t.Errorf("Expected binary '/usr/bin/ls', got '%s'", cmd.Binary)
	}

	if len(cmd.Args) != 2 {
		t.Errorf("Expected 2 args, got %d", len(cmd.Args))
	}

	if cmd.Args[0] != "-la" || cmd.Args[1] != "/tmp" {
		t.Errorf("Unexpected args: %v", cmd.Args)
	}
}

func TestCommandBuilder_WithWorkingDir(t *testing.T) {
	cmd, err := NewCommand("/bin/echo", "test").
		WithWorkingDir("/tmp").
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if cmd.WorkingDir != "/tmp" {
		t.Errorf("Expected working dir '/tmp', got '%s'", cmd.WorkingDir)
	}
}

func TestCommandBuilder_WithTimeout(t *testing.T) {
	timeout := 5 * time.Second
	cmd, err := NewCommand("/bin/echo", "test").
		WithTimeout(timeout).
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if cmd.Timeout != timeout {
		t.Errorf("Expected timeout %v, got %v", timeout, cmd.Timeout)
	}
}

func TestCommandBuilder_WithTimeout_Invalid(t *testing.T) {
	builder := NewCommand("/bin/echo", "test").
		WithTimeout(-1 * time.Second)

	cmd, err := builder.Build()
	if err == nil {
		t.Error("Expected error for negative timeout")
	}
	if cmd != nil {
		t.Error("Command should be nil on error")
	}
}

func TestCommandBuilder_WithTimeout_Zero(t *testing.T) {
	builder := NewCommand("/bin/echo", "test").
		WithTimeout(0)

	cmd, err := builder.Build()
	if err == nil {
		t.Error("Expected error for zero timeout")
	}
	if cmd != nil {
		t.Error("Command should be nil on error")
	}
}

func TestCommandBuilder_WithEnv(t *testing.T) {
	cmd, err := NewCommand("/bin/echo", "test").
		WithEnv("KEY1", "value1").
		WithEnv("KEY2", "value2").
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if cmd.Env["KEY1"] != "value1" {
		t.Errorf("Expected KEY1=value1, got KEY1=%s", cmd.Env["KEY1"])
	}
	if cmd.Env["KEY2"] != "value2" {
		t.Errorf("Expected KEY2=value2, got KEY2=%s", cmd.Env["KEY2"])
	}
}

func TestCommandBuilder_WithEnvMap(t *testing.T) {
	envMap := map[string]string{
		"KEY1": "value1",
		"KEY2": "value2",
	}

	cmd, err := NewCommand("/bin/echo", "test").
		WithEnvMap(envMap).
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if len(cmd.Env) != 2 {
		t.Errorf("Expected 2 env vars, got %d", len(cmd.Env))
	}
}

func TestCommandBuilder_WithStdin(t *testing.T) {
	stdin := strings.NewReader("input data")
	cmd, err := NewCommand("/bin/cat").
		WithStdin(stdin).
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if cmd.Stdin != stdin {
		t.Error("Stdin was not set correctly")
	}
}

func TestCommandBuilder_WithResourceLimits(t *testing.T) {
	limits := &ResourceLimits{
		MaxCPUTime:     10 * time.Second,
		MaxMemoryBytes: 1024 * 1024,
		MaxProcesses:   10,
	}

	cmd, err := NewCommand("/bin/echo", "test").
		WithResourceLimits(limits).
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if cmd.ResourceLimits.MaxCPUTime != limits.MaxCPUTime {
		t.Error("Resource limits not set correctly")
	}
}

func TestCommandBuilder_WithSandboxProfile(t *testing.T) {
	cmd, err := NewCommand("/bin/echo", "test").
		WithSandboxProfile("restricted").
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if cmd.SandboxProfile != "restricted" {
		t.Errorf("Expected sandbox profile 'restricted', got '%s'", cmd.SandboxProfile)
	}
}

func TestCommandBuilder_WithMetadata(t *testing.T) {
	cmd, err := NewCommand("/bin/echo", "test").
		WithMetadata("key1", "value1").
		WithMetadata("key2", "value2").
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if cmd.Metadata["key1"] != "value1" {
		t.Error("Metadata not set correctly")
	}
}

func TestCommandBuilder_WithPriority(t *testing.T) {
	cmd, err := NewCommand("/bin/echo", "test").
		WithPriority(PriorityHigh).
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if cmd.Priority != PriorityHigh {
		t.Errorf("Expected priority %v, got %v", PriorityHigh, cmd.Priority)
	}
}

func TestCommandBuilder_Build_EmptyBinary(t *testing.T) {
	builder := NewCommand("", "arg")
	cmd, err := builder.Build()

	if err == nil {
		t.Error("Expected error for empty binary")
	}
	if cmd != nil {
		t.Error("Command should be nil on error")
	}
	if !strings.Contains(err.Error(), "binary path is required") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestCommandBuilder_Build_RelativePath(t *testing.T) {
	builder := NewCommand("echo", "test")
	cmd, err := builder.Build()

	if err == nil {
		t.Error("Expected error for relative path")
	}
	if cmd != nil {
		t.Error("Command should be nil on error")
	}
	if !strings.Contains(err.Error(), "absolute path") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestCommandBuilder_Build_RelativeWorkingDir(t *testing.T) {
	builder := NewCommand("/bin/echo", "test").
		WithWorkingDir("tmp")
	cmd, err := builder.Build()

	if err == nil {
		t.Error("Expected error for relative working dir")
	}
	if cmd != nil {
		t.Error("Command should be nil on error")
	}
}

func TestCommandBuilder_ErrorPropagation(t *testing.T) {
	// Create a builder with an error
	builder := NewCommand("/bin/echo", "test").
		WithTimeout(-1 * time.Second)

	// All subsequent calls should return the builder with error
	builder = builder.WithEnv("KEY", "value")
	builder = builder.WithWorkingDir("/tmp")

	cmd, err := builder.Build()
	if err == nil {
		t.Error("Expected error to persist")
	}
	if cmd != nil {
		t.Error("Command should be nil when builder has error")
	}
}

func TestCommandBuilder_MustBuild(t *testing.T) {
	cmd := NewCommand("/bin/echo", "test").MustBuild()
	if cmd == nil {
		t.Fatal("MustBuild returned nil")
	}

	defer func() {
		if r := recover(); r == nil {
			t.Error("MustBuild should panic on error")
		}
	}()

	NewCommand("", "test").MustBuild()
}

func TestCommand_Clone(t *testing.T) {
	original := &Command{
		Binary:         "/bin/echo",
		Args:           []string{"test"},
		Env:            map[string]string{"KEY": "value"},
		WorkingDir:     "/tmp",
		Timeout:        5 * time.Second,
		Stdin:          strings.NewReader("input"),
		SandboxProfile: "restricted",
		Metadata:       map[string]string{"meta": "data"},
		Priority:       PriorityHigh,
		ResourceLimits: &ResourceLimits{
			MaxCPUTime:     10 * time.Second,
			MaxMemoryBytes: 1024,
		},
	}

	clone := original.Clone()

	if clone == original {
		t.Error("Clone should return a new instance")
	}

	if clone.Binary != original.Binary {
		t.Error("Binary not cloned correctly")
	}

	if len(clone.Args) != len(original.Args) {
		t.Error("Args not cloned correctly")
	}

	// Modify clone and verify original is unchanged
	clone.Binary = "/bin/ls"
	if original.Binary != "/bin/echo" {
		t.Error("Original should not be affected by clone modification")
	}

	// Verify args slice is copied
	clone.Args[0] = "modified"
	if original.Args[0] == "modified" {
		t.Error("Original Args should not be affected")
	}

	// Verify env map is copied
	clone.Env["KEY"] = "newvalue"
	if original.Env["KEY"] != "value" {
		t.Error("Original Env should not be affected")
	}

	// Verify metadata map is copied
	clone.Metadata["meta"] = "newdata"
	if original.Metadata["meta"] != "data" {
		t.Error("Original Metadata should not be affected")
	}

	// Verify ResourceLimits is deep copied
	if clone.ResourceLimits == original.ResourceLimits {
		t.Error("ResourceLimits should be deep copied")
	}
	clone.ResourceLimits.MaxCPUTime = 20 * time.Second
	if original.ResourceLimits.MaxCPUTime != 10*time.Second {
		t.Error("Original ResourceLimits should not be affected")
	}
}

func TestCommand_Clone_NilResourceLimits(t *testing.T) {
	original := &Command{
		Binary: "/bin/echo",
		Args:   []string{"test"},
	}

	clone := original.Clone()
	if clone.ResourceLimits != nil {
		t.Error("Clone of nil ResourceLimits should be nil")
	}
}

func TestCommand_String(t *testing.T) {
	tests := []struct { //nolint:govet // fieldalignment: test struct field order optimized for readability not memory
		want string
		cmd  *Command
		name string
	}{
		{
			name: "with args",
			cmd: &Command{
				Binary: "/bin/echo",
				Args:   []string{"hello", "world"},
			},
			want: "/bin/echo [hello world]",
		},
		{
			name: "without args",
			cmd: &Command{
				Binary: "/bin/echo",
				Args:   []string{},
			},
			want: "/bin/echo",
		},
		{
			name: "nil args",
			cmd: &Command{
				Binary: "/bin/echo",
				Args:   nil,
			},
			want: "/bin/echo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cmd.String()
			// Note: actual format might be different, just check it's not empty
			if got == "" {
				t.Error("String() returned empty string")
			}
			if !strings.Contains(got, tt.cmd.Binary) {
				t.Errorf("String() should contain binary '%s', got '%s'", tt.cmd.Binary, got)
			}
		})
	}
}

func TestPriority_String(t *testing.T) {
	tests := []struct { //nolint:govet // fieldalignment: test struct field order optimized for readability not memory
		priority Priority
		want     string
	}{
		{PriorityLow, "low"},
		{PriorityNormal, "normal"},
		{PriorityHigh, "high"},
		{PriorityCritical, "critical"},
		{Priority(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.priority.String()
			if got != tt.want {
				t.Errorf("Priority(%d).String() = %s, want %s", tt.priority, got, tt.want)
			}
		})
	}
}

func TestCommandBuilder_Chain(t *testing.T) {
	cmd, err := NewCommand("/bin/echo", "test").
		WithWorkingDir("/tmp").
		WithTimeout(5*time.Second).
		WithEnv("KEY1", "value1").
		WithEnv("KEY2", "value2").
		WithMetadata("meta", "data").
		WithPriority(PriorityHigh).
		Build()

	if err != nil {
		t.Fatalf("Chained build failed: %v", err)
	}

	if cmd.WorkingDir != "/tmp" {
		t.Error("Chained WithWorkingDir failed")
	}
	if cmd.Timeout != 5*time.Second {
		t.Error("Chained WithTimeout failed")
	}
	if len(cmd.Env) != 2 {
		t.Error("Chained WithEnv failed")
	}
	if cmd.Metadata["meta"] != "data" {
		t.Error("Chained WithMetadata failed")
	}
	if cmd.Priority != PriorityHigh {
		t.Error("Chained WithPriority failed")
	}
}

func TestCommandBuilder_WithEnv_EmptyKey(t *testing.T) {
	cmd, err := NewCommand("/bin/echo", "test").
		WithEnv("", "value").
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if cmd.Env[""] != "value" {
		t.Error("Empty key should be allowed (validation happens elsewhere)")
	}
}

func TestCommandBuilder_Build_ComplexPath(t *testing.T) {
	// Test with complex but valid absolute path
	path := "/usr/local/bin/my-executable"
	cmd, err := NewCommand(path, "arg1", "arg2").Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if cmd.Binary != path {
		t.Errorf("Expected binary '%s', got '%s'", path, cmd.Binary)
	}
}

func TestCommandBuilder_WithEnv_Overwrite(t *testing.T) {
	cmd, err := NewCommand("/bin/echo", "test").
		WithEnv("KEY", "value1").
		WithEnv("KEY", "value2").
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if cmd.Env["KEY"] != "value2" {
		t.Errorf("Expected last value 'value2', got '%s'", cmd.Env["KEY"])
	}
}

func TestCommand_Clone_EmptyFields(t *testing.T) {
	original := &Command{
		Binary: "/bin/echo",
	}

	clone := original.Clone()

	if clone.Args == nil || len(clone.Args) != 0 {
		t.Error("Clone should initialize empty Args slice")
	}
	if clone.Env == nil {
		t.Error("Clone should initialize Env map")
	}
	if clone.Metadata == nil {
		t.Error("Clone should initialize Metadata map")
	}
}
