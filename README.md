# vanguard

A small Prometheus black-box exporter. Probes run over the regular network;
the `/metrics` endpoint is served inside a userspace WireGuard tunnel so only
peers on the VPN can scrape it.

Vanguard probes HTTP(S) websites, raw TCP ports, SSH banners, and SMTP
STARTTLS endpoints on the interval you configure. Results (latency, status,
phrase matches, TLS certificate expiry) are exposed as Prometheus gauges and
counters.

## Features

- Metrics endpoint served on a userspace WireGuard tunnel via
  [`wgnet`](https://github.com/chrj/wgnet) — no kernel module or
  `wg`/`wg-quick` needed, and the exporter isn't reachable off-VPN.
- HTTP probes with status, duration, body size, and phrase matching.
- TCP connect probes with optional protocol checks:
  - `ssh` — reads and exports the server banner.
  - `smtp_starttls` — negotiates STARTTLS and records leaf cert expiry.
- Leaf TLS certificate `NotBefore` / `NotAfter` / remaining-seconds for HTTPS
  and SMTP.
- Prometheus `/metrics` endpoint served on the VPN interface, plus `/healthz`.

## Install

```sh
go install github.com/chrj/vanguard@latest
```

Or build from source:

```sh
git clone https://github.com/chrj/vanguard
cd vanguard
go build .
```

Prebuilt binaries are published on the [releases
page](https://github.com/chrj/vanguard/releases).

## Configure

Copy `vanguard.example.toml` and fill in the WireGuard credentials and the
targets you want probed:

```toml
[wireguard]
private_key       = "CLIENT_PRIVATE_KEY_BASE64"
server_public_key = "SERVER_PUBLIC_KEY_BASE64"
server_endpoint   = "vpn.example.com:51820"
address           = "10.42.0.2"
dns               = ["10.42.0.1"]

[metrics]
listen = ":9090"
path   = "/metrics"

[probe]
interval = "30s"
timeout  = "10s"

[[website]]
name    = "main"
url     = "https://example.com/"
phrases = ["Welcome"]

[[tcp_service]]
name = "ssh-main"
host = "10.42.0.1"
port = 22
type = "ssh"

[[tcp_service]]
name        = "smtp-main"
host        = "10.42.0.1"
port        = 25
type        = "smtp_starttls"
server_name = "mail.example.com"
```

`tcp_service.type` accepts `tcp` (default), `ssh`, or `smtp_starttls`.

## Run

```sh
vanguard -config /etc/vanguard.toml
```

The metrics server binds to the VPN address by default. If
`metrics.listen` has no host (e.g. `:9090`), the listener uses
`wireguard.address`, so scrapes only succeed from peers inside the tunnel.

## Metrics

| Metric | Type | Labels |
| --- | --- | --- |
| `vanguard_http_up` | gauge | `target`, `url` |
| `vanguard_http_status_code` | gauge | `target`, `url` |
| `vanguard_http_duration_seconds` | gauge | `target`, `url` |
| `vanguard_http_phrase_found` | gauge | `target`, `url`, `phrase` |
| `vanguard_http_content_bytes` | gauge | `target`, `url` |
| `vanguard_tcp_up` | gauge | `target`, `endpoint`, `type` |
| `vanguard_tcp_duration_seconds` | gauge | `target`, `endpoint`, `type` |
| `vanguard_ssh_up` | gauge | `target`, `endpoint` |
| `vanguard_ssh_banner_info` | gauge | `target`, `endpoint`, `banner` |
| `vanguard_smtp_up` | gauge | `target`, `endpoint` |
| `vanguard_ssl_cert_not_after` | gauge | `target`, `endpoint`, `proto`, `subject`, `issuer` |
| `vanguard_ssl_cert_not_before` | gauge | `target`, `endpoint`, `proto`, `subject`, `issuer` |
| `vanguard_ssl_cert_expiry_seconds` | gauge | `target`, `endpoint`, `proto` |
| `vanguard_probe_last_run_timestamp` | gauge | `target`, `kind` |
| `vanguard_probe_errors_total` | counter | `target`, `kind` |

Go runtime and process collectors are also registered.

## License

MIT — see [LICENSE](LICENSE).
