package main

import (
	"context"
	"flag"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"syscall"

	"github.com/chrj/wgnet"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	configPath := flag.String("config", "vanguard.toml", "path to TOML config file")
	flag.Parse()

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}

	dev, err := buildDevice(cfg)
	if err != nil {
		slog.Error("wireguard", "err", err)
		os.Exit(1)
	}
	defer dev.Close()

	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	m := NewMetrics(reg)

	prober := NewProber(cfg, dev.DialContext, m)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go prober.Run(ctx)

	if err := serve(ctx, cfg, dev, reg); err != nil {
		slog.Error("metrics server", "err", err)
		os.Exit(1)
	}
}

func buildDevice(cfg *Config) (*wgnet.Device, error) {
	wc := wgnet.NewDefaultConfiguration()
	wc.MyIPv4 = netip.MustParseAddr(cfg.WireGuard.Address)
	wc.PrivateKey = cfg.WireGuard.PrivateKey
	wc.ServerPublicKey = cfg.WireGuard.ServerPublicKey
	wc.ServerEndpoint = cfg.WireGuard.ServerEndpoint
	if cfg.WireGuard.MTU > 0 {
		wc.MTU = cfg.WireGuard.MTU
	}
	if cfg.WireGuard.Keepalive > 0 {
		wc.PersistentKeepaliveInterval = cfg.WireGuard.Keepalive
	}
	if len(cfg.WireGuard.DNS) > 0 {
		wc.DNS = wc.DNS[:0]
		for _, d := range cfg.WireGuard.DNS {
			wc.DNS = append(wc.DNS, netip.MustParseAddr(d))
		}
	}
	return wgnet.NewDevice(wc)
}

func serve(ctx context.Context, cfg *Config, dev *wgnet.Device, reg *prometheus.Registry) error {
	mux := http.NewServeMux()
	mux.Handle(cfg.Metrics.Path, promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	srv := &http.Server{Handler: mux}

	tcpAddr, err := net.ResolveTCPAddr("tcp", resolveListen(cfg))
	if err != nil {
		return err
	}
	ln, err := dev.ListenTCP(tcpAddr)
	if err != nil {
		return err
	}

	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()

	slog.Info("serving metrics", "addr", tcpAddr.String(), "path", cfg.Metrics.Path)
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func resolveListen(cfg *Config) string {
	addr := cfg.Metrics.Listen
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return net.JoinHostPort(cfg.WireGuard.Address, "9090")
	}
	if host == "" {
		host = cfg.WireGuard.Address
	}
	return net.JoinHostPort(host, port)
}
