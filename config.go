package main

import (
	"fmt"
	"net/netip"
	"os"
	"time"

	"github.com/BurntSushi/toml"
)

type Config struct {
	WireGuard WireGuardConfig `toml:"wireguard"`
	Metrics   MetricsConfig   `toml:"metrics"`
	Probe     ProbeConfig     `toml:"probe"`
	Websites  []WebsiteTarget `toml:"website"`
	TCP       []TCPTarget     `toml:"tcp_service"`
}

type WireGuardConfig struct {
	PrivateKey      string   `toml:"private_key"`
	ServerPublicKey string   `toml:"server_public_key"`
	ServerEndpoint  string   `toml:"server_endpoint"`
	Address         string   `toml:"address"`
	DNS             []string `toml:"dns"`
	MTU             int      `toml:"mtu"`
	Keepalive       int      `toml:"persistent_keepalive_interval"`
}

type MetricsConfig struct {
	Listen string `toml:"listen"`
	Path   string `toml:"path"`
}

type ProbeConfig struct {
	Interval Duration `toml:"interval"`
	Timeout  Duration `toml:"timeout"`
}

type WebsiteTarget struct {
	Name    string   `toml:"name"`
	URL     string   `toml:"url"`
	Phrases []string `toml:"phrases"`
	Timeout Duration `toml:"timeout"`
}

type TCPTarget struct {
	Name       string   `toml:"name"`
	Host       string   `toml:"host"`
	Port       int      `toml:"port"`
	Type       string   `toml:"type"`
	ServerName string   `toml:"server_name"`
	Timeout    Duration `toml:"timeout"`
}

type Duration time.Duration

func (d *Duration) UnmarshalText(text []byte) error {
	v, err := time.ParseDuration(string(text))
	if err != nil {
		return err
	}
	*d = Duration(v)
	return nil
}

func (d Duration) D() time.Duration { return time.Duration(d) }

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	c := &Config{
		Metrics: MetricsConfig{Listen: ":9090", Path: "/metrics"},
		Probe:   ProbeConfig{Interval: Duration(30 * time.Second), Timeout: Duration(10 * time.Second)},
		WireGuard: WireGuardConfig{
			MTU:       1420,
			Keepalive: 25,
		},
	}

	if err := toml.Unmarshal(data, c); err != nil {
		return nil, err
	}

	if err := c.validate(); err != nil {
		return nil, err
	}

	return c, nil
}

func (c *Config) validate() error {
	if c.WireGuard.PrivateKey == "" {
		return fmt.Errorf("wireguard.private_key is required")
	}
	if c.WireGuard.ServerPublicKey == "" {
		return fmt.Errorf("wireguard.server_public_key is required")
	}
	if c.WireGuard.ServerEndpoint == "" {
		return fmt.Errorf("wireguard.server_endpoint is required")
	}
	if c.WireGuard.Address == "" {
		return fmt.Errorf("wireguard.address is required")
	}
	if _, err := netip.ParseAddr(c.WireGuard.Address); err != nil {
		return fmt.Errorf("wireguard.address: %w", err)
	}
	for _, dns := range c.WireGuard.DNS {
		if _, err := netip.ParseAddr(dns); err != nil {
			return fmt.Errorf("wireguard.dns %q: %w", dns, err)
		}
	}
	seen := map[string]bool{}
	for i, w := range c.Websites {
		if w.Name == "" {
			return fmt.Errorf("website[%d]: name is required", i)
		}
		if seen[w.Name] {
			return fmt.Errorf("duplicate target name: %s", w.Name)
		}
		seen[w.Name] = true
		if w.URL == "" {
			return fmt.Errorf("website %q: url is required", w.Name)
		}
	}
	for i, t := range c.TCP {
		if t.Name == "" {
			return fmt.Errorf("tcp_service[%d]: name is required", i)
		}
		if seen[t.Name] {
			return fmt.Errorf("duplicate target name: %s", t.Name)
		}
		seen[t.Name] = true
		if t.Host == "" || t.Port == 0 {
			return fmt.Errorf("tcp_service %q: host and port are required", t.Name)
		}
		switch t.Type {
		case "", "tcp", "ssh", "smtp_starttls":
		default:
			return fmt.Errorf("tcp_service %q: unknown type %q", t.Name, t.Type)
		}
	}
	return nil
}
