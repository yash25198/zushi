package docker

import (
	"os"
	"path/filepath"
	"testing"
)

const testComposeYML = `name: test
services:
  zcashd:
    image: electriccoinco/zcashd:latest
    ports:
      - 18232:18232
      - 18233:18233
  lightwalletd:
    image: electriccoinco/lightwalletd:latest
    ports:
      - 9067:9067
  zcash-faucet:
    image: golang:1.24-alpine
    ports:
      - 3000:3000
  no-ports:
    image: busybox
`

func writeTestCompose(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(path, []byte(testComposeYML), 0644); err != nil {
		t.Fatalf("failed to write test compose: %v", err)
	}
	return path
}

func TestGetEndpoints(t *testing.T) {
	path := writeTestCompose(t)
	client := NewDefaultClient()

	endpoints, err := client.GetEndpoints(path)
	if err != nil {
		t.Fatalf("GetEndpoints failed: %v", err)
	}

	expected := map[string]string{
		"zcashd":       "localhost:18232",
		"lightwalletd": "localhost:9067",
		"zcash-faucet": "localhost:3000",
	}

	for name, want := range expected {
		got, ok := endpoints[name]
		if !ok {
			t.Errorf("missing endpoint for service %s", name)
			continue
		}
		if got != want {
			t.Errorf("endpoint %s: expected %s, got %s", name, want, got)
		}
	}

	// no-ports service should not have an endpoint
	if _, ok := endpoints["no-ports"]; ok {
		t.Error("no-ports service should not have an endpoint")
	}
}

func TestGetPortsForService(t *testing.T) {
	path := writeTestCompose(t)
	client := NewDefaultClient()

	tests := []struct {
		service string
		want    []string
	}{
		{"zcashd", []string{"18232", "18233"}},
		{"lightwalletd", []string{"9067"}},
		{"zcash-faucet", []string{"3000"}},
	}

	for _, tt := range tests {
		ports, err := client.GetPortsForService(path, tt.service)
		if err != nil {
			t.Errorf("GetPortsForService(%s) failed: %v", tt.service, err)
			continue
		}
		if len(ports) != len(tt.want) {
			t.Errorf("GetPortsForService(%s): expected %d ports, got %d", tt.service, len(tt.want), len(ports))
			continue
		}
		for i, w := range tt.want {
			if ports[i] != w {
				t.Errorf("GetPortsForService(%s)[%d]: expected %s, got %s", tt.service, i, w, ports[i])
			}
		}
	}
}

func TestGetPortsForServiceNotFound(t *testing.T) {
	path := writeTestCompose(t)
	client := NewDefaultClient()

	_, err := client.GetPortsForService(path, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent service, got nil")
	}
}

func TestGetEndpointsMissingFile(t *testing.T) {
	client := NewDefaultClient()

	_, err := client.GetEndpoints("/tmp/nonexistent-compose.yml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestGetPortsForServiceMissingFile(t *testing.T) {
	client := NewDefaultClient()

	_, err := client.GetPortsForService("/tmp/nonexistent-compose.yml", "zcashd")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestGetPortsForServiceNoPorts(t *testing.T) {
	path := writeTestCompose(t)
	client := NewDefaultClient()

	ports, err := client.GetPortsForService(path, "no-ports")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ports) != 0 {
		t.Errorf("expected 0 ports for no-ports service, got %d", len(ports))
	}
}
