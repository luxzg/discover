const { mkdtemp, readFile, writeFile, rm } = require('node:fs/promises');
const { tmpdir } = require('node:os');
const path = require('node:path');
const { createServer } = require('node:http');
const { spawn } = require('node:child_process');
const { randomBytes } = require('node:crypto');
const { once } = require('node:events');

const root = path.resolve(__dirname, '../..');
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
async function listen(server) {
  server.listen(0, '127.0.0.1');
  await once(server, 'listening');
  return `http://127.0.0.1:${server.address().port}`;
}

async function startFixture() {
  if (!process.env.DISCOVER_TEST_BINARY) throw new Error('Use scripts/test-browser.sh');
  const dir = await mkdtemp(path.join(tmpdir(), 'discover-browser-'));
  const secret = randomBytes(24).toString('hex');
  const admin = randomBytes(24).toString('hex');
  const thumbnail = 'data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7';
  const results = Array.from({ length: 14 }, (_, i) => ({
    url: `https://source${i}.example.test/story`,
    title: i < 2 ? 'Battery research reaches a synthetic milestone' : `Fixture ${i} news for browser validation`,
    content: 'Synthetic news, no private data.',
    thumbnail: i % 3 === 0 ? thumbnail : '',
    publishedDate: i % 2 === 0 ? new Date().toISOString() : '',
    score: i < 2 ? 100 - i : 20 - i / 10,
    engines: ['fixture'],
  }));
  const requests = [];
  const upstream = createServer((req, res) => {
    requests.push(new URL(req.url, 'http://fixture'));
    res.setHeader('Content-Type', 'application/json');
    res.end(JSON.stringify({ results, unresponsive_engines: [] }));
  });
  let child, stopping;
  let logs = '';
  const close = () => stopping ||= (async () => {
    if (child?.pid && child.exitCode === null && child.signalCode === null) {
      const exited = once(child, 'exit');
      child.kill('SIGTERM');
      const force = setTimeout(() => child.kill('SIGKILL'), 12000);
      await exited;
      clearTimeout(force);
    }
    upstream.closeAllConnections();
    if (upstream.listening) await new Promise(resolve => upstream.close(resolve));
    await rm(dir, { recursive: true, force: true });
  })();
  const interrupt = () => { void close(); };
  process.once('SIGTERM', interrupt);
  process.once('SIGINT', interrupt);
  try {
    const upstreamURL = await listen(upstream);
    // Reserve a free loopback port; never attach to or reuse an existing service.
    const reservation = createServer();
    const url = await listen(reservation);
    await new Promise(resolve => reservation.close(resolve));
    const config = JSON.parse(await readFile(path.join(root, 'config.example.json'), 'utf8'));
    Object.assign(config, {
      listen_address: new URL(url).host, enable_tls: false,
      user_name: 'fixture-reader', user_secret: secret, admin_secret: admin,
      admin_bind_cidrs: ['127.0.0.1/32'], database_path: path.join(dir, 'fixture.db'),
      searxng_instances: [upstreamURL], per_query_delay_seconds: 0, per_query_jitter_seconds: 0,
      ingest_interval_minutes: 1440, thumbnail_refresh_max_per_run: 0,
      default_batch_size: 5, feed_min_score: 1,
    });
    const configPath = path.join(dir, 'config.json');
    await writeFile(configPath, JSON.stringify(config), { mode: 0o600 });
    child = spawn(process.env.DISCOVER_TEST_BINARY, ['-config', configPath], { cwd: dir, stdio: ['ignore', 'pipe', 'pipe'] });
    child.stdout.on('data', b => { logs = (logs + b).slice(-12000); });
    child.stderr.on('data', b => { logs = (logs + b).slice(-12000); });
    let spawnError;
    child.on('error', err => { spawnError = err; });
    async function request(route, options = {}) {
      return fetch(url + route, { ...options, signal: AbortSignal.timeout(3000) });
    }
    let ready = false;
    const startupDeadline = Date.now() + 10000;
    while (Date.now() < startupDeadline) {
      if (spawnError) throw spawnError;
      if (child.exitCode !== null) throw new Error(`Fixture exited: ${logs}`);
      try { if ((await request('/')).ok) { ready = true; break; } } catch {}
      await sleep(50);
    }
    if (!ready) throw new Error(`Fixture did not start: ${logs}`);
    const login = await request('/admin/api/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ secret: admin }) });
    if (!login.ok) throw new Error('Fixture admin login failed');
    const { csrf_token: csrf } = await login.json();
    const headers = { Cookie: login.headers.getSetCookie().map(c => c.split(';')[0]).join('; '), 'X-CSRF-Token': csrf, 'Content-Type': 'application/json' };
    for (const [route, body] of [['/admin/api/topics', { query: 'fixture news', weight: 10, enabled: true }], ['/admin/api/ingest', {}]]) {
      if (!(await request(route, { method: 'POST', headers, body: JSON.stringify(body) })).ok) throw new Error('Fixture seeding failed');
    }
    let seeded = false;
    const seedDeadline = Date.now() + 10000;
    while (Date.now() < seedDeadline) {
      const response = await request('/admin/api/status', { headers });
      const state = await response.json();
      if (response.ok && !state.ingest.state.running && state.counts.unread > 0) { seeded = true; break; }
      await sleep(50);
    }
    if (!seeded) throw new Error(`Fixture ingestion failed: ${logs}`);
    return { url, secret, admin, requests, close: async () => {
      process.removeListener('SIGTERM', interrupt);
      process.removeListener('SIGINT', interrupt);
      await close();
    } };
  } catch (err) {
    process.removeListener('SIGTERM', interrupt);
    process.removeListener('SIGINT', interrupt);
    await close();
    throw err;
  }
}
module.exports = { startFixture };
