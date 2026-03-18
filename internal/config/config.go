package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
)

var (
	DefaultName    = "zushi.config.json"
	DefaultCompose = "docker-compose.yml"

	DefaultDatadir = appDataDir("zushi")
	DefaultPath    = filepath.Join(DefaultDatadir, DefaultName)

	InitialState = map[string]string{
		"network": "regtest",
		"ready":   strconv.FormatBool(false),
		"running": strconv.FormatBool(false),
	}
)

// appDataDir returns the OS-appropriate application data directory.
func appDataDir(appName string) string {
	var homeDir string
	homeDir, _ = os.UserHomeDir()

	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(homeDir, "Library", "Application Support", appName)
	case "windows":
		appData := os.Getenv("LOCALAPPDATA")
		if appData != "" {
			return filepath.Join(appData, appName)
		}
		return filepath.Join(homeDir, appName)
	default: // linux, freebsd, etc.
		return filepath.Join(homeDir, "."+appName)
	}
}
