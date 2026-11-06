const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { tmpdir } = require('node:os');
const { spawnSync } = require('node:child_process');

test('shared browser helper uses selected installation, not project node_modules', t => {
  const dir = fs.mkdtempSync(path.join(tmpdir(), 'discover-shared-tools-'));
  t.after(() => fs.rmSync(dir, {recursive:true, force:true}));
  fs.writeFileSync(path.join(dir, 'package.json'), JSON.stringify({name:'playwright', version:'99.0.0-fixture'}));
  fs.writeFileSync(path.join(dir, 'chromium'), 'synthetic executable presence only');
  fs.writeFileSync(path.join(dir, 'test.js'), `exports.chromium={executablePath:()=>${JSON.stringify(path.join(dir, 'chromium'))}};`);
  const run = module => spawnSync('node', ['scripts/browser-tools.cjs'], {encoding:'utf8', env:{...process.env, PLAYWRIGHT_MODULE:module}});
  let r = run(dir);
  assert.equal(r.status, 0, r.stderr);
  assert.match(r.stdout, /99.0.0-fixture/);
  fs.unlinkSync(path.join(dir, 'chromium'));
  r = run(dir); assert.notEqual(r.status, 0); assert.match(r.stderr, /Chromium is missing/);
  r = run('relative-tools'); assert.notEqual(r.status, 0); assert.match(r.stderr, /absolute/);
});
