package envutil

import (
	"reflect"
	"testing"
)

func TestMinimalEnvironment(t *testing.T) {
	env := MinimalEnvironment()

	// Check that required keys exist
	requiredKeys := []string{"PATH", "LANG", "LC_ALL", "HOME", "USER"}
	for _, key := range requiredKeys {
		if _, exists := env[key]; !exists {
			t.Errorf("MinimalEnvironment() missing required key: %s", key)
		}
	}

	// Check specific values
	if env["PATH"] != "/usr/bin:/bin" {
		t.Errorf("Expected PATH='/usr/bin:/bin', got '%s'", env["PATH"])
	}

	if env["LANG"] != "C.UTF-8" {
		t.Errorf("Expected LANG='C.UTF-8', got '%s'", env["LANG"])
	}

	if env["LC_ALL"] != "C.UTF-8" {
		t.Errorf("Expected LC_ALL='C.UTF-8', got '%s'", env["LC_ALL"])
	}

	if env["HOME"] != "/tmp" {
		t.Errorf("Expected HOME='/tmp', got '%s'", env["HOME"])
	}

	if env["USER"] != "nobody" {
		t.Errorf("Expected USER='nobody', got '%s'", env["USER"])
	}

	// Check that no extra keys are present
	if len(env) != len(requiredKeys) {
		t.Errorf("Expected %d keys, got %d", len(requiredKeys), len(env))
	}
}

func TestMergeEnvironment(t *testing.T) {
	base := map[string]string{
		"PATH": "/usr/bin",
		"LANG": "en_US.UTF-8",
		"HOME": "/home/user",
	}

	override := map[string]string{
		"LANG": "C.UTF-8",
		"USER": "testuser",
	}

	result := MergeEnvironment(base, override)

	// Check that base values not in override are preserved
	if result["PATH"] != "/usr/bin" {
		t.Errorf("Expected PATH='/usr/bin', got '%s'", result["PATH"])
	}

	if result["HOME"] != "/home/user" {
		t.Errorf("Expected HOME='/home/user', got '%s'", result["HOME"])
	}

	// Check that override values take precedence
	if result["LANG"] != "C.UTF-8" {
		t.Errorf("Expected LANG='C.UTF-8' (from override), got '%s'", result["LANG"])
	}

	// Check that new keys from override are added
	if result["USER"] != "testuser" {
		t.Errorf("Expected USER='testuser', got '%s'", result["USER"])
	}

	// Check that result is a new map (not sharing references)
	if len(result) != 4 {
		t.Errorf("Expected 4 keys, got %d", len(result))
	}

	// Verify result is independent from base
	result["NEW_KEY"] = "value"
	if _, exists := base["NEW_KEY"]; exists {
		t.Error("Result map should be independent from base")
	}

	// Verify result is independent from override
	delete(result, "USER")
	if _, exists := override["USER"]; !exists {
		t.Error("Override map should not be modified")
	}
}

func TestMergeEnvironment_EmptyBase(t *testing.T) {
	override := map[string]string{
		"PATH": "/usr/bin",
		"LANG": "C.UTF-8",
	}

	result := MergeEnvironment(nil, override)

	if !reflect.DeepEqual(result, override) {
		t.Errorf("Expected result to equal override when base is nil, got %v", result)
	}
}

func TestMergeEnvironment_EmptyOverride(t *testing.T) {
	base := map[string]string{
		"PATH": "/usr/bin",
		"LANG": "C.UTF-8",
	}

	result := MergeEnvironment(base, nil)

	if !reflect.DeepEqual(result, base) {
		t.Errorf("Expected result to equal base when override is nil, got %v", result)
	}
}

func TestMergeEnvironment_BothEmpty(t *testing.T) {
	result := MergeEnvironment(nil, nil)

	if result == nil {
		t.Error("Expected non-nil empty map, got nil")
	}

	if len(result) != 0 {
		t.Errorf("Expected empty map, got %d keys", len(result))
	}
}
