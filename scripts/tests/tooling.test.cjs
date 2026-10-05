const { test } = require('node:test');
const assert = require('node:assert/strict');
const { mkdtempSync, mkdirSync, writeFileSync, rmSync } = require('node:fs');
const { tmpdir } = require('node:os');
const path = require('node:path');
const { spawnSync } = require('node:child_process');
const root = path.resolve(__dirname, '../..');

test('SearXNG inventory uses read-only commands and does not read settings', () => {
  const dir = mkdtempSync(path.join(tmpdir(), 'discover-searx-inventory-'));
  try {
    mkdirSync(path.join(dir, 'bin'));
    mkdirSync(path.join(dir, 'searx-venv', 'bin'), {recursive:true});
    writeFileSync(path.join(dir, 'searx-settings.yml'), 'DO_NOT_DISCLOSE_SECRET');
    for (const [name,body] of Object.entries({
      id: '[ "$1" = "-u" ] && echo 1000\n[ "$1" = "-un" ] && echo searxng\nexit 0',
      systemctl: 'case "$*" in *--value*) echo searxng;; *show*) echo User=searxng;; *) exit 42;; esac',
      git: 'case "$*" in *" log "*) echo "abcdef 2026-02-14";; *" status "*) exit 0;; *) exit 43;; esac',
    })) writeFileSync(path.join(dir,'bin',name), `#!/bin/sh\n${body}\n`, {mode:0o755});
    writeFileSync(path.join(dir,'searx-venv','bin','python'), '#!/bin/sh\necho synthetic-python\n', {mode:0o755});
    const r=spawnSync('bash',['scripts/searxng-inspect.sh','--home',dir], {cwd:root,env:{...process.env,PATH:`${dir}/bin:${process.env.PATH}`},encoding:'utf8'});
    assert.equal(r.status,0,r.stderr);
    assert.match(r.stdout,/abcdef 2026-02-14/);
    assert.doesNotMatch(r.stdout,/DO_NOT_DISCLOSE_SECRET/);
    assert.match(r.stdout,/no service, source, packages or settings changed/);
  } finally { rmSync(dir,{recursive:true,force:true}); }
});

test('tool helper preserves operator PATH over fallback installations', () => {
  const dir = mkdtempSync(path.join(tmpdir(), 'discover-tools-'));
  try {
    mkdirSync(path.join(dir, 'bin'));
    writeFileSync(path.join(dir, 'bin/go'), '#!/bin/sh\necho selected-compiler\n', { mode: 0o755 });
    const r = spawnSync('bash', ['-c', 'source scripts/tool-env.sh\ngo version'], {
      cwd: root, env: { ...process.env, PATH: `${dir}/bin:${process.env.PATH}`, HOME: dir }, encoding: 'utf8',
    });
    assert.equal(r.status, 0, r.stderr);
    assert.equal(r.stdout.trim(), 'selected-compiler');
  } finally { rmSync(dir, { recursive: true, force: true }); }
});

test('security scanner failure is not reported as a successful scan', () => {
  const dir = mkdtempSync(path.join(tmpdir(), 'discover-scan-'));
  try {
    for (const [name, body] of Object.entries({ go: 'exit 0', scanner: '[ "$1" = "-version" ] && exit 0\nexit 23', npm: 'exit 0' })) {
      writeFileSync(path.join(dir, name), `#!/bin/sh\n${body}\n`, { mode: 0o755 });
    }
    const r = spawnSync('bash', ['scripts/security-check.sh'], {
      cwd: root, env: { ...process.env, PATH: `${dir}:${process.env.PATH}`, GOVULNCHECK: path.join(dir, 'scanner') }, encoding: 'utf8',
    });
    assert.equal(r.status, 23, r.stderr);
  } finally { rmSync(dir, { recursive: true, force: true }); }
});

for (const [label, output, expected] of [
  ['available update', '{"example":{"current":"1.0.0","latest":"2.0.0"}}', 0],
  ['registry error', '{"error":{"code":"ENETUNREACH"}}', 1],
  ['empty failed response', '{}', 1],
]) {
  test(`tooling report distinguishes ${label} from a clean query`, () => {
    const dir = mkdtempSync(path.join(tmpdir(), 'discover-report-'));
    try {
      for (const [name, body] of Object.entries({
        go: 'exit 0', curl: 'echo "[]"',
        npm: `[ "$1" = "--version" ] && exit 0\nprintf '%s' '${output}'\nexit 1`,
      })) {
        writeFileSync(path.join(dir, name), `#!/bin/sh\n${body}\n`, { mode: 0o755 });
      }
      const r = spawnSync('bash', ['scripts/tooling-report.sh', '--online'], {
        cwd: root, env: { ...process.env, PATH: `${dir}:${process.env.PATH}` }, encoding: 'utf8',
      });
      assert.equal(r.status, expected, r.stderr);
    } finally { rmSync(dir, { recursive: true, force: true }); }
  });
}
