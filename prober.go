package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/smtp"
	"net/url"
	"strings"
	"time"
)

type Prober struct {
	cfg    *Config
	dialer *net.Dialer
	client *http.Client
	m      *Metrics
}

func NewProber(cfg *Config, m *Metrics) *Prober {
	dialer := &net.Dialer{}
	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   cfg.Probe.Timeout.D(),
		ResponseHeaderTimeout: cfg.Probe.Timeout.D(),
		DisableKeepAlives:     true,
		ForceAttemptHTTP2:     false,
	}
	return &Prober{
		cfg:    cfg,
		dialer: dialer,
		client: &http.Client{
			Transport: transport,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return http.ErrUseLastResponse
				}
				return nil
			},
		},
		m: m,
	}
}

func (p *Prober) Run(ctx context.Context) {
	t := time.NewTicker(p.cfg.Probe.Interval.D())
	defer t.Stop()

	p.probeAll(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.probeAll(ctx)
		}
	}
}

func (p *Prober) probeAll(ctx context.Context) {
	for _, w := range p.cfg.Websites {
		p.probeHTTP(ctx, w)
	}
	for _, t := range p.cfg.TCP {
		p.probeTCP(ctx, t)
	}
}

func (p *Prober) targetTimeout(d Duration) time.Duration {
	if d > 0 {
		return d.D()
	}
	return p.cfg.Probe.Timeout.D()
}

func (p *Prober) probeHTTP(parent context.Context, t WebsiteTarget) {
	ctx, cancel := context.WithTimeout(parent, p.targetTimeout(t.Timeout))
	defer cancel()

	p.m.ProbeLastRun.WithLabelValues(t.Name, "http").SetToCurrentTime()

	start := time.Now()
	var dnsStart, connectStart, tlsStart time.Time
	var dnsDur, connectDur, tlsDur, ttfb time.Duration
	trace := &httptrace.ClientTrace{
		DNSStart: func(_ httptrace.DNSStartInfo) {
			dnsStart = time.Now()
		},
		DNSDone: func(info httptrace.DNSDoneInfo) {
			if info.Err == nil {
				dnsDur = time.Since(dnsStart)
			}
		},
		ConnectStart: func(_, _ string) {
			connectStart = time.Now()
		},
		ConnectDone: func(_, _ string, err error) {
			if err == nil {
				connectDur = time.Since(connectStart)
			}
		},
		TLSHandshakeStart: func() {
			tlsStart = time.Now()
		},
		TLSHandshakeDone: func(_ tls.ConnectionState, err error) {
			if err == nil {
				tlsDur = time.Since(tlsStart)
			}
		},
		GotFirstResponseByte: func() {
			ttfb = time.Since(start)
		},
	}
	req, err := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, trace), http.MethodGet, t.URL, nil)
	if err != nil {
		p.httpFail(t, err)
		return
	}
	req.Header.Set("User-Agent", "vanguard/0.1")

	resp, err := p.client.Do(req)
	if err != nil {
		p.httpFail(t, err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		p.httpFail(t, err)
		return
	}

	p.m.HTTPDuration.WithLabelValues(t.Name, t.URL).Set(time.Since(start).Seconds())
	p.m.HTTPDNSDuration.WithLabelValues(t.Name, t.URL).Set(dnsDur.Seconds())
	p.m.HTTPConnectDuration.WithLabelValues(t.Name, t.URL).Set(connectDur.Seconds())
	p.m.HTTPTLSDuration.WithLabelValues(t.Name, t.URL).Set(tlsDur.Seconds())
	p.m.HTTPTTFBDuration.WithLabelValues(t.Name, t.URL).Set(ttfb.Seconds())
	p.m.HTTPStatus.WithLabelValues(t.Name, t.URL).Set(float64(resp.StatusCode))
	p.m.HTTPContentBytes.WithLabelValues(t.Name, t.URL).Set(float64(len(body)))

	ok := resp.StatusCode >= 200 && resp.StatusCode < 400
	for _, phrase := range t.Phrases {
		found := 0.0
		if strings.Contains(string(body), phrase) {
			found = 1
		} else {
			ok = false
		}
		p.m.HTTPPhrase.WithLabelValues(t.Name, t.URL, phrase).Set(found)
	}

	up := 0.0
	if ok {
		up = 1
	}
	p.m.HTTPUp.WithLabelValues(t.Name, t.URL).Set(up)

	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		host := urlHost(t.URL)
		p.recordCert(t.Name, host, "https", resp.TLS.PeerCertificates[0])
	}
}

func (p *Prober) httpFail(t WebsiteTarget, err error) {
	slog.Warn("http probe failed", "target", t.Name, "url", t.URL, "err", err)
	p.m.HTTPUp.WithLabelValues(t.Name, t.URL).Set(0)
	p.m.ProbeErrors.WithLabelValues(t.Name, "http").Inc()
}

func urlHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return u.Host
}

func (p *Prober) probeTCP(parent context.Context, t TCPTarget) {
	ctx, cancel := context.WithTimeout(parent, p.targetTimeout(t.Timeout))
	defer cancel()

	endpoint := net.JoinHostPort(t.Host, fmt.Sprintf("%d", t.Port))
	kind := t.Type
	if kind == "" {
		kind = "tcp"
	}
	p.m.ProbeLastRun.WithLabelValues(t.Name, kind).SetToCurrentTime()

	start := time.Now()
	conn, err := p.dialer.DialContext(ctx, "tcp", endpoint)
	if err != nil {
		p.m.TCPUp.WithLabelValues(t.Name, endpoint, kind).Set(0)
		p.m.ProbeErrors.WithLabelValues(t.Name, kind).Inc()
		return
	}
	defer func() { _ = conn.Close() }()

	p.m.TCPDuration.WithLabelValues(t.Name, endpoint, kind).Set(time.Since(start).Seconds())
	p.m.TCPUp.WithLabelValues(t.Name, endpoint, kind).Set(1)

	deadline, _ := ctx.Deadline()
	_ = conn.SetDeadline(deadline)

	switch kind {
	case "ssh":
		p.probeSSH(t, endpoint, conn)
	case "smtp_starttls":
		p.probeSMTP(t, endpoint, conn)
	}
}

func (p *Prober) probeSSH(t TCPTarget, endpoint string, conn net.Conn) {
	br := bufio.NewReader(conn)
	line, err := br.ReadString('\n')
	if err != nil || !strings.HasPrefix(line, "SSH-") {
		p.m.SSHUp.WithLabelValues(t.Name, endpoint).Set(0)
		p.m.ProbeErrors.WithLabelValues(t.Name, "ssh").Inc()
		return
	}
	p.m.SSHUp.WithLabelValues(t.Name, endpoint).Set(1)
	p.m.SSHVersion.WithLabelValues(t.Name, endpoint, strings.TrimSpace(line)).Set(1)
}

func (p *Prober) probeSMTP(t TCPTarget, endpoint string, conn net.Conn) {
	serverName := t.ServerName
	if serverName == "" {
		serverName = t.Host
	}

	c, err := smtp.NewClient(conn, serverName)
	if err != nil {
		p.m.SMTPUp.WithLabelValues(t.Name, endpoint).Set(0)
		p.m.ProbeErrors.WithLabelValues(t.Name, "smtp_starttls").Inc()
		return
	}
	defer func() { _ = c.Close() }()

	if err := c.Hello("vanguard.local"); err != nil {
		p.m.SMTPUp.WithLabelValues(t.Name, endpoint).Set(0)
		p.m.ProbeErrors.WithLabelValues(t.Name, "smtp_starttls").Inc()
		return
	}

	tlsCfg := &tls.Config{ServerName: serverName, MinVersion: tls.VersionTLS12}
	if err := c.StartTLS(tlsCfg); err != nil {
		p.m.SMTPUp.WithLabelValues(t.Name, endpoint).Set(0)
		p.m.ProbeErrors.WithLabelValues(t.Name, "smtp_starttls").Inc()
		return
	}

	state, ok := c.TLSConnectionState()
	if !ok || len(state.PeerCertificates) == 0 {
		p.m.SMTPUp.WithLabelValues(t.Name, endpoint).Set(0)
		p.m.ProbeErrors.WithLabelValues(t.Name, "smtp_starttls").Inc()
		return
	}

	p.m.SMTPUp.WithLabelValues(t.Name, endpoint).Set(1)
	p.recordCert(t.Name, endpoint, "smtp", state.PeerCertificates[0])
}

func (p *Prober) recordCert(target, endpoint, proto string, cert *x509.Certificate) {
	p.m.CertNotAfter.WithLabelValues(target, endpoint, proto, cert.Subject.CommonName, cert.Issuer.CommonName).Set(float64(cert.NotAfter.Unix()))
	p.m.CertNotBefore.WithLabelValues(target, endpoint, proto, cert.Subject.CommonName, cert.Issuer.CommonName).Set(float64(cert.NotBefore.Unix()))
	p.m.CertExpirySecs.WithLabelValues(target, endpoint, proto).Set(time.Until(cert.NotAfter).Seconds())
}
