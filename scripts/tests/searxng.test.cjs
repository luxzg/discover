const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const { tmpdir } = require('node:os');
const { join, resolve } = require('node:path');
const { spawnSync } = require('node:child_process');
const root = resolve(__dirname, '../..');

function fixture(t, failure = 'none') {
  const dir = fs.mkdtempSync(join(tmpdir(), 'discover-searxng-'));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  const home = join(dir, 'home'), bin = join(dir, 'bin'), scripts = join(dir, 'scripts');
  for (const p of [home, bin, scripts, join(home, 'searxng'), join(home, 'searx-venv/bin')]) fs.mkdirSync(p, { recursive: true });
  fs.writeFileSync(join(home, 'searx-settings.yml'), 'private-fixture-settings');
  fs.writeFileSync(join(home, 'searxng/original'), 'old source');
  const log = join(dir, 'log'), unit = join(dir, 'service.unit');
  fs.writeFileSync(unit, 'original-unit');
  const executable = (path, body) => fs.writeFileSync(path, `#!/bin/bash\nset -eu\n${body}\n`, { mode: 0o755 });
  executable(join(bin, 'id'), 'if [[ "$*" == "-u searxng" || "${WORKER:-0}" == 1 ]]; then echo 1001; else echo 0; fi');
  // Privileged ownership/modes are simulated, not changed on the host. Path
  // symlink checks still exercise the real shell/file operations.
  executable(join(bin, 'stat'), 'if [[ "$2" == %a ]]; then echo 755; elif [[ "${@: -1}" == "$TEST_HOME" ]]; then echo 1001; else echo 0; fi');
  executable(join(bin, 'sleep'), 'exit 0');
  executable(join(bin, 'systemctl'), `echo "systemctl $*" >> "$TEST_LOG"
if [[ $1 == show ]]; then
  case "$3" in
    --property=User) echo searxng;;
    --property=WorkingDirectory) echo "$TEST_HOME/searxng";;
    --property=ExecStart) if [[ $FAILURE == launch ]]; then echo unsupported; else echo "{ argv[]=$TEST_HOME/searx-venv/bin/python -m searx.webapp ; }"; fi;;
    --property=Environment) if [[ $FAILURE == environment ]]; then echo unsupported; else echo "SEARXNG_SETTINGS_PATH=$TEST_HOME/searx-settings.yml"; fi;;
    --property=FragmentPath) echo "$TEST_UNIT";;
  esac
elif [[ $1 == start && $FAILURE == start && ! -f "$TEST_DIR/failed-start" ]]; then
  touch "$TEST_DIR/failed-start"; exit 1
fi`);
  executable(join(bin, 'git'), `echo "git $*" >> "$TEST_LOG"
if [[ $1 == clone ]]; then
  [[ $FAILURE != download ]] || exit 1
  for destination; do :; done
  mkdir -p "$destination"; echo candidate > "$destination/candidate"
elif [[ "$*" == *rev-parse* ]]; then echo 0123456789012345678901234567890123456789
elif [[ "$*" == *status* ]]; then
  if [[ $FAILURE == dirty ]]; then echo ' M fixture'; fi
elif [[ "$*" == *log* ]]; then echo 'candidate revision'
fi`);
  executable(join(home, 'searx-venv/bin/python'), `echo "python $*" >> "$TEST_LOG"
if [[ "$*" == *'-m venv'* ]]; then
  mkdir -p "$3/bin"; cp "$0" "$3/bin/python"
elif [[ "$*" == *'check.py settings'* && "$0" == *'/update-'* && $FAILURE == candidate ]]; then exit 1
elif [[ "$*" == *'check.py health'* && $(readlink -f "$0") == *'/update-'* && $FAILURE == health ]]; then exit 1
elif [[ "$*" == *'pip freeze'* && $FAILURE == settings-edit ]]; then echo operator-edit > "$TEST_HOME/searx-settings.yml"
fi`);
  executable(join(bin, 'runuser'), `echo "runuser $*" >> "$TEST_LOG"
shift 3
if [[ $1 == env ]]; then
  shift 2
  while [[ $1 == *=* ]]; do shift; done
  exec env WORKER=1 PATH="$TEST_BIN:/usr/bin:/bin" "$@"
fi
exec "$@"`);
  for (const name of ['searxng-worker.sh', 'searxng-check.py']) fs.copyFileSync(join(root, 'scripts', name), join(scripts, name));
  // Redirect privileged locations only in this disposable copy. No real unit,
  // lock, sudo, package installation or network endpoint is touched.
  let script = fs.readFileSync(join(root, 'scripts/searxng-update.sh'), 'utf8');
  script = script.replace('backup_root=/var/backups/searxng', `backup_root=${dir}/backups`)
    .replace('/run/searxng-update.XXXXXXXX', `${dir}/helpers.XXXXXXXX`)
    .replace('/etc/systemd/system/"$service".service', '"$TEST_UNIT"');
  fs.writeFileSync(join(scripts, 'searxng-update.sh'), script);
  return { dir, home, log, scripts, run: (...args) => spawnSync('bash', [join(scripts, 'searxng-update.sh'), '--home', home, ...args], {
    encoding: 'utf8', env: { ...process.env, PATH: `${bin}:${process.env.PATH}`, SUDO_UID: '1000', TEST_BIN: bin, TEST_HOME: home, TEST_LOG: log, TEST_UNIT: unit, TEST_DIR: dir, FAILURE: failure }
  }) };
}

test('SearXNG default preflight never downloads or stops service', t => {
  const f = fixture(t); const r = f.run();
  assert.equal(r.status, 0, r.stderr);
  assert.doesNotMatch(fs.readFileSync(f.log, 'utf8'), /git clone|systemctl stop|pip install/);
  assert.ok(!fs.existsSync(join(f.dir, 'backups')));
  assert.ok(!fs.readdirSync(f.dir).some(name => name.startsWith('helpers.')));
});

for (const failure of ['launch', 'environment', 'dirty', 'download', 'candidate', 'settings-edit', 'start', 'health', 'none']) {
  test(`SearXNG update ${failure}: candidate ordering, config retention and recovery`, t => {
    const f = fixture(t, failure); const r = f.run('--apply');
    const events = fs.readFileSync(f.log, 'utf8');
    assert.equal(r.status === 0, failure === 'none', r.stderr);
    assert.equal(fs.readFileSync(join(f.home, 'searx-settings.yml'), 'utf8'), failure === 'settings-edit' ? 'operator-edit\n' : 'private-fixture-settings');
    if (['launch', 'environment', 'dirty', 'download', 'candidate', 'settings-edit'].includes(failure)) {
      assert.doesNotMatch(events, /systemctl stop/);
      assert.ok(fs.existsSync(join(f.home, 'searxng/original')));
    } else {
      assert.ok(events.indexOf('git clone') < events.indexOf('systemctl stop'));
      assert.ok(events.indexOf('check.py settings') < events.indexOf('systemctl stop'));
      if (failure === 'none') {
        assert.ok(fs.lstatSync(join(f.home, 'searxng')).isSymbolicLink());
        const snapshot = r.stdout.match(/Snapshot: (.+)/)[1];
        assert.equal(fs.readFileSync(join(snapshot, 'settings.yml'), 'utf8'), 'private-fixture-settings');
        const rollback = f.run('--rollback', snapshot);
        assert.equal(rollback.status, 0, rollback.stderr);
        assert.ok(!fs.lstatSync(join(f.home, 'searxng')).isSymbolicLink());
        assert.ok(fs.existsSync(join(f.home, 'searxng/original')));
        assert.equal(fs.readFileSync(join(f.home, 'searx-settings.yml'), 'utf8'), 'private-fixture-settings');
        assert.notEqual(f.run('--rollback', snapshot).status, 0, 'second rollback must not overwrite live paths');
      } else {
        assert.ok(fs.existsSync(join(f.home, 'searxng/original')), r.stderr);
        assert.match(r.stdout, /Previous service recovered|Update failed/);
      }
    }
    assert.ok(!fs.readdirSync(f.dir).some(name => name.startsWith('helpers.')));
    // Every Python/package/application command is inside the worker, never root.
    assert.doesNotMatch(events, /runuser .*-- (python|pip)/);
  });
}

test('SearXNG candidate symlinks cannot redirect recovery into arbitrary live paths', t => {
  const f = fixture(t); const r = f.run('--apply');
  assert.equal(r.status, 0, r.stderr);
  const snapshot = r.stdout.match(/Snapshot: (.+)/)[1];
  fs.unlinkSync(join(f.home, 'searxng'));
  fs.symlinkSync(join(f.dir, 'unrelated'), join(f.home, 'searxng'));
  const rollback = f.run('--rollback', snapshot);
  assert.notEqual(rollback.status, 0);
  assert.match(rollback.stderr, /Not the active release/);
});

test('SearXNG settings checker rejects public/debug/default-secret/JSON-disabled candidates without leaking values', t => {
  const dir = fs.mkdtempSync(join(tmpdir(), 'discover-searxng-config-'));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  fs.mkdirSync(join(dir, 'searx'));
  fs.writeFileSync(join(dir, 'searx/__init__.py'), 'from .config import settings\n');
  fs.writeFileSync(join(dir, 'searx/webapp.py'), 'raise RuntimeError("preflight must not initialize app/network/cache")\n');
  fs.writeFileSync(join(dir, 'yaml.py'), 'import json\ndef safe_load(file): return json.load(file)\n');
  const settings = { general: { debug: false }, search: { formats: ['json'] }, server: { bind_address: '127.0.0.1', port: 8888, secret_key: 'synthetic-private-secret', public_instance: false } };
  fs.writeFileSync(join(dir, 'settings.yml'), '{"use_default_settings":true}');
  function check(value) {
    fs.writeFileSync(join(dir, 'searx/config.py'), `import json\nsettings=json.loads(${JSON.stringify(JSON.stringify(value))})\n`);
    return spawnSync('python3', ['-B', join(root, 'scripts/searxng-check.py'), 'settings', dir], { encoding: 'utf8', env: { ...process.env, SEARXNG_SETTINGS_PATH: join(dir, 'settings.yml') } });
  }
  assert.equal(check(settings).status, 0);
  for (const value of [
    { ...settings, general: { debug: true } },
    { ...settings, search: { formats: ['html'] } },
    { ...settings, server: { ...settings.server, bind_address: '0.0.0.0' } },
    { ...settings, server: { ...settings.server, public_instance: true } },
    { ...settings, server: { ...settings.server, secret_key: 'ultrasecretkey' } }
  ]) {
    const r = check(value); assert.notEqual(r.status, 0); assert.doesNotMatch(r.stdout + r.stderr, /synthetic-private-secret|ultrasecretkey/);
  }
});
