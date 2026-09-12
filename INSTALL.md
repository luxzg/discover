# Install Guide

## 1. Prerequisites

- Debian/Ubuntu server
- `sudo` access
- Git, Bash, curl, CA certificates, `sqlite3`, and standard coreutils/util-linux tools
- Go 1.26.8 or newer; use a supported security-patched Go release
- Existing TLS cert/key files (Let’s Encrypt) if running HTTPS directly

### 1.1 Create dedicated service user

```bash
sudo apt update
sudo apt install git curl ca-certificates sqlite3
sudo useradd -m -s /bin/bash discover
sudo su - discover
```

### 1.2 Install Go (manual) and add PATH for this user

For downloads and official install instructions:
- https://go.dev/dl/
- https://go.dev/doc/install

Example (amd64 Linux):

```bash
curl -fLO https://go.dev/dl/go1.26.8.linux-amd64.tar.gz
sha256sum go1.26.8.linux-amd64.tar.gz
# Compare with the SHA256 published on go.dev/dl before extracting.
mkdir -p "$HOME/toolchains/go1.26.8"
tar -C "$HOME/toolchains/go1.26.8" --strip-components=1 -xzf go1.26.8.linux-amd64.tar.gz

echo 'export PATH="$HOME/toolchains/go1.26.8/bin:$HOME/go/bin:$PATH"' >> ~/.profile
source ~/.profile

go version
```

Use the matching architecture archive on ARM64 or other systems. Keep the Go
toolchain separate from `$HOME/go`, which is normally GOPATH/module storage.
Do not extract over an older Go installation. Re-enter `sudo su - discover` after
updating `.profile`. Existing Go installs can use automatic toolchain selection
from `go.mod`; `GOTOOLCHAIN=local` requires a sufficiently recent installed Go.

Build helpers respect the Go executable selected by PATH before using fallback
installation locations. Confirm `go version` as `discover` on the server:
updating the laptop compiler does not update the server compiler. Review supported
patches periodically using `MAINTENANCE.md`; its Node/npm/Chromium tooling is for
development only and is not required on this server.

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
./scripts/build.sh
./discover --version
```

## 3. Create Config

```bash
./discover -config config.json
```

The binary writes a default `config.json` and exits. Edit it before next start.
On later runs existing config is not overwritten.
Missing keys use in-memory defaults with a startup warning. Unknown keys and
null values fail validation; fix typos explicitly. `admin_bind_cidrs: []` explicitly
allows admin login from any IP, but still requires the admin secret. Invalid CIDRs
abort startup rather than accidentally widening access.

Default admin networks are loopback (`127.0.0.1/32`, `::1/128`), `192.168.0.0/16`
and `10.0.0.0/8`. Add your actual administrator access network to
`admin_bind_cidrs` if different. Public addresses, `172.16.0.0/12` and non-loopback
IPv6 are not allowed by default, regardless of correct credentials. Prefer a
narrow allowlist rather than `[]` for an internet-accessible service.

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
./discover --check-config -config config.json
./discover -config config.json
```

`--check-config` validates values and TLS key/certificate access without opening
or modifying SQLite, generating config, or contacting external services. Run it
as the service user. TLS certificate directory permissions must permit that user
to read the configured files; do not make the private key world-readable.

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
TimeoutStopSec=30
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

Rule-based hide jobs log `hide: completed in ...` or `hide: failed after ...`.
To isolate these from ingestion logs:

```bash
journalctl -u discover --since today --no-pager | grep 'hide:'
```

The browser acknowledges accepted jobs immediately; completion no longer depends
on an open browser request. A service restart cancels unfinished jobs, so verify
the rule/card state after deployment before retrying an interrupted action.

## 7. Update Existing Installation

Preferred update method uses helper scripts.

### 7.1 Remote update script (build-only, run on server)

Run as `discover` user on the remote server:

```bash
cd /home/discover/apps/discover
./scripts/update_remote.sh
```

Notes:
- refuses a dirty working tree, pulls fast-forward only, and uses pinned Go modules
- writes `discover.next`, verifies build metadata and checks existing config/TLS
- does not replace the live binary or stop/restart the service
- use the local wrapper below for full deployment; do not run a service-user-writable script as root

### 7.2 Local wrapper to run remote update over SSH

From your local PC:

```bash
cd ~/dev/discover
git status --short
git pull --ff-only
# Review scripts/deploy.sh before granting it remote sudo privileges.
./scripts/run_remote_update.sh
```

Or provide values non-interactively:

```bash
./scripts/run_remote_update.sh -ip 10.10.10.10 -user myusername
```

If arguments are not provided, the script prompts for remote host/IP and SSH
user. SSH asks for its authentication and sudo prompts through the allocated TTY.
Update the local checkout first: the remote pull cannot update the local copy
of privileged orchestration. Resolve any local changes before pulling; do not
discard them automatically.
It sends the local, reviewed privileged orchestration over SSH (rather than
executing service-user-writable scripts as root), pulls the remote checkout as
`discover`, then:

1. Builds a candidate as `discover` while the current service remains running.
2. Validates config and TLS access as `discover`.
3. Retains the previous binary and config in an access-restricted backup directory.
4. Stops the service and creates a SQLite `.backup` snapshot, including committed WAL data, then checks snapshot integrity.
5. Atomically replaces the binary and starts the service.
6. Checks active service status and prints version/recent logs.

Build/config failures leave the running binary alone. Failures after stopping
attempt to restart the prior service; failures after swapping attempt to restore
the previous binary first. **No database is ever restored automatically.** Backups
and failed snapshots are retained under `/var/backups/discover/`; copy important backups off-host.
An active process check is not a full application health test: verify login/feed
and the new version after deployment.

Compiler/dependency updates require this rebuild and restart too. To inspect the
installed binary's toolchain and module metadata on the server (from its checkout):

```bash
go version -m ./discover
./discover --version
```

The local wrapper builds on the remote server, not on the laptop. If its compiler
needs a security update, install that as an operator action before deployment;
do not assume a recently rebuilt laptop binary is the one running remotely.

Permission model:
- remote SSH user needs sudo permission to run the deployment orchestration, including `runuser`, service control and backup operations
- remote SSH user does not need direct write access to `/home/discover/apps/discover`; build step is executed as `discover`
- privileged orchestration comes from your local administrator-controlled checkout; review it before running. Server-side build scripts, candidate inspection and SQLite backup commands run as `discover`, never root

### 7.3 Server-Only Alternative

Use a separate checkout owned by the administrator, not by the service account.
For example, in the administrator's home (choose a directory not used already):

```bash
git clone https://github.com/luxzg/discover.git ~/discover-deployer
cd ~/discover-deployer
git log -1
# Review scripts/deploy.sh before granting it root privileges.
sudo bash ./scripts/deploy.sh
```

If you changed config keys in a new release, review and update `config.json` before starting the service.

### 7.4 Upgrade Checks And Recovery

- v2.26 upgrades the SQLite engine bundled in Discover from 3.50.4 to 3.53.4.
  Use the same update script and existing `discover.db`; no export/import,
  database conversion, re-ingestion, config change or system SQLite upgrade is
  required. Existing deployment backups remain part of the normal update.
- When upgrading from before v2.23, the first start migrates derived URL/title keys, score baselines and timestamp representations. Large databases can take longer to start. Do not interrupt startup just because the feed is not immediately reachable.
- Existing scores remain the baseline; repeated identical results stop increasing them. Review your score thresholds only after observing new results.
- Old hidden decisions remain hidden; known automatic duplicate hides are tracked separately going forward.
- Restart invalidates in-memory sessions: log in again to feed/admin.
- Verify displayed version, publication dates where known, Other sources, rule edits and one manual ingestion.

```bash
journalctl -u discover --since today -n 100 --no-pager
```

Before any manual database restore, stop the service and retain a new snapshot
of its current state. A restore discards newer actions/articles and requires an
explicit operator decision; do not copy a database over a running service or
mix a restored DB with stale WAL/SHM sidecars. Keep the matching previous binary
and config alongside its backup. Diagnose failures before choosing a rollback.

For an independent live SQLite backup, use the administrator-owned checkout
from section 7.3, not scripts writable by the service account:

```bash
cd ~/discover-deployer
sudo bash ./scripts/backup-database.sh --database /home/discover/apps/discover/discover.db --backup-dir /var/backups/discover --owner root
```

Use the actual `database_path` if customized. Do not back up only the `.db` file
with ordinary `cp` while the service is running; committed data may be in WAL.
The standalone helper keeps snapshots owned by the executing user; it does not
transfer ownership. Root backups require a root-owned destination and ancestors
without group/other write access. Copying a backup to another account is a
separate deliberate administrator action.

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

If you want to keep article/history data, use the backup command above and copy
the snapshot/config outside the app directory before cleanup. Deployment snapshots
under `/var/backups/discover` are retained separately. TLS material and SearXNG are separate;
see `SEARXNG.md` before removing that service.
