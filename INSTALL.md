# Install Guide

## 1. Prerequisites

- Debian/Ubuntu server
- `sudo` access
- Existing TLS cert/key files (Let’s Encrypt) if running HTTPS directly

### 1.1 Create dedicated service user

```bash
sudo useradd -m -s /bin/bash discover
sudo su - discover
```

### 1.2 Install Go (manual) and add PATH for this user

For downloads and official install instructions:
- https://go.dev/dl/
- https://go.dev/doc/install

Example (amd64 Linux):

```bash
wget https://go.dev/dl/go1.26.0.linux-amd64.tar.gz
tar -C "$HOME" -xzf go1.26.0.linux-amd64.tar.gz

echo 'export PATH=$PATH:$HOME/go/bin' >> ~/.bashrc
source ~/.bashrc
# use ~/.bashrc or ~/.profile depending on environment

go version
```

### 1.3 Create project directory

```bash
mkdir -p ~/apps
cd ~/apps
```

## 2. Clone and Build

Keep running commands as user `discover`:

```bash
git clone https://github.com/luxzg/discover.git
cd discover
git status
go mod tidy
go build -o discover ./cmd/discover
```

## 3. Create Config

```bash
./discover -config config.json
```

The binary writes a default `config.json` and exits. Edit it before next start.
On later runs existing config is not overwritten.

```bash
nano config.json
```

## 4. Configure

Example important keys:

```json
{
  "listen_address": ":8443",
  "enable_tls": true,
  "tls_cert_path": "/etc/letsencrypt/live/example.com/fullchain.pem",
  "tls_key_path": "/etc/letsencrypt/live/example.com/privkey.pem",
  "user_name": "discover",
  "user_secret": "replace-with-strong-random-user-secret",
  "admin_secret": "replace-with-strong-random-secret",
  "database_path": "discover.db",
  "daily_ingest_time": "07:30",
  "ingest_interval_minutes": 120,
  "feed_min_score": 1,
  "auto_hide_below_score": 1,
  "dedupe_title_key_chars": 50,
  "thumbnail_refresh_min_score": 60,
  "thumbnail_refresh_max_per_run": 40,
  "hide_rule_default_penalty": 10,
  "searxng_instances": ["http://localhost:8888"]
}
```

For local testing you can set `"enable_tls": false` and use `http://localhost:<port>`.

## 5. Run Manually and Test

```bash
./discover -config config.json
```

Test by opening local IP like:
```
http://192.168.1.2:8443/admin
```
Sign in with your `admin_secret`, then set up a single topic (enabled), click Add/Update, and once it is there click `Run Now` in the `Ingestion` section.
If ingestion finishes without error, continue to front end, eg. `http://192.168.1.2:8443/`, and sign in with `user_name` + `user_secret`.
If it works proceed setting up TLS, and systemd service (as root).

## 6. systemd Service

Create `/etc/systemd/system/discover.service` (as root):
`nano /etc/systemd/system/discover.service`

```ini
[Unit]
Description=Discover Personal Feed
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=discover
Group=discover
WorkingDirectory=/home/discover/apps/discover
ExecStart=/home/discover/apps/discover/discover -config /home/discover/apps/discover/config.json
Restart=on-failure
RestartSec=3
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=full
ProtectHome=false
ReadWritePaths=/home/discover/apps/discover

[Install]
WantedBy=multi-user.target
```

Then enable and start:

```bash
systemctl daemon-reload
systemctl enable --now discover.service
systemctl status discover.service
```

If you've setup DNS and TLS you should be able to read it now by visiting public URL like:
`https://discover-feed.example.org:8443/`

### 6.1 Diagnostics (journalctl)

Useful log checks for the running service:

```bash
# today's logs (from midnight)
journalctl -u discover --since today --no-pager

# specific date/time range
journalctl -u discover --since "2026-02-22 00:00:00" --until "2026-02-22 23:59:59" --no-pager

# last N lines
journalctl -u discover -n 200 --no-pager

# follow live logs
journalctl -u discover --since today -f
```

## 7. Update Existing Installation

Preferred update method uses helper scripts.

### 7.1 Remote update script (build-only, run on server)

Run as `discover` user on the remote server:

```bash
cd /home/discover/apps/discover
./scripts/update_remote.sh
```

Notes:
- script runs: `git pull --ff-only`, `go mod tidy`, `go build -o /home/discover/apps/discover/discover ./cmd/discover`
- does not restart service on its own

### 7.2 Local wrapper to run remote update over SSH

From your local PC:

```bash
cd ~/dev/discover
./scripts/run_remote_update.sh
```

It asks for remote host/IP and SSH user, then executes:

- `sudo systemctl stop discover`
- run update/build script as `discover`
- `sudo systemctl start discover`
- `sudo systemctl status discover --no-pager -n 25`
- `sudo journalctl -u discover -n 50 --no-pager`

Permission model:
- remote SSH user must have sudo rights for `systemctl` and `journalctl`
- remote SSH user does not need direct write access to `/home/discover/apps/discover`; build step is executed as `discover`

### 7.3 Manual fallback (if scripts are unavailable)

```bash
sudo systemctl stop discover
sudo su - discover
cd ~/apps/discover
git pull --ff-only
go mod tidy
go build -o discover ./cmd/discover
exit
sudo systemctl start discover
sudo systemctl status discover
```

If you changed config keys in a new release, review and update `config.json` before starting the service.

## 8. Uninstall

If you want to remove Discover completely:

```bash
sudo systemctl stop discover
sudo systemctl disable discover
sudo rm -f /etc/systemd/system/discover.service
sudo systemctl daemon-reload
sudo systemctl reset-failed
```

Optional data cleanup (permanent):

```bash
sudo rm -rf /home/discover/apps/discover
sudo userdel -r discover
```

If you want to keep article/history data, back up `discover.db` before deleting the app directory.
