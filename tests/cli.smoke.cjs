const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const net = require('node:net');
const { spawn, spawnSync } = require('node:child_process');
const binary = path.resolve(__dirname, '../discover');
const example = JSON.parse(fs.readFileSync(path.resolve(__dirname, '../config.example.json')));

function fixture(t) {
  const cwd = fs.mkdtempSync(path.join(os.tmpdir(), 'discover-cli-'));
  t.after(() => fs.rmSync(cwd, { recursive: true, force: true }));
  const config = { ...example, enable_tls: false, user_secret: 'test-only-reader-secret', admin_secret: 'test-only-admin-secret' };
  const save = () => fs.writeFileSync(path.join(cwd, 'fixture.json'), JSON.stringify(config));
  return { cwd, config, save };
}

test('built version and config checks never create config or database', t => {
  const f = fixture(t);
  let r = spawnSync(binary, ['--version'], { cwd: f.cwd, encoding: 'utf8' });
  assert.equal(r.status, 0, r.stderr);
  assert.match(r.stdout, /^v\d+\.\d+ \(commit=[a-f0-9]+, built=/);
  assert.deepEqual(fs.readdirSync(f.cwd), []);
  r = spawnSync(binary, ['--check-config'], { cwd: f.cwd, encoding: 'utf8' });
  assert.notEqual(r.status, 0);
  assert.deepEqual(fs.readdirSync(f.cwd), []);
  f.save();
  const original = fs.readFileSync(path.join(f.cwd, 'fixture.json'), 'utf8');
  r = spawnSync(binary, ['--check-config', '-config', 'fixture.json'], { cwd: f.cwd, encoding: 'utf8' });
  assert.equal(r.status, 0, r.stderr);
  assert.deepEqual(fs.readdirSync(f.cwd), ['fixture.json']);
  assert.equal(fs.readFileSync(path.join(f.cwd, 'fixture.json'), 'utf8'), original);
});

test('listener failure exits nonzero for systemd Restart=on-failure', async t => {
  const f = fixture(t);
  const listener = net.createServer();
  await new Promise((resolve, reject) => listener.once('error', reject).listen(0, '127.0.0.1', resolve));
  t.after(() => listener.close());
  f.config.listen_address = `127.0.0.1:${listener.address().port}`;
  f.save();
  const child = spawn(binary, ['-config', 'fixture.json'], { cwd: f.cwd, stdio: ['ignore', 'ignore', 'pipe'] });
  let stderr = '';
  child.stderr.on('data', chunk => { stderr += chunk; });
  const timeout = setTimeout(() => child.kill('SIGKILL'), 10000);
  t.after(() => clearTimeout(timeout));
  const [code, signal] = await new Promise((resolve, reject) => {
    child.once('error', reject);
    child.once('exit', (code, signal) => resolve([code, signal]));
  });
  assert.equal(signal, null, stderr);
  assert.equal(code, 1, stderr);
  assert.match(stderr, /HTTP server stopped:/);
});
