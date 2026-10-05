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

The operator confirmed the private installation is under `/usr/local/searxng`
and likely has not been updated since February 2026. The remaining source/venv
paths below are from the historical recipe and still need server confirmation.
Source revisions/dates are a better identifier than assuming a numeric release.

From an administrator-owned Discover checkout on the server (see INSTALL.md
section 7.3), run the read-only inventory:

```bash
cd ~/discover-deployer
git pull --ff-only
sudo bash scripts/searxng-inspect.sh
```

Alternatively run `bash scripts/searxng-inspect.sh` as the `searxng` service user
if the helper is readable from that user's chosen location. The root invocation
executes Git/Python checks as the service user, never root. It reports selected
systemd location fields, Git revision/date/dirty state, Python version, dependency
consistency and settings-file presence. It does not read settings contents,
change packages, stop services, fetch Git updates or modify data. Custom layouts
can pass `--home /absolute/path --service service_name`; unexpected/missing paths
are diagnostic failures, not permission to reinstall automatically.

Share only non-sensitive revision/Python/dependency results if troubleshooting;
do not paste the complete settings or environment. An installation-specific
update helper is pending this inventory. It must preserve the existing secret,
JSON format and loopback listener, retain the prior source/environment, preflight
compatibility and verify bounded JSON queries before claiming success. Do not
run the generic upstream update script blindly against the custom pyenv layout.

Upstream's current [installation documentation](https://docs.searxng.org/admin/installation-searxng.html)
uses a virtual environment with editable package installation and minimal settings
overrides inheriting current defaults. A historical full copy of settings may
retain stale engine definitions even after a source upgrade; review it rather
than replacing it and losing local settings. Application-server migration is a
separate decision; the [Granian guide](https://docs.searxng.org/admin/installation-granian.html)
documents a production option. Keeping a private instance updated a few times
a year can refresh engine adapters but cannot guarantee result quality/dates.

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
