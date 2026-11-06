const { test } = require('node:test');
const assert = require('node:assert/strict');
const { spawnSync } = require('node:child_process');
const { resolve } = require('node:path');
const root = resolve(__dirname, '../..');

test('all SQLite file families and temporary deployment artifacts are ignored', () => {
  for (const extension of ['db', 'sqlite', 'sqlite3']) {
    for (const suffix of ['', '-wal', '-shm', '-journal']) {
      const name = `synthetic-ignore-check.${extension}${suffix}`;
      const r = spawnSync('git', ['check-ignore', '--no-index', '--quiet', '--', name], {cwd: root});
      assert.equal(r.status, 0, name);
    }
  }
  for (const name of ['.discover-install.synthetic', 'config.json']) {
    assert.equal(spawnSync('git', ['check-ignore', '--no-index', '--quiet', '--', name], {cwd: root}).status, 0, name);
  }
  assert.equal(spawnSync('git', ['check-ignore', '--no-index', '--quiet', '--', 'config.example.json'], {cwd: root}).status, 1);
});
