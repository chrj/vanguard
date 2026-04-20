package main

import "testing"

func TestResolveListen(t *testing.T) {
	cases := []struct {
		name   string
		listen string
		addr   string
		want   string
	}{
		{"empty host filled from wireguard", ":9090", "10.42.0.2", "10.42.0.2:9090"},
		{"explicit host respected", "192.168.1.1:8080", "10.42.0.2", "192.168.1.1:8080"},
		{"unparseable falls back to address:9090", "not-an-addr", "10.42.0.2", "10.42.0.2:9090"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{
				Metrics:   MetricsConfig{Listen: tc.listen},
				WireGuard: WireGuardConfig{Address: tc.addr},
			}
			if got := resolveListen(cfg); got != tc.want {
				t.Errorf("resolveListen = %q, want %q", got, tc.want)
			}
		})
	}
}
