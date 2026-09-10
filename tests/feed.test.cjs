const { test } = require('node:test');
const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');

function browser(file = 'feed.js') {
  const elements = new Map();
  function element() { return { textContent: '', innerHTML: '', value: '', dataset: {}, disabled: false, hidden: false,
    listeners: {}, addEventListener(event, fn) { this.listeners[event] = fn; },
    classList: { add() {}, remove() {}, toggle() {} }, querySelectorAll() { return []; } }; }
  const document = { getElementById(id) { if (!elements.has(id)) elements.set(id, element()); return elements.get(id); },
    addEventListener() {}, querySelectorAll() { return []; }, body: element() };
  const c = vm.createContext({ document, console, URL, Date, setTimeout, setInterval() {},
    window: { scrollTo() {} }, fetch: () => new Promise(() => {}) });
  vm.runInContext(readFileSync(path.join(__dirname, '../internal/server/web', file), 'utf8'), c);
  return { c, elements, eval: source => vm.runInContext(source, c) };
}

test('publication label omits unknown and displays real dates', () => {
  const b = browser();
  assert.equal(b.eval("publishedLabel('0001-01-01T00:00:00Z')"), '');
  assert.equal(b.eval("publishedLabel('invalid')"), '');
  assert.equal(b.eval('publishedLabel(new Date().toISOString())'), 'today');
  assert.match(b.eval("publishedLabel('2026-01-01T10:00:00Z')"), /ago|today|yesterday/);
});

test('cards escape titles and source URLs; unknown date has no extra separator', () => {
  const b = browser();
  const html = b.eval(`card({id:1,url:'javascript:alert(1)',title:'<img src=x>',score:10,source_domain:'example.com',sources:[{id:2,url:'https://other.example/a',source_domain:'other.example'}]})`);
  assert.doesNotMatch(html, /javascript:/);
  assert.match(html, /&lt;img src=x&gt;/);
  assert.match(html, /Other sources \(1\)/);
  assert.match(html, /score 10\.00<\/div>/);
});

test('failed feed request does not trigger ingest', async () => {
  const b = browser(); const calls = [];
  b.c.fetch = async url => { calls.push(url); return { ok: false, status: 500, statusText: 'failure', json: async () => ({}) }; };
  b.eval('authenticated=true');
  await b.elements.get('nextBtn').listeners.click();
  assert.deepEqual(calls, ['/api/feed']);
  assert.equal(b.elements.get('nextBtn').disabled, false);
});

test('failed logout keeps authenticated UI and offers retry', async () => {
  const b = browser();
  b.c.fetch = async () => { throw new Error('offline'); };
  b.eval('authenticated=true');
  await b.elements.get('userLogoutBtn').listeners.click();
  assert.equal(b.eval('authenticated'), true);
  assert.match(b.elements.get('status').textContent, /retry/);
});

test('401 clears auth for any feed action', async () => {
  const b = browser(); b.eval("authenticated=true; csrfToken='old'");
  b.c.fetch = async () => ({ ok: false, status: 401, json: async () => ({ error: 'expired' }) });
  await assert.rejects(b.eval("api('/api/articles/action',{method:'POST',body:'{}'})"));
  assert.equal(b.eval('authenticated'), false);
  assert.equal(b.eval('csrfToken'), '');
});

test('admin edit saves same row ID then clears editor', async () => {
  const b = browser('admin.js'); const bodies = [];
  b.c.fetch = async (url, opts) => { if (opts.body) bodies.push(JSON.parse(opts.body)); return { ok: true, json: async () => ({ items: [] }) }; };
  b.eval("authenticated=true; editingTopicID=7; document.getElementById('topicQ'); document.getElementById('topicW'); document.getElementById('topicE')");
  b.elements.get('topicQ').value = 'fixed typo';
  b.elements.get('topicW').value = '2';
  await b.elements.get('addTopic').onclick();
  assert.equal(bodies[0].id, 7);
  assert.equal(b.eval('editingTopicID'), 0);
});
