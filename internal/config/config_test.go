package config

import (
	"runtime"
	"strings"
	"testing"
)

func TestDefaultName(t *testing.T) {
	if DefaultName != "zushi.config.json" {
		t.Errorf("unexpected DefaultName: %s", DefaultName)
	}
}

func TestDefaultCompose(t *testing.T) {
	if DefaultCompose != "docker-compose.yml" {
		t.Errorf("unexpected DefaultCompose: %s", DefaultCompose)
	}
}

func TestDefaultDatadirNotEmpty(t *testing.T) {
	if DefaultDatadir == "" {
		t.Fatal("DefaultDatadir should not be empty")
	}
}

func TestDefaultDatadirContainsAppName(t *testing.T) {
	if !strings.Contains(DefaultDatadir, "zushi") {
		t.Errorf("DefaultDatadir should contain 'zushi', got: %s", DefaultDatadir)
	}
}

func TestDefaultPathEndsWithConfigFile(t *testing.T) {
	if !strings.HasSuffix(DefaultPath, DefaultName) {
		t.Errorf("DefaultPath should end with %s, got: %s", DefaultName, DefaultPath)
	}
}

func TestInitialState(t *testing.T) {
	if InitialState["network"] != "regtest" {
		t.Errorf("expected network=regtest, got %s", InitialState["network"])
	}
	if InitialState["ready"] != "false" {
		t.Errorf("expected ready=false, got %s", InitialState["ready"])
	}
	if InitialState["running"] != "false" {
		t.Errorf("expected running=false, got %s", InitialState["running"])
	}
}

func TestAppDataDirPlatform(t *testing.T) {
	dir := appDataDir("testapp")

	switch runtime.GOOS {
	case "darwin":
		if !strings.Contains(dir, "Library/Application Support/testapp") {
			t.Errorf("on darwin expected Library/Application Support path, got: %s", dir)
		}
	case "linux":
		if !strings.HasSuffix(dir, "/.testapp") {
			t.Errorf("on linux expected ~/.testapp path, got: %s", dir)
		}
	}
}
