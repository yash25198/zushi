package state

import (
	"os"
	"path/filepath"
	"testing"
)

func tempStatePath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "test-state.json")
}

func TestNew(t *testing.T) {
	path := "/tmp/test/state.json"
	initial := map[string]string{"key": "value"}
	s := New(path, initial)

	if s.FilePath() != path {
		t.Errorf("expected FilePath %s, got %s", path, s.FilePath())
	}
}

func TestSetAndGet(t *testing.T) {
	path := tempStatePath(t)
	initial := map[string]string{"network": "regtest"}
	s := New(path, initial)

	// First Get should create the file with initial state
	data, err := s.Get()
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if data["network"] != "regtest" {
		t.Errorf("expected network=regtest, got %s", data["network"])
	}

	// File should exist now
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("state file was not created")
	}

	// Set additional data
	err = s.Set(map[string]string{"running": "true"})
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Get should return merged data
	data, err = s.Get()
	if err != nil {
		t.Fatalf("Get after Set failed: %v", err)
	}
	if data["network"] != "regtest" {
		t.Errorf("expected network=regtest after merge, got %s", data["network"])
	}
	if data["running"] != "true" {
		t.Errorf("expected running=true, got %s", data["running"])
	}
}

func TestSetOverwritesExistingKey(t *testing.T) {
	path := tempStatePath(t)
	initial := map[string]string{"running": "false"}
	s := New(path, initial)

	_, _ = s.Get() // initialize

	err := s.Set(map[string]string{"running": "true"})
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	data, err := s.Get()
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if data["running"] != "true" {
		t.Errorf("expected running=true, got %s", data["running"])
	}
}

func TestGetBool(t *testing.T) {
	path := tempStatePath(t)
	initial := map[string]string{"ready": "false", "running": "true"}
	s := New(path, initial)

	val, err := s.GetBool("ready")
	if err != nil {
		t.Fatalf("GetBool failed: %v", err)
	}
	if val != false {
		t.Errorf("expected false, got %v", val)
	}

	val, err = s.GetBool("running")
	if err != nil {
		t.Fatalf("GetBool failed: %v", err)
	}
	if val != true {
		t.Errorf("expected true, got %v", val)
	}
}

func TestGetBoolMissingKey(t *testing.T) {
	path := tempStatePath(t)
	initial := map[string]string{"ready": "false"}
	s := New(path, initial)

	_, err := s.GetBool("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing key, got nil")
	}
}

func TestGetString(t *testing.T) {
	path := tempStatePath(t)
	initial := map[string]string{"network": "regtest"}
	s := New(path, initial)

	val, err := s.GetString("network")
	if err != nil {
		t.Fatalf("GetString failed: %v", err)
	}
	if val != "regtest" {
		t.Errorf("expected regtest, got %s", val)
	}
}

func TestGetStringMissingKey(t *testing.T) {
	path := tempStatePath(t)
	initial := map[string]string{"network": "regtest"}
	s := New(path, initial)

	_, err := s.GetString("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing key, got nil")
	}
}

func TestSetCreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "sub", "dir")
	path := filepath.Join(nested, "state.json")
	s := New(path, map[string]string{})

	err := s.Set(map[string]string{"key": "val"})
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	if _, err := os.Stat(nested); os.IsNotExist(err) {
		t.Fatal("nested directory was not created")
	}
}

func TestMultipleSetsMerge(t *testing.T) {
	path := tempStatePath(t)
	s := New(path, map[string]string{})

	s.Set(map[string]string{"a": "1"})
	s.Set(map[string]string{"b": "2"})
	s.Set(map[string]string{"c": "3"})

	data, err := s.Get()
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if data["a"] != "1" || data["b"] != "2" || data["c"] != "3" {
		t.Errorf("expected all three keys, got %v", data)
	}
}
