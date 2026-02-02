// Package testutil provides shared utilities for testing.
package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

// LoadFixture loads a fixture file from the fixtures directory.
func LoadFixture(t *testing.T, path string) []byte {
	t.Helper()
	fullPath := filepath.Join("testdata", path)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("failed to load fixture %s: %v", path, err)
	}
	return data
}

// LoadFixtureString loads a fixture file as a string.
func LoadFixtureString(t *testing.T, path string) string {
	return string(LoadFixture(t, path))
}

// TempDir creates a temporary directory for the test.
func TempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "gateway-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(dir)
	})
	return dir
}

// WriteTemp writes content to a temporary file.
func WriteTemp(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	return path
}

// GoldenFile compares output against a golden file.
// If the GOLDEN_UPDATE env var is set, it updates the golden file.
func GoldenFile(t *testing.T, name string, actual []byte) {
	t.Helper()
	golden := filepath.Join("testdata", name+".golden")

	if os.Getenv("GOLDEN_UPDATE") != "" {
		if err := os.MkdirAll(filepath.Dir(golden), 0755); err != nil {
			t.Fatalf("failed to create golden dir: %v", err)
		}
		if err := os.WriteFile(golden, actual, 0644); err != nil {
			t.Fatalf("failed to write golden file: %v", err)
		}
		return
	}

	expected, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("failed to read golden file %s: %v (run with GOLDEN_UPDATE=1 to create)", golden, err)
	}

	if string(expected) != string(actual) {
		t.Errorf("output does not match golden file %s\nExpected:\n%s\n\nActual:\n%s", golden, expected, actual)
	}
}
