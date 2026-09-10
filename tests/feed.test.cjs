const { test } = require('node:test');
const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');
const { webcrypto } = require('node:crypto');

function browser(file = 'feed.js') {
  const elements = new Map();
  function element() { return { textContent: '', innerHTML: '', value: '', dataset: {}, disabled: false, hidden: false,
    listeners: {}, addEventListener(event, fn) { this.listeners[event] = fn; },
    classList: { add() {}, remove() {}, toggle() {} }, querySelectorAll() { return []; } }; }
  const document = { getElementById(id) { if (!elements.has(id)) elements.set(id, element()); return elements.get(id); },
    addEventListener() {}, querySelectorAll() { return []; }, body: element() };
  const c = vm.createContext({ document, console, URL, Date, setTimeout, clearTimeout, AbortController, crypto: webcrypto, setInterval() {},
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

test('hide closes menu, shows local progress, polls and returns matches only after completion', async () => {
  const b = browser(); b.eval('authenticated=true; currentIds=[1,2]; waitForHidePoll=async()=>{}');
  const local = {textContent:'',hidden:true}; let closed=false; const calls=[];
  b.c.cardEl={querySelector(selector){return selector==='.menu'?{classList:{remove(){closed=true}}}:local}};
  b.c.fetch=async(url,opts)=>{
    calls.push(url);
    assert.equal(closed,true);
    if(opts.method==='POST'){
      assert.match(local.textContent,/Submitting/);
      assert.equal(JSON.parse(opts.body).request_id.length,32);
      return {ok:true,json:async()=>({id:'job',status:'running'})};
    }
    assert.match(local.textContent,/Applying hide/);
    return {ok:true,json:async()=>({id:'job',status:'completed',matched_ids:[1,2]})};
  };
  const ids=await b.eval("applyHideRequest(cardEl,1,'example.com',100)");
  assert.deepEqual(Array.from(ids),[1,2]);
  assert.equal(calls.length,2);
  assert.equal(b.elements.get('nextBtn').disabled,false);
});

test('uncertain hide retry reuses the request ID and does not claim success', async () => {
  const b=browser();b.eval('authenticated=true');
  b.c.cardEl={querySelector(){return null}};
  const ids=[];
  b.c.fetch=async(_url,opts)=>{ids.push(JSON.parse(opts.body).request_id);throw new TypeError('Failed to fetch')};
  await assert.rejects(b.eval("applyHideRequest(cardEl,1,'example.com',100)"),/may still be running/);
  await assert.rejects(b.eval("applyHideRequest(cardEl,1,'example.com',100)"),/may still be running/);
  assert.equal(ids.length,2);assert.equal(ids[0],ids[1]);
});

test('unreadable accepted hide responses retain their retry ID', async () => {
  for (const name of ['AbortError', 'SyntaxError']) {
    const b=browser(); b.eval('authenticated=true');
    b.c.cardEl={querySelector(){return null}};
    const ids=[];
    b.c.fetch=async(_url,opts)=>{
      ids.push(JSON.parse(opts.body).request_id);
      return {ok:true,status:202,json:async()=>{const err=new Error('interrupted body');err.name=name;throw err}};
    };
    await assert.rejects(b.eval("applyHideRequest(cardEl,1,'example.com',100)"),/may still be running/);
    await assert.rejects(b.eval("applyHideRequest(cardEl,1,'example.com',100)"),/may still be running/);
    assert.equal(ids[0],ids[1]);
  }
});

test('malformed hide status does not clear the saved request', async () => {
  const b=browser();b.eval('authenticated=true');
  b.c.cardEl={hideRequest:{request_id:'saved'},querySelector(){return null}};
  await assert.rejects(b.eval('waitForHide({},cardEl)'),/Could not confirm/);
  assert.equal(b.c.cardEl.hideRequest.request_id,'saved');
});

test('a delayed hide-poll 401 cannot clear a newer login', async () => {
  const b=browser();b.eval("authenticated=true; csrfToken='old'; waitForHidePoll=async()=>{}");
  let deliver, entered;
  const ready=new Promise(resolve=>{entered=resolve});
  b.c.fetch=()=>new Promise(resolve=>{deliver=resolve;entered()});
  const pending=b.eval("waitForHide({id:'job',status:'running'},null)");
  await ready;
  b.eval("sessionGeneration+=2; authenticated=true; csrfToken='new'");
  deliver({ok:false,status:401,json:async()=>({error:'old session expired'})});
  await assert.rejects(pending,/Session changed/);
  assert.equal(b.eval('authenticated'),true);
  assert.equal(b.eval('csrfToken'),'new');
});

test('Hide Domain click confirms completion before removal and reports uncertain failures on the card', async () => {
  for (const offline of [false,true]) {
    const b=browser();b.eval('authenticated=true; currentIds=[1]; waitForHidePoll=async()=>{}');
    const prompts=['example.com','100'];b.c.prompt=()=>prompts.shift();
    let removed=false,closed=false;
    const local={textContent:'',hidden:true};
    const card={dataset:{id:'1'},remove(){removed=true},querySelector(selector){
      if(selector==='.card-link') return {href:'https://example.com/a'};
      if(selector==='.menu') return {classList:{remove(){closed=true}}};
      return local;
    }};
    const target={dataset:{action:'domain'},disabled:false,closest(){return card},matches(selector){return selector==='[data-action]'}};
    b.c.fetch=async(_url,opts)=>{
      assert.equal(removed,false);assert.equal(closed,true);
      if(offline) throw new TypeError('offline');
      return {ok:true,json:async()=>opts.method==='POST'?{id:'job',status:'running'}:{id:'job',status:'completed',matched_ids:[1]}};
    };
    await b.elements.get('feed').listeners.click({target});
    assert.equal(removed,!offline);
    assert.equal(target.disabled,false);
    if(offline) assert.match(local.textContent,/Hide status: Could not confirm hide completion/);
    else assert.deepEqual(Array.from(b.eval('currentIds')),[]);
  }
});
