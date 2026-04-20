package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const minimalConfig = `
[wireguard]
private_key = "priv"
server_public_key = "pub"
server_endpoint = "vpn.example.com:51820"
address = "10.42.0.2"
dns = ["10.42.0.1"]
`

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "vanguard.toml")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

func TestLoadConfig_Defaults(t *testing.T) {
	cfg, err := LoadConfig(writeConfig(t, minimalConfig))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Metrics.Listen != ":9090" {
		t.Errorf("listen default = %q", cfg.Metrics.Listen)
	}
	if cfg.Metrics.Path != "/metrics" {
		t.Errorf("path default = %q", cfg.Metrics.Path)
	}
	if cfg.Probe.Interval.D() != 30*time.Second {
		t.Errorf("interval default = %v", cfg.Probe.Interval.D())
	}
	if cfg.Probe.Timeout.D() != 10*time.Second {
		t.Errorf("timeout default = %v", cfg.Probe.Timeout.D())
	}
	if cfg.WireGuard.MTU != 1420 {
		t.Errorf("mtu default = %d", cfg.WireGuard.MTU)
	}
	if cfg.WireGuard.Keepalive != 25 {
		t.Errorf("keepalive default = %d", cfg.WireGuard.Keepalive)
	}
}

func TestLoadConfig_ValidationErrors(t *testing.T) {
	cases := []struct {
		name, body, substr string
	}{
		{
			name: "missing private_key",
			body: `[wireguard]
server_public_key = "p"
server_endpoint = "e:1"
address = "10.0.0.1"`,
			substr: "private_key",
		},
		{
			name: "invalid address",
			body: `[wireguard]
private_key = "a"
server_public_key = "b"
server_endpoint = "c:1"
address = "not-an-ip"`,
			substr: "address",
		},
		{
			name: "invalid dns",
			body: `[wireguard]
private_key = "a"
server_public_key = "b"
server_endpoint = "c:1"
address = "10.0.0.1"
dns = ["also-not-an-ip"]`,
			substr: "dns",
		},
		{
			name: "duplicate target name",
			body: minimalConfig + `
[[website]]
name = "dup"
url = "http://example.com"

[[tcp_service]]
name = "dup"
host = "1.2.3.4"
port = 22`,
			substr: "duplicate",
		},
		{
			name: "unknown tcp type",
			body: minimalConfig + `
[[tcp_service]]
name = "a"
host = "h"
port = 1
type = "gopher"`,
			substr: "unknown type",
		},
		{
			name: "website without url",
			body: minimalConfig + `
[[website]]
name = "nourl"`,
			substr: "url is required",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadConfig(writeConfig(t, tc.body))
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.substr)
			}
			if !strings.Contains(err.Error(), tc.substr) {
				t.Errorf("error %v did not contain %q", err, tc.substr)
			}
		})
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	if _, err := LoadConfig("/does/not/exist.toml"); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestDurationUnmarshal(t *testing.T) {
	var d Duration
	if err := d.UnmarshalText([]byte("250ms")); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if d.D() != 250*time.Millisecond {
		t.Errorf("d = %v", d.D())
	}
	if err := d.UnmarshalText([]byte("bogus")); err == nil {
		t.Error("expected error for bogus duration")
	}
}
