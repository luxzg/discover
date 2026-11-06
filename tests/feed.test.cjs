const { test } = require('node:test');
const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');
const { webcrypto } = require('node:crypto');

function browser(file = 'feed.js', initialFetch = () => new Promise(() => {})) {
  const elements = new Map();
  function element() { return { textContent: '', innerHTML: '', value: '', dataset: {}, disabled: false, hidden: false,
    listeners: {}, addEventListener(event, fn) { this.listeners[event] = fn; },
    focus() {}, classList: { add() {}, remove() {}, toggle() {} }, querySelectorAll() { return []; } }; }
  const document = { getElementById(id) { if (!elements.has(id)) elements.set(id, element()); return elements.get(id); },
    addEventListener() {}, querySelectorAll() { return []; }, body: element() };
  const c = vm.createContext({ document, console, URL, Date, setTimeout, clearTimeout, AbortController, crypto: webcrypto, setInterval() {},
    window: { scrollTo() {} }, fetch: initialFetch });
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

for (const file of ['feed.js', 'admin.js']) {
  for (const outcome of ['401', 'success', 'json-error', 'network-error']) {
    test(`${file}: delayed startup ${outcome} cannot undo a real login`, async () => {
      let resolve, reject;
      const pending = new Promise((yes,no) => { resolve=yes; reject=no; });
      const b = browser(file, () => pending);
      b.c.fetch = async url => ({ok:true, json:async()=> url.endsWith('/login') ? {csrf_token:'new-token'} : url.endsWith('/dedupe/jobs') ? {status:'idle'} : url.endsWith('/status') ? {status:'idle',ingest:{state:{}}} : {items:[],active_days:30}});
      if (file === 'feed.js') {
        b.elements.get('userName').value='reader'; b.elements.get('userSecret').value='secret';
        await b.elements.get('userLoginBtn').listeners.click();
      } else {
        b.elements.get('secret').value='secret'; await b.elements.get('loginBtn').onclick();
      }
      assert.equal(b.eval('authenticated'), true);
      const status = b.elements.get('status').textContent;
      if (outcome === 'network-error') reject(new Error('old offline request'));
      else resolve({ok:outcome!=='401',status:outcome==='401'?401:200,json:async()=>{if(outcome==='json-error')throw new SyntaxError('old truncated body');return {csrf_token:'old-token'}}});
      await new Promise(setImmediate);
      assert.equal(b.eval('authenticated'), true);
      assert.equal(b.eval('csrfToken'),'new-token');
      assert.equal(b.elements.get('status').textContent,status);
    });
  }
}

test('cards escape titles and source URLs; unknown date has no extra separator', () => {
  const b = browser();
  const html = b.eval(`card({id:1,url:'javascript:alert(1)',title:'<img src=x>',score:10,source_domain:'example.com',sources:[{id:2,url:'https://other.example/a',source_domain:'other.example'}]})`);
  assert.doesNotMatch(html, /javascript:/);
  assert.match(html, /&lt;img src=x&gt;/);
  assert.match(html, /Other sources \(1\)/);
  assert.match(html, /score 10\.00<\/div>/);
});

test('database date is explicitly first seen, not publication', () => {
  const b = browser();
  const html = b.eval("card({id:1,title:'Unknown publication',score:5,first_seen_at:new Date().toISOString()})");
  assert.match(html,/First seen today/);
  assert.doesNotMatch(html,/Published/);
});

test('archive panel validates days and confirms mutations; domain report escapes domains', async () => {
  const b = browser('admin.js');
  b.eval('authenticated=true; setAuthUI()');
  b.c.confirm = () => true;
  b.eval("document.getElementById('archiveDays')");
  const calls = [];
  b.c.fetch = async (url,opts) => {
    calls.push({url,body:opts.body});
    return {ok:true,json:async()=>({active_days:30,stats:{candidates:3,restorable:1,archived:3,restored:1},items:[{domain:'<script>',positive:2,read:2}]})};
  };
  b.elements.get('archiveDays').value='90';
  await b.elements.get('previewArchive').onclick();
  assert.equal(calls[0].url,'/admin/api/archive?days=90');
  assert.match(b.elements.get('archiveResult').textContent,/Would newly archive 3 article rows; restore 1 age-archived rows/);
  await b.elements.get('applyArchive').onclick();
  assert.deepEqual(JSON.parse(calls[1].body),{days:90});
  assert.match(b.elements.get('archiveResult').textContent,/Archived 3; restored 1/);
  b.elements.get('archiveDays').value='1.5';
  const count=calls.length;
  await b.elements.get('applyArchive').onclick();
  assert.equal(calls.length,count);
  await b.elements.get('loadDomains').onclick();
  assert.match(b.elements.get('domainRows').innerHTML,/&lt;script&gt;/);
  b.eval('authenticated=false; setAuthUI()');
  assert.equal(b.elements.get('domainRows').innerHTML,'');
});

test('domain report explains an empty filtered result', async () => {
  const b = browser('admin.js');
  b.eval('authenticated=true; setAuthUI()');
  b.c.fetch = async () => ({ok:true,json:async()=>({items:[]})});
  await b.elements.get('loadDomains').onclick();
  assert.match(b.elements.get('domainRows').innerHTML,/No domains with at least 2 positive reads yet/);
});

test('admin engine check is explicit, CSRF protected and reports escaped observations', async () => {
  const b = browser('admin.js');
  b.eval("authenticated=true; csrfToken='fixture-csrf'; adminDataDeferred=false; setAuthUI()");
  const calls = [];
  const next = new Date(Date.now() + 3600000).toISOString();
  const check = {id:1, status:'completed', completed_at:new Date().toISOString(), report:{checked_instances:1,configured_instances:1,categories:[{instance:1,category:'news',status:'warnings',results:0,engines:[{name:'<script>',status:'captcha'}]}]}};
  b.c.fetch = async (url,opts) => {
    calls.push({url,opts});
    return {ok:true,json:async()=>url.endsWith('/dedupe/jobs') ? {status:'idle'} : url.endsWith('search-check') ? {id:1,status:'running'} : {search_check:check,ingest:{state:{next_scheduled_at:next,schedule_mode:'interval',search_check_cooldown_until:next}}}};
  };
  await b.eval('refreshStatus()');
  assert.deepEqual(calls.map(c=>c.url), ['/admin/api/dedupe/jobs','/admin/api/status']);
  assert.match(b.elements.get('nextScheduled').textContent,/Next scheduled ingestion:/);
  assert.match(b.elements.get('engineRows').innerHTML,/&lt;script&gt;: CAPTCHA/);
  assert.doesNotMatch(b.elements.get('engineRows').innerHTML,/<script>/);
  b.eval('latestEngineCheckID=0');
  b.elements.get('checkEngines').disabled=false;
  await b.elements.get('checkEngines').onclick();
  assert.equal(calls[2].url,'/admin/api/search-check');
  assert.equal(calls[2].opts.method,'POST');
  assert.equal(calls[2].opts.headers['X-CSRF-Token'],'fixture-csrf');
  assert.equal(b.elements.get('checkEngines').disabled,true);
  assert.match(b.elements.get('engineCheckState').textContent,/unknown status/);
  b.eval('authenticated=false; setAuthUI()');
  assert.equal(b.elements.get('engineRows').innerHTML,'');
  assert.equal(b.elements.get('enginePanel').hidden,true);
});

test('admin engine check rejects stale snapshots and shows scheduled work waiting', async () => {
  const b = browser('admin.js');
  b.eval('authenticated=true; latestEngineCheckID=2; adminDataDeferred=false');
  b.eval("renderEngineCheck({id:1,status:'completed',report:{}})");
  assert.equal(b.elements.get('engineCheckState').textContent,'');
  b.c.fetch = async url => ({ok:true,json:async()=>url.endsWith('/dedupe/jobs') ? {status:'idle'} : ({ingest:{state:{search_check_running:true,next_scheduled_at:new Date(Date.now()-1000).toISOString()}},search_check:{id:2,status:'running'}})});
  await b.eval('refreshStatus()');
  assert.match(b.elements.get('nextScheduled').textContent,/due, waiting for engine check/);
  assert.equal(b.elements.get('runIngest').disabled,true);
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

for (const phase of ['seen','feed']) {
  for (const action of ['dont','domain']) {
    test(`rule hide ${action} cannot overlap Load Next at ${phase}`, async () => {
      const b=browser(); b.eval('authenticated=true; currentIds=[1]; setAuthUI()');
      let ready,release;
      const entered=new Promise(resolve=>{ready=resolve});
      const pending=new Promise(resolve=>{release=resolve});
      const calls=[]; let prompts=0;
      b.c.prompt=()=> (++prompts%2 ? 'example.com' : '100');
      const item={id:2,title:'New matching card',url:'https://example.com/new',score:10};
      b.c.fetch=async url=>{
        calls.push(url);
        if(url===(phase==='seen'?'/api/feed/seen':'/api/feed')) { ready(); return pending; }
        return {ok:true,json:async()=>url==='/api/feed'?{items:[item]}:{id:'hide',status:'completed',matched_ids:[1]}};
      };
      const card={dataset:{id:'1'},remove(){},querySelector(selector){
        if(selector==='.card-link')return {href:'https://example.com/old'};
        if(selector==='.card-title')return {textContent:'Old matching card'};
        if(selector==='.menu')return {classList:{remove(){}}};
        return {textContent:'',hidden:true};
      }};
      const target={dataset:{action},closest(){return card},matches(s){return s==='[data-action]'}};
      const next=b.elements.get('nextBtn').listeners.click(); await entered;
      await b.elements.get('feed').listeners.click({target});
      release({ok:true,json:async()=>phase==='feed'?{items:[item]}:{ok:true}}); await next;
      assert.equal(prompts,0);
      assert.equal(calls.includes('/api/articles/dontshow'),false);
      assert.equal(b.elements.get('nextBtn').disabled,false);
    });
  }
}

test('Load Next waits for accepted hide and initial recovery, including helper callers', async () => {
  for (const initialization of [false,true]) {
    const b=browser(); b.eval('authenticated=true; currentIds=[1]; setAuthUI()');
    let ready,release; const entered=new Promise(r=>{ready=r}); const pending=new Promise(r=>{release=r});
    const calls=[]; b.c.cardEl={querySelector(){return null}};
    b.c.fetch=async url=>{
      calls.push(url);
      if(calls.length===1) { ready(); return pending; }
      return {ok:true,json:async()=>({items:[{id:2,title:'New',score:10}]})};
    };
    const work=b.eval(initialization?'initializeFeed()':"applyHideRequest(cardEl,1,'example.com',100)");
    await entered;
    assert.equal(b.elements.get('nextBtn').disabled,true);
    await b.elements.get('nextBtn').listeners.click();
    assert.equal(calls.length,1);
    await assert.rejects(b.eval("applyHideRequest(cardEl,1,'other.example',100)"), /Wait for/);
    release({ok:true,json:async()=>({id:'job',status:'completed',matched_ids:[1]})}); await work;
    assert.equal(b.elements.get('nextBtn').disabled,false);
  }
});

test('admin rejects unreadable/non-object successes but preserves HTTP failure status', async () => {
  for (const value of ['truncated',null,[],true,'text']) {
    const b=browser('admin.js');
    b.c.fetch=async()=>({ok:true,json:async()=>{if(value==='truncated')throw new SyntaxError('truncated');return value}});
    await assert.rejects(b.eval("call('/admin/api/rules')"));
  }
  for (const code of [401,500]) {
    const b=browser('admin.js'); b.eval('authenticated=true; setAuthUI()');
    b.c.fetch=async()=>({ok:false,status:code,json:async()=>{throw new SyntaxError('not JSON')}});
    await assert.rejects(b.eval("call('/admin/api/status')"), err=>err.status===code);
    assert.equal(b.eval('authenticated'),code!==401);
  }
});

test('malformed successful rule/topic mutation retains the editor and prior list', async () => {
  const b=browser('admin.js'); b.eval("authenticated=true; setAuthUI(); document.getElementById('topicQ').value='Retain this draft'");
  b.eval("document.getElementById('topics').innerHTML='Prior topics'");
  b.c.fetch=async()=>({ok:true,json:async()=>{throw new SyntaxError('truncated mutation reply')}});
  await b.elements.get('addTopic').onclick();
  assert.equal(b.elements.get('topicQ').value,'Retain this draft');
  assert.equal(b.elements.get('topics').innerHTML,'Prior topics');
});

for (const file of ['feed.js','admin.js']) {
  test(`${file}: successful login requires a usable CSRF token`, async()=>{
    const b=browser(file);
    b.c.fetch=async()=>({ok:true,json:async()=>({items:[],ingest:{state:{}}})});
    if(file==='feed.js'){
      b.elements.get('userName').value='reader';b.elements.get('userSecret').value='secret';
      await b.elements.get('userLoginBtn').listeners.click();
    } else {b.elements.get('secret').value='secret';await b.elements.get('loginBtn').onclick()}
    assert.equal(b.eval('authenticated'),false);
  });
}

for (const kind of ['Topic','Rule']) {
  for (const change of ['typing','toggle','select','clear','unchanged','duplicate']) {
    test(`${kind} save preserves subsequent ${change} intent`, async()=>{
      const b=browser('admin.js');b.eval('authenticated=true; setAuthUI()');
      const topic=kind==='Topic', text=topic?'topicQ':'ruleP', number=topic?'topicW':'rulePenalty', enabled=topic?'topicE':'ruleE';
      b.eval(`editing${kind}ID=7; document.getElementById('${text}').value='Original'; document.getElementById('${number}').value='2'; document.getElementById('${enabled}').checked=true`);
      let ready,release;const entered=new Promise(r=>{ready=r}), pending=new Promise(r=>{release=r});let submissions=0;
      b.c.fetch=async(_url,opts)=>{
        if(opts.method==='POST'){submissions++;ready();return pending}
        return {ok:true,json:async()=>({items:[]})};
      };
      const save=b.elements.get(`add${kind}`).onclick(); await entered;let second;
      if(change==='typing'){b.elements.get(text).value='New draft';b.elements.get(text).listeners.input?.()}
      if(change==='toggle'){b.elements.get(enabled).checked=false;b.elements.get(enabled).listeners.change?.()}
      if(change==='clear'){b.elements.get(`new${kind}`).onclick();b.elements.get(text).value='New draft';b.elements.get(text).listeners.input?.()}
      if(change==='select'){
        const dataset=topic?{editTopic:'8',topicQuery:'New draft',topicWeight:'3',topicEnabled:'false'}:{editRule:'8',rulePattern:'New draft',rulePenalty:'3',ruleEnabled:'false'};
        await b.c.document.body.listeners.click({target:{dataset,matches:s=>s===`[data-edit-${kind.toLowerCase()}]`}});
      }
      if(change==='duplicate') second=b.elements.get(`add${kind}`).onclick();
      release({ok:true,json:async()=>({ok:true})}); await save; if(second)await second;
      assert.equal(submissions,1);
      if(['typing','select','clear'].includes(change))assert.equal(b.elements.get(text).value,'New draft');
      if(change==='toggle'){assert.equal(b.elements.get(text).value,'Original');assert.equal(b.elements.get(enabled).checked,false)}
      if(change==='select')assert.equal(b.eval(`editing${kind}ID`),8);
      if(['unchanged','duplicate'].includes(change))assert.equal(b.elements.get(text).value,'');
    });
  }
}

test('admin recovers running dedupe without querying SQLite-backed panels', async()=>{
  const b=browser('admin.js');b.eval('authenticated=true; setAuthUI()');
  const calls=[];let running=true;
  b.c.fetch=async url=>{calls.push(url);return {ok:true,json:async()=>url.endsWith('/dedupe/jobs') ? {id:'job',status:running?'running':'completed',schedule:{next_scheduled_at:new Date(Date.now()-1000).toISOString()},result:{stats:{same_run_hidden:2,historical_hidden:3},dedupe_hidden_total:20}} : {items:[],ingest:{state:{}},active_days:30}}};
  await b.eval('bootstrapAfterAuth()');
  assert.deepEqual(calls,['/admin/api/dedupe/jobs']);
  assert.equal(b.elements.get('runDedupe').disabled,true);
  assert.equal(b.elements.get('runIngest').disabled,true);
  assert.equal(b.elements.get('checkEngines').disabled,true);
  assert.match(b.elements.get('nextScheduled').textContent,/waiting for dedupe/);
  await b.eval('refreshStatus()');
  assert.deepEqual(calls,['/admin/api/dedupe/jobs','/admin/api/dedupe/jobs']);
  running=false;await b.eval('refreshStatus()');
  assert.match(b.elements.get('dedupeState').textContent,/completed: hidden=5/);
  assert.equal(b.elements.get('runDedupe').disabled,false);
  assert.deepEqual(calls.slice(2),['/admin/api/dedupe/jobs','/admin/api/archive','/admin/api/topics','/admin/api/rules','/admin/api/status']);
});

test('uncertain dedupe acceptance checks memory state instead of submitting again',async()=>{
  const b=browser('admin.js');b.eval("authenticated=true; csrfToken='token'; setAuthUI()");
  const calls=[];
  b.c.fetch=async(url,opts)=>{calls.push({url,opts});if(opts.method==='POST')throw new TypeError('offline');return {ok:true,json:async()=>({id:'job',status:'running'})}};
  await b.elements.get('runDedupe').onclick();
  assert.deepEqual(calls.map(c=>c.url),['/admin/api/dedupe/jobs','/admin/api/dedupe/jobs']);
  assert.equal(calls[0].opts.headers['X-CSRF-Token'],'token');
  assert.equal(b.elements.get('runDedupe').disabled,true);
  assert.match(b.elements.get('dedupeState').textContent,/accepted/);
});

test('an old dedupe poll cannot clear a newer acceptance or auth generation',async()=>{
  const b=browser('admin.js');b.eval('authenticated=true; setAuthUI()');
  let release;const pending=new Promise(r=>{release=r});
  b.c.fetch=async(_url,opts)=>opts.method==='POST'?{ok:true,json:async()=>({id:'new',status:'running'})}:pending;
  const poll=b.eval('refreshDedupe()');
  await b.elements.get('runDedupe').onclick();
  release({ok:true,json:async()=>({status:'idle'})});await poll;
  assert.equal(b.eval('manualDedupeInFlight'),true);
  assert.equal(b.eval('lastDedupeState.id'),'new');
  let late; b.c.fetch=()=>new Promise(r=>{late=r});
  const obsolete=b.eval('refreshDedupe()');b.eval('advanceSession(); authenticated=false; setAuthUI()');
  late({ok:true,json:async()=>({id:'old',status:'running'})});
  await assert.rejects(obsolete,e=>e.staleSession===true);
  assert.equal(b.eval('manualDedupeInFlight'),false);
});

for(const id of ['applyArchive','loadDomains']) {
  test(`dedupe polling preserves pending ${id} busy state`,async()=>{
    const b=browser('admin.js');b.eval("authenticated=true; setAuthUI(); document.getElementById('archiveDays').value='30'");b.c.confirm=()=>true;
    let release,ready;const entered=new Promise(r=>{ready=r}),pending=new Promise(r=>{release=r});let requests=0;
    b.c.fetch=async (url,opts)=>{if(url.endsWith('/dedupe/jobs'))return {ok:true,json:async()=>({status:'idle'})};if((id==='applyArchive'&&opts.method==='POST')||(id==='loadDomains'&&url.endsWith('/domains'))){requests++;ready();return pending}return {ok:true,json:async()=>({items:[],ingest:{state:{}}})}};
    const first=b.elements.get(id).onclick();await entered;
    await b.eval('refreshDedupe()');
    assert.equal(b.elements.get(id).disabled,true);
    const second=b.elements.get(id).onclick();
    release({ok:true,json:async()=>({items:[],stats:{archived:0,restored:0}})});
    await first;await second;
    assert.equal(requests,1);
  });
}

for(const [event,button,alternate] of [['click',0,false],['click',0,true],['auxclick',1,false],['auxclick',1,true],['auxclick',2,false]]) {
  test(`article opens: ${event} button ${button}, alternate ${alternate}`,async()=>{
    const b=browser();b.eval("authenticated=true; csrfToken='token'");
    const calls=[];let removed=0,prevented=0;
    b.c.fetch=async(url,opts)=>{calls.push({url,opts});return {ok:true,json:async()=>({ok:true})}};
    const card={dataset:{id:'2'},remove(){removed++}};
    const link={dataset:alternate?{sourceId:'7'}:{}};
    const target={matches:()=>false,closest:s=>s==='.card'?card:s==='[data-click]'?link:null};
    const handler=b.elements.get('feed').listeners[event];assert.equal(typeof handler,'function');
    await handler({target,button,preventDefault(){prevented++}});
    assert.equal(calls.length,button===2?0:1);
    if(calls.length){assert.equal(calls[0].url,'/api/articles/click');assert.deepEqual(JSON.parse(calls[0].opts.body),{id:alternate?7:2});assert.equal(calls[0].opts.headers['X-CSRF-Token'],'token')}
    assert.equal(removed,0);assert.equal(prevented,0);
  });
}
