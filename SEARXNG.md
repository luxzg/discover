# SearXNG For Discover

## Current Integration (Reviewed 2026-10-05)

Discover uses the JSON search endpoint of a private SearXNG instance, normally
`http://localhost:8888`. Enable `json` under `search.formats`. Keep the listener
on loopback unless another trusted host needs access; do not expose an unprotected
search service just to serve Discover.

Relevant settings (merge into your existing settings; do not replace your secret):

```yaml
use_default_settings: true
search:
  formats:
    - html
    - json
server:
  bind_address: "127.0.0.1"
  port: 8888
  public_instance: false
```

For new installations, follow the current [upstream installation guide](https://docs.searxng.org/admin/installation.html)
and its maintenance/migration notes. It now documents Granian as well as other
server options. Use a production application server rather than exposing the
development server. The older pyenv recipe below is retained as deployment
history, not a claim that today's SearXNG requires Python 3.12 or cannot use
Debian's current Python. Do not rebuild a working instance solely for Discover's update.

### Search Contract And Diagnostics

Discover sends eight requests per topic/instance: `day` and `week`, `news` and
`general`, pages `1` and `2`, always `format=json`. It does not request an unlimited
time range. The [search API](https://docs.searxng.org/dev/search_api) documents
pagination/categories and notes that time support is engine-dependent. That page
omits `week` from its example enumeration, but the current
[request parser](https://github.com/searxng/searxng/blob/master/searx/webadapter.py)
accepts it. There is no supported `count` parameter to enlarge a page.

Since Discover v2.29 these eight requests are sequential and individually paced:
5 seconds plus 0..2 seconds jitter by default. The original topic delay is extra.
Missing JSON keys inherit defaults without editing the file. At 32 topics this
adds roughly 25 minutes of search pauses; it reduces bursts, not total request
count. It does not guarantee upstream rate limits/CAPTCHAs disappear. Engine
backoff/clearer outcome reporting remains future work.

SearXNG has its own [engine suspension settings](https://docs.searxng.org/admin/settings/settings_search.html)
and skips engines while those suspensions are active. Repeated suspension warnings
therefore do not necessarily mean repeated requests reached that provider. Discover
v2.31 reports observations but does not yet select engines or maintain per-engine
cooldowns; selective requests and sparse recovery probes are planned in TODO.md.
Do not reset suspensions or repeatedly restart SearXNG to force retries.

Test one exact Discover-style request on the server:

```bash
curl --fail-with-body --get 'http://localhost:8888/search' --data-urlencode 'q=site:wccftech.com gpu' --data 'format=json' --data 'categories=news' --data 'time_range=week' --data 'pageno=1'
```

Repeat with `categories=general`, `time_range=day` and `pageno=2` as needed.
Inspect `results`, `unresponsive_engines`, `publishedDate` and `pubdate` in JSON;
do not share full private queries/results without redaction. `results: []` is a
successful empty search. HTTP 429, invalid/non-JSON replies, missing/null results
or unresponsive engines are different conditions. Check SearXNG's preferences
and logs for engine support/errors. A successful HTTP reply does not imply that
every underlying engine answered.

No setting guarantees publication dates or a fixed result count. Some engines
ignore time filters or return updated/undated material. Discover preserves known
dates and additionally attempts bounded publisher metadata enrichment. If neither
source supplies publication, the feed labels original discovery as First seen,
not publication. `site:` is
sent to engines as query syntax and depends on their support.

Discover does not follow redirects from the search API, even within the same
origin. Set the final base URL explicitly. Configured SearXNG endpoints and their
DNS are administrator-trusted network destinations, unlike public article URLs.
Use loopback/IP for a local instance; use HTTPS for a remote instance.

## Existing Installation Inventory Before Updating

On 2026-10-05 the operator ran the read-only inventory and confirmed:

- service `searxng`, dedicated user `searxng`, working directory
  `/usr/local/searxng/searxng` and unit `/etc/systemd/system/searxng.service`
- source revision `da9c0815a`, dated 2026-02-15 11:30:35 +0100
- the expected virtual environment runs Python 3.12.12; `pip check` reported no
  broken requirements, and `searx-settings.yml` exists

This confirms the historical source/venv layout, not current upstream currency
or full settings/application-server compatibility. Source revisions/dates are a
better identifier than assuming a numeric release. No upgrade was performed.

From an administrator-owned Discover checkout on the server (see INSTALL.md
section 7.3), run the read-only inventory:

```bash
cd ~/discover-deployer
git pull --ff-only
sudo bash scripts/searxng-inspect.sh
```

No `chmod` is needed when invoking it with Bash. Changing its executable bit
inside a service checkout can make Git report a local mode change and block
the Discover updater's clean-tree preflight. If that is the only change you
made, inspect `git diff --summary` there as `discover`, then restore just that
bit with `chmod -x scripts/searxng-inspect.sh`; do not discard unrelated edits.

Alternatively run `bash scripts/searxng-inspect.sh` as the `searxng` service user
if the helper is readable from that user's chosen location. The root invocation
executes Git/Python checks as the service user, never root. It reports selected
systemd location fields, Git revision/date/dirty state, Python version, dependency
consistency and settings-file presence. It does not read settings contents,
change packages, stop services, fetch Git updates or modify data. Custom layouts
can pass `--home /absolute/path --service service_name`; unexpected/missing paths
are diagnostic failures, not permission to reinstall automatically.

Share only non-sensitive revision/Python/dependency results if troubleshooting;
do not paste the complete settings or environment. The updater below now checks
the actual launch contract and candidate compatibility without replacing settings.
Do not run generic upstream install-script upgrades blindly against this layout.

Upstream's current [installation documentation](https://docs.searxng.org/admin/installation-searxng.html)
uses a virtual environment with editable package installation and minimal settings
overrides inheriting current defaults. A historical full copy of settings may
retain stale engine definitions even after a source upgrade; review it rather
than replacing it and losing local settings. Application-server migration is a
separate decision; the [Granian guide](https://docs.searxng.org/admin/installation-granian.html)
documents a production option. Keeping a private instance updated a few times
a year can refresh engine adapters but cannot guarantee result quality/dates.

## Update The Existing Source/Pyenv Installation

Use an **administrator-owned** Discover checkout, never
`/home/discover/apps/discover` for a sudo script. The administrator's checkout
and ancestors must not be group/other writable. Do not relax a private home:
the updater stages reviewed helper copies under `/run` for the service user.

On the server, sign in as your administrator (not `discover` or `searxng`):

```bash
# First time only, if this directory does not already exist:
git clone https://github.com/luxzg/discover.git ~/discover-deployer
cd ~/discover-deployer
# On later runs, inspect changes first, then update this administrator copy:
git status --short
git pull --ff-only
```

Review `scripts/searxng-update.sh`, `scripts/searxng-worker.sh` and
`scripts/searxng-check.py` before granting sudo. Run the preflight:

```bash
sudo bash scripts/searxng-update.sh --check
```

This creates/removes disposable reviewed helper copies only; it does not download
packages, back up settings, search upstream, change source/config or stop services.
It requires the inventoried service contract: dedicated non-root user,
`WorkingDirectory=/usr/local/searxng/searxng`,
`ExecStart=/usr/local/searxng/searx-venv/bin/python -m searx.webapp`, and exactly
`SEARXNG_SETTINGS_PATH=/usr/local/searxng/searx-settings.yml` with no environment
files. It checks clean source, installed dependencies, private `127.0.0.1:8888`,
debug disabled, JSON enabled and a non-default secret. Unsupported contracts are
refused, not automatically rewritten. `--home`/`--service` support the same layout
at another path; IPv6/port/application-server changes need separate review.

The full-legacy-settings warning is nonfatal and does not rewrite those settings.
Successful preflight ends with **Preflight complete**. If an older helper prints
only `Terminated` after settings validation, update the administrator checkout
and retry `--check`, not `--apply`. Since v2.30, revision reporting bypasses Git
pagers/signature checks and credential prompts; timeout errors name the worker
phase and its unchanged deadline. If it still fails, share the last progress
step and phase/exit message, not settings or environment contents.

If preflight succeeds, update explicitly:

```bash
sudo bash scripts/searxng-update.sh --apply
```

**Effects and prerequisites:**

- Requires sudo, Bash, Git, runuser/flock, timeout and standard coreutils, a
  working existing Python/venv, HTTPS access to GitHub/PyPI and at least 2 GiB
  free space. Native dependency builds may need OS development libraries; the
  script does not install system packages or upgrade pyenv/base Python. Current
  [upstream package metadata](https://github.com/searxng/searxng/blob/master/setup.py)
  accepts Python 3.10+; installation/import checks still decide actual compatibility.
- Downloads current upstream `master` into a new `update-*` directory under the
  service home. `--revision FULL_40_CHARACTER_COMMIT` pins a reviewed revision.
  Creates a separate venv using existing base Python, installs current upstream
  packages/bootstrap dependencies, records revision and resolved packages, checks
  dependency consistency, loads candidate settings and checks webapp module
  resolution/syntax. It deliberately does not import/start webapp before switching:
  upstream webapp imports initialize caches and engine networking. Full application
  startup compatibility is checked by post-switch local health and recovery.
  These operations run as `searxng`, never root, with caller proxy/pip/Python
  environment overrides removed. Private pip index/proxy setups need separate
  adaptation; no arbitrary package source flag is accepted.
- Retains root-restricted settings/unit snapshots under `/var/backups/searxng`.
  Previous source/venv paths are moved into the candidate's `previous-*` paths
  at activation; candidate and old installations are never automatically deleted.
  Scripts do not overwrite settings or the systemd unit. Concurrent settings/unit
  edits during preparation abort before service stop.
- Stops SearXNG only after preparation, switches the stable source/venv paths to
  candidate symlinks, starts the unchanged unit and checks local JSON `/config`.
  Preparation is bounded to 30 minutes; worker phases to two minutes. Local
  health requires JSON config with enabled news/general engines, but is not
  evidence that upstream engines recovered. Service restart can
  clear process-local engine suspension history; do not use this updater/restarts
  repeatedly as a way to force blocked engines to answer.
- If activation/start/local health fails, attempts to put the previous source/
  environment paths back and restart. No settings or databases are restored.
  Failures before activation leave the running installation alone. SIGKILL or
  power loss can interrupt recovery; retain printed paths for manual inspection.
- A full historical settings copy remains intact and may retain stale engine or
  plugin definitions. Failed candidate settings loads abort before downtime;
  review compatibility/minimal overrides separately rather than replacing secrets
  with defaults. Existing development-server launch is intentionally retained;
  migrating to Granian/uWSGI is a different task.

Afterwards, check logs and continue normal scheduled Discover use:

```bash
journalctl -u searxng --since today -n 60 --no-pager
sudo bash scripts/searxng-inspect.sh
```

Optional **two** spaced upstream searches (not required for update success):

```bash
sudo bash scripts/searxng-update.sh --apply --search-check
```

Use that flag only on a planned update when engines are ready for testing; it
updates first and then checks news/general with `day`, page 1. It does not require
nonempty results to treat the API contract as working; engine warnings are printed
as counts. When engines are already blocked, omit it. A failed optional search
check reports failure but leaves an otherwise healthy installed candidate in place.

### Explicit Source/Environment Rollback

The updater prints a command with its snapshot path. Use the actual printed path:

```bash
sudo bash scripts/searxng-update.sh --rollback /var/backups/searxng/release-XXXXXXXX
```

This is a deliberate source/environment rollback, not a settings restore. It
verifies the snapshot identifies the current release, preflights the old app
against current settings, stops SearXNG, restores the exact previous paths and
checks local health. With multiple upgrades, roll back the latest first. No
snapshot/release is deleted. If rollback fails after stopping, inspect retained
paths and logs locally; do not overwrite current settings or blindly rerun an
old installer. Keep backups private: they contain your existing secret.

### Field Evidence (2026-10-05)

Operator confirmed v2.28 deployment, saved 45-day archive limit and main-domain
reports. A full ingest returned usable entries and enriched images/publication
dates, but every topic had upstream engine warnings. Follow-up bounded JSON
checks returned no results and reported rate limiting, CAPTCHA/access-denial,
HTTP errors/timeouts. These do not prove all errors are rate limits or that an
update will fix them. No production log/query/report samples are committed here.

The operator subsequently completed the v2.30 preflight and source/environment
upgrade from February's revision to `2026.10.4+d48c4b5`. Existing Python 3.12.12,
settings and unit were preserved; dependencies and local JSON health passed,
with previous paths and restricted snapshots retained. No upstream search probe
was run by that update, so engine recovery remains unverified. The operator also
confirmed Discover v2.30 in service/Admin/feed, with no ingestion active.

Since Discover v2.31, use Admin **Search Engines / Check Search Engines** for a
readable version of the earlier bounded JSON diagnostic. See USAGE.md for its
active-search effects, spacing, cooldown and interpretation; do not repeatedly
check blocked engines. Admin also shows the next automatic ingestion time.

After deploying v2.31, the operator confirmed the diagnostic report and cooldown:
the single configured instance returned ten results in each sampled category.
Bing News and DuckDuckGo contributed results; other engines reported CAPTCHA,
rate limiting, access denial or HTTP errors. This establishes partial observed
availability, not complete recovery or reliable results for every topic. The
operator left the next three-hour scheduled ingestion to run normally; its
completion and ongoing pacing effects have not yet been reported. No additional
active checks were run by the agent, and private config/log contents are not
included here.

## Historical Private-Instance Installation Recipe

I had some issues installing on Debian Trixie due to Python version mismatch (Python 3.13 being the new default),
while step by step instructions and install scripts at https://docs.searxng.org/admin/installation.html don't account for that. The following was tested on both Debian Testing, Debian Trixie and Ubuntu 22.04.5 LTS

Way I did it:

```
# Install build dependencies (as root)
su - root
apt update
apt install -y make build-essential curl git \
  libssl-dev zlib1g-dev libbz2-dev libreadline-dev \
  libsqlite3-dev libffi-dev liblzma-dev tk-dev \
  ca-certificates

# create dedicated user
useradd -r -m -d /usr/local/searxng -s /bin/bash searxng

# switch to that new user
su - searxng
```

```
# Install pyenv (as dedicated 'searxng' user)

curl https://pyenv.run | bash

# Add pyenv to shell
echo '# pyenv' >> ~/.bashrc
echo 'export PYENV_ROOT="$HOME/.pyenv"' >> ~/.bashrc
echo 'export PATH="$PYENV_ROOT/bin:$PATH"' >> ~/.bashrc
echo 'eval "$(pyenv init -)"' >> ~/.bashrc

# Reload shell
exec $SHELL

# Install Python 3.12.12
pyenv install 3.12.12
# wait a minute or two for it to finish
pyenv global 3.12.12

# Verify
python --version

# Clone SearXNG
git clone https://github.com/searxng/searxng.git ~/searxng
cd ~/searxng

# Create virtual environment
python -m venv ~/searx-venv
source ~/searx-venv/bin/activate

# Upgrade tooling
pip install --upgrade pip setuptools wheel

# Install required build dependencies
pip install msgspec typing_extensions pyyaml

# Install SearXNG
pip install -e . --no-build-isolation

# Enable JSON output
cp searx/settings.yml ~/searx-settings.yml
# nano ~/searx-settings.yml
sed -i '/^  formats:/,/^$/s/^    - html$/    - html\n    - json/' ~/searx-settings.yml
# change secret key with random string
SECRET=$(openssl rand -hex 32)
sed -i "s/^  secret_key: .*/  secret_key: \"$SECRET\"/" ~/searx-settings.yml
unset SECRET
chmod 600 ~/searx-settings.yml
# apply it all
export SEARXNG_SETTINGS_PATH=~/searx-settings.yml

# Run SearXNG manually for test
python -m searx.webapp
# access it at http://localhost:8888/
# test with: curl "http://localhost:8888/search?q=test&format=json"

# stop the test
Ctrl+C
# exit user to return to root
exit
```

```
# Create systemd service (as root)
nano /etc/systemd/system/searxng.service
```

```
[Unit]
Description=SearXNG
After=network.target

[Service]
Type=simple
User=searxng
Group=searxng
WorkingDirectory=/usr/local/searxng/searxng
Environment=SEARXNG_SETTINGS_PATH=/usr/local/searxng/searx-settings.yml
ExecStart=/usr/local/searxng/searx-venv/bin/python -m searx.webapp
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```
# fix ownership, just in case
chown -R searxng:searxng /usr/local/searxng

# apply change, enable and start service
systemctl daemon-reload
systemctl enable searxng
systemctl start searxng
systemctl status searxng

# test and exit
curl "http://localhost:8888/search?q=test&format=json"
exit
```

This setup contains everything related to SearXNG setup inside a single folder: `/usr/local/searxng/`

```
/usr/local/searxng/.pyenv               # pyenv itself
/usr/local/searxng/.pyenv/versions      # Python 3.12.12 build
/usr/local/searxng/searxng              # git clone of SearXNG
/usr/local/searxng/searx-venv           # Python virtual environment
/usr/local/searxng/searx-settings.yml   # SearXNG config
```

Only exception is system unit file: `/etc/systemd/system/searxng.service`

# Test

* with: `curl "http://localhost:8888/search?q=test&format=json&categories=general&time_range=week&pageno=1"`
* or use the exact request example above in a browser

## SearXNG uninstall

If you installed the way I described, to uninstall please follow these steps:

```
# uninstall SearXNG, run commands as root
su - root

# Stop and disable service
systemctl stop searxng
systemctl disable searxng

# Remove systemd unit
rm -f /etc/systemd/system/searxng.service
systemctl daemon-reload

# Ensure no processes are running
pkill -u searxng || true

# Remove entire installation directory
rm -rf /usr/local/searxng

# Remove dedicated user (and its home)
userdel -r searxng || true

# Remove possible lingering group
groupdel searxng || true

# Check for any leftover symlinks (optional check)
which python
which pyenv
```
