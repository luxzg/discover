# Install Guide

## 1. Prerequisites

- Debian/Ubuntu server
- Go toolchain for building (or copy prebuilt binary)
- Existing TLS cert/key files (Let’s Encrypt) if running HTTPS directly

## 2. Build

```bash
go mod tidy
go build -o discover ./cmd/discover
```

## 3. Create Config

```bash
./discover -config /etc/discover/config.json
```

The binary writes a default config and exits. Edit it before next start.

## 4. Configure

Example important keys:

```json
{
  "listen_address": ":8443",
  "enable_tls": true,
  "tls_cert_path": "/etc/letsencrypt/live/example.com/fullchain.pem",
  "tls_key_path": "/etc/letsencrypt/live/example.com/privkey.pem",
  "admin_secret": "replace-with-strong-random-secret",
  "database_path": "/var/lib/discover/discover.db",
  "daily_ingest_time": "07:30",
  "searxng_instances": ["https://search.unredacted.org"]
}
```

For local testing you can set `"enable_tls": false` and use `http://localhost:<port>`.

## 5. Run Manually

```bash
./discover -config /etc/discover/config.json
```

## 6. systemd Service

Create `/etc/systemd/system/discover.service`:

```ini
[Unit]
Description=Discover Personal Feed
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=discover
Group=discover
WorkingDirectory=/opt/discover
ExecStart=/opt/discover/discover -config /etc/discover/config.json
Restart=on-failure
RestartSec=3
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=full
ProtectHome=true
ReadWritePaths=/var/lib/discover

[Install]
WantedBy=multi-user.target
```

Then enable and start:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now discover.service
sudo systemctl status discover.service
```
