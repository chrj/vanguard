package main

import "github.com/prometheus/client_golang/prometheus"

type Metrics struct {
	HTTPUp           *prometheus.GaugeVec
	HTTPStatus       *prometheus.GaugeVec
	HTTPDuration     *prometheus.GaugeVec
	HTTPPhrase       *prometheus.GaugeVec
	HTTPContentBytes *prometheus.GaugeVec

	TCPUp       *prometheus.GaugeVec
	TCPDuration *prometheus.GaugeVec

	SSHUp      *prometheus.GaugeVec
	SSHVersion *prometheus.GaugeVec

	SMTPUp *prometheus.GaugeVec

	CertNotAfter   *prometheus.GaugeVec
	CertNotBefore  *prometheus.GaugeVec
	CertExpirySecs *prometheus.GaugeVec

	ProbeLastRun *prometheus.GaugeVec
	ProbeErrors  *prometheus.CounterVec
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		HTTPUp: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "vanguard_http_up",
			Help: "1 if HTTP probe succeeded, 0 otherwise.",
		}, []string{"target", "url"}),
		HTTPStatus: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "vanguard_http_status_code",
			Help: "HTTP response status code.",
		}, []string{"target", "url"}),
		HTTPDuration: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "vanguard_http_duration_seconds",
			Help: "Total duration of the HTTP probe in seconds.",
		}, []string{"target", "url"}),
		HTTPPhrase: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "vanguard_http_phrase_found",
			Help: "1 if the phrase was found in the response body.",
		}, []string{"target", "url", "phrase"}),
		HTTPContentBytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "vanguard_http_content_bytes",
			Help: "Size of the response body in bytes.",
		}, []string{"target", "url"}),

		TCPUp: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "vanguard_tcp_up",
			Help: "1 if TCP connect succeeded, 0 otherwise.",
		}, []string{"target", "endpoint", "type"}),
		TCPDuration: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "vanguard_tcp_duration_seconds",
			Help: "TCP connect duration in seconds.",
		}, []string{"target", "endpoint", "type"}),

		SSHUp: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "vanguard_ssh_up",
			Help: "1 if SSH banner was received.",
		}, []string{"target", "endpoint"}),
		SSHVersion: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "vanguard_ssh_banner_info",
			Help: "SSH banner info, value is always 1.",
		}, []string{"target", "endpoint", "banner"}),

		SMTPUp: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "vanguard_smtp_up",
			Help: "1 if SMTP STARTTLS handshake succeeded.",
		}, []string{"target", "endpoint"}),

		CertNotAfter: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "vanguard_ssl_cert_not_after",
			Help: "Unix timestamp of the leaf certificate NotAfter.",
		}, []string{"target", "endpoint", "proto", "subject", "issuer"}),
		CertNotBefore: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "vanguard_ssl_cert_not_before",
			Help: "Unix timestamp of the leaf certificate NotBefore.",
		}, []string{"target", "endpoint", "proto", "subject", "issuer"}),
		CertExpirySecs: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "vanguard_ssl_cert_expiry_seconds",
			Help: "Seconds until the leaf certificate expires.",
		}, []string{"target", "endpoint", "proto"}),

		ProbeLastRun: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "vanguard_probe_last_run_timestamp",
			Help: "Unix timestamp of the last probe attempt.",
		}, []string{"target", "kind"}),
		ProbeErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "vanguard_probe_errors_total",
			Help: "Number of probe errors.",
		}, []string{"target", "kind"}),
	}
	reg.MustRegister(
		m.HTTPUp, m.HTTPStatus, m.HTTPDuration, m.HTTPPhrase, m.HTTPContentBytes,
		m.TCPUp, m.TCPDuration,
		m.SSHUp, m.SSHVersion,
		m.SMTPUp,
		m.CertNotAfter, m.CertNotBefore, m.CertExpirySecs,
		m.ProbeLastRun, m.ProbeErrors,
	)
	return m
}
