// Package envutil provides environment variable utilities.
package envutil

// MinimalEnvironment returns a minimal safe environment.
func MinimalEnvironment() map[string]string {
	return map[string]string{
		"PATH":   "/usr/bin:/bin",
		"LANG":   "C.UTF-8",
		"LC_ALL": "C.UTF-8",
		"HOME":   "/tmp",
		"USER":   "nobody",
	}
}

// MergeEnvironment merges base environment with overrides.
// Overrides take precedence.
func MergeEnvironment(base, override map[string]string) map[string]string {
	result := make(map[string]string, len(base)+len(override))

	for k, v := range base {
		result[k] = v
	}

	for k, v := range override {
		result[k] = v
	}

	return result
}
