package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func newTestProber(t *testing.T) *Prober {
	t.Helper()
	cfg := &Config{
		Probe: ProbeConfig{
			Interval: Duration(time.Second),
			Timeout:  Duration(2 * time.Second),
		},
	}
	m := NewMetrics(prometheus.NewRegistry())
	return NewProber(cfg, m)
}

func splitHostPort(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("port atoi: %v", err)
	}
	return host, port
}

func TestProbeHTTP_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("hello vanguard world"))
	}))
	defer srv.Close()

	p := newTestProber(t)
	target := WebsiteTarget{Name: "t1", URL: srv.URL, Phrases: []string{"vanguard"}}
	p.probeHTTP(context.Background(), target)

	if got := testutil.ToFloat64(p.m.HTTPUp.WithLabelValues("t1", srv.URL)); got != 1 {
		t.Errorf("HTTPUp = %v, want 1", got)
	}
	if got := testutil.ToFloat64(p.m.HTTPStatus.WithLabelValues("t1", srv.URL)); got != 200 {
		t.Errorf("HTTPStatus = %v, want 200", got)
	}
	if got := testutil.ToFloat64(p.m.HTTPPhrase.WithLabelValues("t1", srv.URL, "vanguard")); got != 1 {
		t.Errorf("HTTPPhrase = %v, want 1", got)
	}
	if got := testutil.ToFloat64(p.m.HTTPContentBytes.WithLabelValues("t1", srv.URL)); got != 20 {
		t.Errorf("HTTPContentBytes = %v, want 20", got)
	}
}

func TestProbeHTTP_PhraseMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()

	p := newTestProber(t)
	target := WebsiteTarget{Name: "t1", URL: srv.URL, Phrases: []string{"absent"}}
	p.probeHTTP(context.Background(), target)

	if got := testutil.ToFloat64(p.m.HTTPUp.WithLabelValues("t1", srv.URL)); got != 0 {
		t.Errorf("HTTPUp = %v, want 0 (phrase missing)", got)
	}
	if got := testutil.ToFloat64(p.m.HTTPPhrase.WithLabelValues("t1", srv.URL, "absent")); got != 0 {
		t.Errorf("HTTPPhrase = %v, want 0", got)
	}
}

func TestProbeHTTP_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	p := newTestProber(t)
	target := WebsiteTarget{Name: "t1", URL: srv.URL}
	p.probeHTTP(context.Background(), target)

	if got := testutil.ToFloat64(p.m.HTTPUp.WithLabelValues("t1", srv.URL)); got != 0 {
		t.Errorf("HTTPUp = %v, want 0 on 500", got)
	}
	if got := testutil.ToFloat64(p.m.HTTPStatus.WithLabelValues("t1", srv.URL)); got != 500 {
		t.Errorf("HTTPStatus = %v, want 500", got)
	}
}

func TestProbeHTTP_DialError(t *testing.T) {
	p := newTestProber(t)
	target := WebsiteTarget{Name: "t1", URL: "http://127.0.0.1:1/"}
	p.probeHTTP(context.Background(), target)

	if got := testutil.ToFloat64(p.m.HTTPUp.WithLabelValues("t1", target.URL)); got != 0 {
		t.Errorf("HTTPUp = %v, want 0", got)
	}
	if got := testutil.ToFloat64(p.m.ProbeErrors.WithLabelValues("t1", "http")); got != 1 {
		t.Errorf("ProbeErrors = %v, want 1", got)
	}
}

func TestProbeTCP_Success(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()

	p := newTestProber(t)
	host, port := splitHostPort(t, ln.Addr().String())
	target := TCPTarget{Name: "tcp1", Host: host, Port: port, Type: "tcp"}
	p.probeTCP(context.Background(), target)

	endpoint := ln.Addr().String()
	if got := testutil.ToFloat64(p.m.TCPUp.WithLabelValues("tcp1", endpoint, "tcp")); got != 1 {
		t.Errorf("TCPUp = %v, want 1", got)
	}
}

func TestProbeTCP_DialError(t *testing.T) {
	p := newTestProber(t)
	target := TCPTarget{Name: "tcp1", Host: "127.0.0.1", Port: 1, Type: "tcp"}
	p.probeTCP(context.Background(), target)

	if got := testutil.ToFloat64(p.m.TCPUp.WithLabelValues("tcp1", "127.0.0.1:1", "tcp")); got != 0 {
		t.Errorf("TCPUp = %v, want 0", got)
	}
	if got := testutil.ToFloat64(p.m.ProbeErrors.WithLabelValues("tcp1", "tcp")); got != 1 {
		t.Errorf("ProbeErrors = %v, want 1", got)
	}
}

func TestProbeSSH_Banner(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		_, _ = c.Write([]byte("SSH-2.0-OpenSSH_9.3\r\n"))
	}()

	p := newTestProber(t)
	host, port := splitHostPort(t, ln.Addr().String())
	target := TCPTarget{Name: "ssh1", Host: host, Port: port, Type: "ssh"}
	p.probeTCP(context.Background(), target)

	endpoint := ln.Addr().String()
	if got := testutil.ToFloat64(p.m.SSHUp.WithLabelValues("ssh1", endpoint)); got != 1 {
		t.Errorf("SSHUp = %v, want 1", got)
	}
	if got := testutil.ToFloat64(p.m.SSHVersion.WithLabelValues("ssh1", endpoint, "SSH-2.0-OpenSSH_9.3")); got != 1 {
		t.Errorf("SSHVersion = %v, want 1", got)
	}
}

func TestProbeSSH_BadBanner(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		_, _ = c.Write([]byte("HTTP/1.1 400 Bad Request\r\n"))
	}()

	p := newTestProber(t)
	host, port := splitHostPort(t, ln.Addr().String())
	target := TCPTarget{Name: "ssh1", Host: host, Port: port, Type: "ssh"}
	p.probeTCP(context.Background(), target)

	endpoint := ln.Addr().String()
	if got := testutil.ToFloat64(p.m.SSHUp.WithLabelValues("ssh1", endpoint)); got != 0 {
		t.Errorf("SSHUp = %v, want 0", got)
	}
}

func TestURLHost(t *testing.T) {
	cases := map[string]string{
		"https://example.com/foo":  "example.com",
		"http://example.com:8080/": "example.com:8080",
	}
	for raw, want := range cases {
		if got := urlHost(raw); got != want {
			t.Errorf("urlHost(%q) = %q, want %q", raw, got, want)
		}
	}
}
