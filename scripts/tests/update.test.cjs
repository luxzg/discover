const { test } = require('node:test');
const assert = require('node:assert/strict');
const { mkdtempSync, writeFileSync, readFileSync, mkdirSync, rmSync } = require('node:fs');
const { tmpdir } = require('node:os');
const { join, resolve } = require('node:path');
const { spawnSync } = require('node:child_process');
const root = resolve(__dirname, '../..');

test('SSH arguments are validated before connecting', () => {
  const r = spawnSync('bash', ['scripts/run_remote_update.sh', '-ip', '-oProxyCommand=bad', '-user', 'operator'], { cwd: root, encoding: 'utf8' });
  assert.notEqual(r.status, 0);
  assert.match(r.stderr, /Invalid SSH/);
});

test('backup refuses privileged ownership transfers before creating snapshots', t => {
  const dir = mkdtempSync(join(tmpdir(), 'discover-backup-'));
  t.after(() => rmSync(dir, { recursive: true, force: true }));
  writeFileSync(join(dir, 'id'), '#!/bin/bash\nif [[ "$*" == "-u" ]]; then echo 0; elif [[ "$*" == "-un" ]]; then echo root; else echo 65534; fi\n', { mode: 0o755 });
  writeFileSync(join(dir, 'sqlite3'), '#!/bin/bash\nexit 99\n', { mode: 0o755 });
  writeFileSync(join(dir, 'fixture.db'), 'fixture');
  const r = spawnSync('bash', ['scripts/backup-database.sh', '--database', join(dir, 'fixture.db'), '--backup-dir', join(dir, 'backups'), '--owner', 'nobody'], {
    cwd: root, encoding: 'utf8', env: { ...process.env, PATH: `${dir}:${process.env.PATH}` }
  });
  assert.notEqual(r.status, 0);
  assert.match(r.stderr, /ownership transfers are not supported/);
});

test('wrapper sends trusted local orchestration, not a root command for a service-owned script', t => {
  const dir = mkdtempSync(join(tmpdir(), 'discover-wrapper-'));
  t.after(() => rmSync(dir, { recursive: true, force: true }));
  const log = join(dir, 'args');
  writeFileSync(join(dir, 'ssh'), '#!/bin/bash\nprintf "%s\\n" "$@" > "$TEST_LOG"\n', { mode: 0o755 });
  const r = spawnSync('bash', ['scripts/run_remote_update.sh', '-ip', 'example.test', '-user', 'operator'], {
    cwd: root, encoding: 'utf8', env: { ...process.env, PATH: `${dir}:${process.env.PATH}`, TEST_LOG: log }
  });
  assert.equal(r.status, 0, r.stderr);
  const args = readFileSync(log, 'utf8');
  assert.match(args, /operator@example.test/);
  assert.match(args, /sudo bash -c/);
  assert.ok(args.indexOf('pull --ff-only') < args.indexOf('systemctl stop'));
  assert.doesNotMatch(args, /exec bash \/home\/discover/);
});

for (const failure of ['build', 'backup', 'start', 'none']) {
  test(`deployment ${failure}: ordering and recovery with isolated command doubles`, t => {
    const dir = mkdtempSync(join(tmpdir(), 'discover-deploy-'));
    t.after(() => rmSync(dir, { recursive: true, force: true }));
    const bin = join(dir, 'bin'); const app = join(dir, 'app');
    mkdirSync(bin); mkdirSync(app); mkdirSync(join(app, 'scripts'));
    const log = join(dir, 'log');
    const executable = (path, body) => writeFileSync(path, `#!/bin/bash\n${body}\n`, { mode: 0o755 });
    executable(join(bin, 'id'), 'echo 0');
    executable(join(bin, 'flock'), 'exit 0');
    executable(join(bin, 'sleep'), 'exit 0');
    executable(join(bin, 'runuser'), 'shift 3\ncase "$1" in\n git) echo pull >> "$TEST_LOG"; exit 0;;\n env) echo build >> "$TEST_LOG"; [[ "$FAILURE" != build ]]; exit $?;;\n *) exec "$@";;\nesac');
    executable(join(bin, 'chown'), 'exit 0');
    executable(join(bin, 'sqlite3'), 'if [[ "$*" == *quick_check* ]]; then echo ok; else echo backup >> "$TEST_LOG"; [[ "$FAILURE" != backup ]]; fi');
    executable(join(bin, 'journalctl'), 'exit 0');
    executable(join(bin, 'systemctl'), 'echo "systemctl $*" >> "$TEST_LOG"\nif [[ "$FAILURE" == start && "$1" == start && ! -f "$APP_DIR/failed" ]]; then touch "$APP_DIR/failed"; exit 1; fi');
    executable(join(app, 'discover'), 'echo old');
    executable(join(app, 'discover.next'), 'if [[ "$1" == --database-path ]]; then echo "$APP_DIR/test.db"; else echo new; fi');
    writeFileSync(join(app, 'config.json'), '{}'); writeFileSync(join(app, 'test.db'), 'fixture');
    // Substitute only the lock location in the test copy, never touch /run/lock.
    const script = readFileSync(join(root, 'scripts/deploy.sh'), 'utf8').replace('/run/lock/discover-deploy.lock', join(dir, 'lock'));
    writeFileSync(join(dir, 'deploy.sh'), script);
    const r = spawnSync('bash', [join(dir, 'deploy.sh')], { encoding: 'utf8', env: { ...process.env, APP_DIR: app, BACKUP_ROOT: join(dir, 'backups'), PATH: `${bin}:${process.env.PATH}`, TEST_LOG: log, FAILURE: failure } });
    const events = readFileSync(log, 'utf8');
    if (failure === 'build') { assert.notEqual(r.status, 0); assert.doesNotMatch(events, /systemctl stop/); }
    else {
      assert.ok(events.indexOf('build') < events.indexOf('systemctl stop'));
      assert.ok(events.indexOf('systemctl stop') < events.indexOf('backup'));
      assert.match(events, /systemctl start/);
      assert.equal(r.status === 0, failure === 'none', r.stderr);
      assert.match(readFileSync(join(app, 'discover'), 'utf8'), failure === 'none' ? /echo new/ : /echo old/);
    }
    assert.equal(readFileSync(join(app, 'test.db'), 'utf8'), 'fixture');
  });
}
