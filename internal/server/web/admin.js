const statusEl = document.getElementById('status');
const ingestStateEl = document.getElementById('ingestState');
const countsEl = document.getElementById('counts');
const secretEl = document.getElementById('secret');
const runIngestBtn = document.getElementById('runIngest');
const runDedupeBtn = document.getElementById('runDedupe');
const loginBtn = document.getElementById('loginBtn');
const logoutBtn = document.getElementById('logoutBtn');
const topicsPanel = document.getElementById('topicsPanel');
const rulesPanel = document.getElementById('rulesPanel');
const ingestionPanel = document.getElementById('ingestionPanel');
const countsPanel = document.getElementById('countsPanel');
const checkEnginesBtn = document.getElementById('checkEngines');
let engineCheckInFlight = false;
let latestEngineCheckID = 0;

let manualIngestInFlight = false;
let manualDedupeInFlight = false;
let dedupeSubmitInFlight = false;
let dedupeRevision = 0;
let lastDedupeState = {status:'idle'};
let adminDataDeferred = true;
let authenticated = false;
let csrfToken = '';
let editingTopicID = 0;
let editingRuleID = 0;
let topicEditorRevision = 0;
let ruleEditorRevision = 0;
let topicSaveInFlight = false;
let ruleSaveInFlight = false;
let archiveInFlight = false;
let domainReportInFlight = false;
let lastSchedulerState = {};
let requestedRunID = 0;
let statusInFlight = false;
let sessionGeneration = 0;
let authInFlight = false;
const sessionRequests = new Set();

function staleSessionError() {
  const err = new Error('Session changed; ignoring an old response.');
  err.staleSession = true;
  return err;
}

function advanceSession() {
  sessionGeneration++;
  for (const controller of sessionRequests) controller.abort();
  statusInFlight = false;
  manualIngestInFlight = false;
  manualDedupeInFlight = false;
  dedupeSubmitInFlight = false;
  dedupeRevision++;
  lastDedupeState = {status:'idle'};
  adminDataDeferred = true;
  engineCheckInFlight = false;
  topicSaveInFlight = false;
  ruleSaveInFlight = false;
  archiveInFlight = false;
  domainReportInFlight = false;
  lastSchedulerState = {};
}

async function sessionFetch(url, opts) {
  const generation = sessionGeneration;
  const controller = new AbortController();
  const abort = () => controller.abort();
  if (opts.signal?.aborted) abort();
  else opts.signal?.addEventListener('abort', abort, {once:true});
  sessionRequests.add(controller);
  try {
    const response = await fetch(url, {...opts, signal:controller.signal});
    if (generation !== sessionGeneration) throw staleSessionError();
    return response;
  } catch (err) {
    if (generation !== sessionGeneration) throw staleSessionError();
    throw err;
  } finally {
    sessionRequests.delete(controller);
    opts.signal?.removeEventListener('abort', abort);
  }
}

function nowStamp() {
  const d = new Date();
  return `${d.toLocaleDateString()} ${d.toLocaleTimeString()}`;
}

function status(msg) {
  statusEl.textContent = `${nowStamp()} ${msg}`;
}

function escAttr(v) {
  return String(v || '')
    .replace(/&/g, '&amp;')
    .replace(/"/g, '&quot;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');
}

function escHtml(v) {
  return String(v || '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');
}

async function call(url, opts = {}) {
  const generation = sessionGeneration;
  const headers = { ...(opts.headers || {}) };
  const method = String(opts.method || 'GET').toUpperCase();
  const hasBody = typeof opts.body !== 'undefined';
  if (hasBody && !headers['Content-Type']) {
    headers['Content-Type'] = 'application/json';
  }
  if (csrfToken && method !== 'GET' && method !== 'HEAD' && method !== 'OPTIONS') {
    headers['X-CSRF-Token'] = csrfToken;
  }
  const r = await sessionFetch(url, { ...opts, headers });
  let j = {}, bodyError;
  try { j = await r.json(); } catch (err) { bodyError = err; }
  if (generation !== sessionGeneration) throw staleSessionError();
  if (!r.ok) {
    if (r.status === 401) {
      authenticated = false;
      csrfToken = '';
      advanceSession();
      manualIngestInFlight = false;
      setAuthUI();
    }
    const err = new Error(j?.error || r.statusText || `HTTP ${r.status}`);
    err.status = r.status;
    throw err;
  }
  if (bodyError) throw new SyntaxError('Unreadable server response; completion is unconfirmed. Reload to verify before retrying.');
  if (!j || typeof j !== 'object' || Array.isArray(j)) throw new Error('Invalid server response; expected an object.');
  if (['/admin/api/login','/admin/api/session'].includes(url) && (typeof j.csrf_token !== 'string' || !j.csrf_token.trim())) {
    throw new Error('Invalid session response; reload before retrying sign-in.');
  }
  return j;
}

function setAuthUI() {
  const running = manualIngestInFlight || Boolean(lastSchedulerState.running);
  const checking = engineCheckInFlight || Boolean(lastSchedulerState.search_check_running);
  const cooling = Date.parse(lastSchedulerState.cooldown_until) > Date.now();
  const checkCooling = Date.parse(lastSchedulerState.search_check_cooldown_until) > Date.now();
  loginBtn.disabled = authenticated || authInFlight;
  logoutBtn.disabled = !authenticated || authInFlight;
  secretEl.disabled = authenticated || authInFlight;
  runIngestBtn.disabled = !authenticated || authInFlight || running || checking || cooling || manualDedupeInFlight || dedupeSubmitInFlight;
  runDedupeBtn.disabled = !authenticated || authInFlight || manualDedupeInFlight || dedupeSubmitInFlight || running || checking || archiveInFlight || topicSaveInFlight || ruleSaveInFlight;
  topicsPanel.hidden = !authenticated;
  rulesPanel.hidden = !authenticated;
  ingestionPanel.hidden = !authenticated;
  countsPanel.hidden = !authenticated;
  document.getElementById('archivePanel').hidden = !authenticated;
  document.getElementById('domainsPanel').hidden = !authenticated;
  document.getElementById('enginePanel').hidden = !authenticated;
  checkEnginesBtn.disabled = !authenticated || authInFlight || checking || checkCooling || running || manualDedupeInFlight || dedupeSubmitInFlight;
  for (const id of ['addTopic','addRule','previewArchive','applyArchive','loadDomains']) {
    document.getElementById(id).disabled = !authenticated || authInFlight || manualDedupeInFlight || dedupeSubmitInFlight;
  }
  document.getElementById('addTopic').disabled ||= topicSaveInFlight;
  document.getElementById('addRule').disabled ||= ruleSaveInFlight;
  document.getElementById('applyArchive').disabled ||= archiveInFlight;
  document.getElementById('loadDomains').disabled ||= domainReportInFlight;
  if (!authenticated) {
    latestEngineCheckID = 0;
    document.getElementById('engineRows').innerHTML = '';
    document.getElementById('engineCheckState').textContent = '';
    document.getElementById('domainRows').innerHTML = '';
    document.getElementById('archiveResult').textContent = '';
    document.getElementById('dedupeState').textContent = '';
  }
}

loginBtn.onclick = async () => {
  if (authInFlight || authenticated) return;
  const secret = secretEl.value.trim();
  if (!secret) {
    status('enter admin secret first');
    return;
  }
  authInFlight = true;
  advanceSession();
  setAuthUI();
  try {
    const res = await call('/admin/api/login', { method: 'POST', body: JSON.stringify({ secret }) });
    csrfToken = res.csrf_token || '';
    authenticated = true;
    setAuthUI();
    secretEl.value = '';
    status('signed in');
    await bootstrapAfterAuth();
  } catch (e) {
    if (e.staleSession) return;
    status(`sign in failed: ${e.message}`);
  } finally {
    authInFlight = false;
    setAuthUI();
  }
};

logoutBtn.onclick = async () => {
  if (authInFlight || !authenticated) return;
  authInFlight = true;
  advanceSession();
  setAuthUI();
  try {
    await call('/admin/api/logout', { method: 'POST', body: JSON.stringify({}) });
  } catch (e) {
    if (e.staleSession) return;
    if (e.status !== 401) { status(`sign out failed: ${e.message}; please retry`); return; }
  } finally {
    authInFlight = false;
    setAuthUI();
  }
  authenticated = false;
  csrfToken = '';
  manualIngestInFlight = false;
  manualDedupeInFlight = false;
  setAuthUI();
  document.getElementById('topics').innerHTML = '';
  document.getElementById('rules').innerHTML = '';
  ingestStateEl.textContent = '';
  countsEl.textContent = '';
  status('signed out');
};

async function loadTopics() {
  const j = await call('/admin/api/topics');
  const stats = j.topic_stats || {};
  document.getElementById('topics').innerHTML = (j.items || []).map(t => {
    const s = stats[String(t.id)] || {};
    const unread = Number(s.unread || 0);
    const total = Number(s.total || 0);
    return `<li>${escHtml(t.query)} (w=${t.weight}, enabled=${t.enabled}, unread=${unread}, total=${total}) <button data-edit-topic="${t.id}" data-topic-query="${escAttr(t.query)}" data-topic-weight="${t.weight}" data-topic-enabled="${t.enabled}">edit</button> <button data-del-topic="${t.id}">delete</button></li>`;
  }).join('');
}

async function loadRules() {
  const j = await call('/admin/api/rules');
  document.getElementById('rules').innerHTML = (j.items || []).map(r => `<li>${escHtml(r.pattern)} (-${r.penalty}, enabled=${r.enabled}, applied=${Number(r.applied_count || 0)}) <button data-edit-rule="${r.id}" data-rule-pattern="${escAttr(r.pattern)}" data-rule-penalty="${r.penalty}" data-rule-enabled="${r.enabled}">edit</button> <button data-del-rule="${r.id}">delete</button></li>`).join('');
}

function topicDraft() {
  return {id:editingTopicID, query:document.getElementById('topicQ').value, weight:Number(document.getElementById('topicW').value || 1), enabled:Boolean(document.getElementById('topicE').checked)};
}
function ruleDraft() {
  return {id:editingRuleID, pattern:document.getElementById('ruleP').value, penalty:Number(document.getElementById('rulePenalty').value || 5), enabled:Boolean(document.getElementById('ruleE').checked)};
}
for (const id of ['topicQ','topicW','topicE']) {
  document.getElementById(id).addEventListener('input', () => topicEditorRevision++);
  document.getElementById(id).addEventListener('change', () => topicEditorRevision++);
}
for (const id of ['ruleP','rulePenalty','ruleE']) {
  document.getElementById(id).addEventListener('input', () => ruleEditorRevision++);
  document.getElementById(id).addEventListener('change', () => ruleEditorRevision++);
}

document.getElementById('addTopic').onclick = async () => {
  if (topicSaveInFlight) return;
  if (!authenticated || authInFlight || manualDedupeInFlight || dedupeSubmitInFlight) {
    status('sign in first');
    return;
  }
  const generation = sessionGeneration, revision = topicEditorRevision, submitted = JSON.stringify(topicDraft());
  topicSaveInFlight = true;
  setAuthUI();
  try {
    await call('/admin/api/topics', { method: 'POST', body: submitted });
    if (generation === sessionGeneration && revision === topicEditorRevision && submitted === JSON.stringify(topicDraft())) resetTopicEditor();
    status('topic saved');
    try { await loadTopics(); }
    catch (e) { if (!e.staleSession) status(`topic saved; list refresh failed: ${e.message}`); }
  } catch (e) {
    if (e.staleSession) return;
    status(`topic save failed: ${e.message}`);
  } finally {
    if (generation === sessionGeneration) { topicSaveInFlight = false; setAuthUI(); }
  }
};

document.getElementById('addRule').onclick = async () => {
  if (ruleSaveInFlight) return;
  if (!authenticated || authInFlight || manualDedupeInFlight || dedupeSubmitInFlight) {
    status('sign in first');
    return;
  }
  const generation = sessionGeneration, revision = ruleEditorRevision, submitted = JSON.stringify(ruleDraft());
  ruleSaveInFlight = true;
  setAuthUI();
  try {
    await call('/admin/api/rules', { method: 'POST', body: submitted });
    if (generation === sessionGeneration && revision === ruleEditorRevision && submitted === JSON.stringify(ruleDraft())) resetRuleEditor();
    status('rule saved');
    try { await loadRules(); }
    catch (e) { if (!e.staleSession) status(`rule saved; list refresh failed: ${e.message}`); }
  } catch (e) {
    if (e.staleSession) return;
    status(`rule save failed: ${e.message}`);
  } finally {
    if (generation === sessionGeneration) { ruleSaveInFlight = false; setAuthUI(); }
  }
};

runIngestBtn.onclick = async () => {
  if (authInFlight) return;
  const generation = sessionGeneration;
  if (manualIngestInFlight || runIngestBtn.disabled) {
    status('manual ingest ignored: already running');
    return;
  }
  try {
    manualIngestInFlight = true;
    runIngestBtn.disabled = true;
    runIngestBtn.classList.add('is-busy');
    runIngestBtn.textContent = 'Run Now (Running...)';
    status('manual ingest requested (running...)');
    const accepted = await call('/admin/api/ingest', { method: 'POST', body: JSON.stringify({}) });
    requestedRunID = accepted.state.run_id;
    status('manual ingest accepted; progress shown below');
    await refreshStatus();
  } catch (e) {
    if (e.staleSession) return;
    if (String(e.message).includes('just completed')) {
      status(`manual ingest cooldown: ${e.message}`);
    } else {
      status(`manual ingest failed: ${e.message}`);
    }
  } finally {
    if (generation === sessionGeneration) {
      manualIngestInFlight = false;
      await refreshStatus().catch(() => {});
    }
  }
};

runDedupeBtn.onclick = async () => {
  if (!authenticated || authInFlight || dedupeSubmitInFlight) return;
  const generation = sessionGeneration;
  if (manualDedupeInFlight || runDedupeBtn.disabled) {
    status('retroactive dedupe ignored: already running');
    return;
  }
  try {
    dedupeSubmitInFlight = true;
    dedupeRevision++;
    setAuthUI();
    status('submitting retroactive dedupe...');
    const state = await call('/admin/api/dedupe/jobs', { method: 'POST', body: JSON.stringify({}) });
    dedupeRevision++;
    renderDedupe(state);
  } catch (e) {
    if (e.staleSession) return;
    status(`dedupe request unconfirmed: ${e.message}; checking server state before any retry`);
    try { await refreshDedupe(); } catch (checkError) { if (!checkError.staleSession) status(`dedupe status unavailable: ${checkError.message}; reload before retrying`); }
  } finally {
    if (generation === sessionGeneration) {
      dedupeSubmitInFlight = false;
      setAuthUI();
    }
  }
};

function renderSchedule(state, waiting = '') {
  const next = localDate(state.next_scheduled_at);
  document.getElementById('nextScheduled').textContent = next ?
    `Next scheduled ingestion: ${next} (${state.schedule_mode || 'scheduled'}${waiting && Date.parse(state.next_scheduled_at) <= Date.now() ? '; due, waiting for ' + waiting : ''}).` :
    (state.running && state.current_source === 'scheduled' ? 'Next scheduled ingestion will be set after this scheduled run finishes.' : 'No next scheduled ingestion is currently available.');
}

function renderDedupe(state) {
  if (!['idle','running','completed','failed'].includes(state.status)) throw new Error('Invalid dedupe status; reload before retrying.');
  const changed = state.id !== lastDedupeState.id || state.status !== lastDedupeState.status;
  lastDedupeState = state;
  if (state.schedule) lastSchedulerState = state.schedule;
  manualDedupeInFlight = state.status === 'running';
  let message = 'No retroactive dedupe since this service started.';
  if (manualDedupeInFlight) message = 'Retroactive dedupe accepted; running in the background. You can close this page and return.';
  if (state.status === 'completed') {
    const st = state.result?.stats || {};
    message = `retroactive dedupe completed: hidden=${Number(st.same_run_hidden || 0) + Number(st.historical_hidden || 0)} (highest_only=${Number(st.same_run_hidden || 0)}, historical=${Number(st.historical_hidden || 0)}); all-time=${Number(state.result?.dedupe_hidden_total || 0)}`;
  }
  if (state.status === 'failed') message = state.error || 'Dedupe was not completed; check service logs before retrying.';
  document.getElementById('dedupeState').textContent = message;
  if (changed && state.status !== 'idle') status(message);
  setAuthUI();
  runDedupeBtn.classList.toggle('is-busy', manualDedupeInFlight);
  runDedupeBtn.textContent = manualDedupeInFlight ? 'Run Retroactive Dedupe (Running...)' : 'Run Retroactive Dedupe';
  if (state.schedule) renderSchedule(state.schedule, manualDedupeInFlight ? 'dedupe' : '');
}

async function refreshDedupe() {
  const revision = dedupeRevision;
  const state = await call('/admin/api/dedupe/jobs');
  if (revision === dedupeRevision) renderDedupe(state);
  return manualDedupeInFlight || dedupeSubmitInFlight;
}

document.body.addEventListener('click', async (e) => {
  if (!authenticated || authInFlight) return;
  if (e.target.matches('[data-edit-topic]')) {
    topicEditorRevision++;
    editingTopicID = Number(e.target.dataset.editTopic);
    document.getElementById('addTopic').textContent = 'Update Topic';
    document.getElementById('topicQ').value = e.target.dataset.topicQuery || '';
    document.getElementById('topicW').value = e.target.dataset.topicWeight || '1';
    document.getElementById('topicE').checked = String(e.target.dataset.topicEnabled) === 'true';
    document.getElementById('topicQ').focus();
    status('topic loaded into editor');
  }
  if (e.target.matches('[data-edit-rule]')) {
    ruleEditorRevision++;
    editingRuleID = Number(e.target.dataset.editRule);
    document.getElementById('addRule').textContent = 'Update Rule';
    document.getElementById('ruleP').value = e.target.dataset.rulePattern || '';
    document.getElementById('rulePenalty').value = e.target.dataset.rulePenalty || '5';
    document.getElementById('ruleE').checked = String(e.target.dataset.ruleEnabled) === 'true';
    document.getElementById('ruleP').focus();
    status('rule loaded into editor');
  }
  if (e.target.matches('[data-del-topic]')) {
    if (manualDedupeInFlight || dedupeSubmitInFlight) { status('wait for retroactive dedupe to finish'); return; }
    try {
      await call(`/admin/api/topics?id=${e.target.dataset.delTopic}`, { method: 'DELETE' });
      if (editingTopicID === Number(e.target.dataset.delTopic)) resetTopicEditor();
      await loadTopics();
      status('topic deleted');
    } catch (err) {
      if (err.staleSession) return;
      status(`topic delete failed: ${err.message}`);
    }
  }
  if (e.target.matches('[data-del-rule]')) {
    if (manualDedupeInFlight || dedupeSubmitInFlight) { status('wait for retroactive dedupe to finish'); return; }
    try {
      await call(`/admin/api/rules?id=${e.target.dataset.delRule}`, { method: 'DELETE' });
      if (editingRuleID === Number(e.target.dataset.delRule)) resetRuleEditor();
      await loadRules();
      status('rule deleted');
    } catch (err) {
      if (err.staleSession) return;
      status(`rule delete failed: ${err.message}`);
    }
  }
});

async function refreshStatus() {
  if (!authenticated || authInFlight || statusInFlight) return;
  const generation = sessionGeneration;
  statusInFlight = true;
  try {
    if (await refreshDedupe()) return;
    if (adminDataDeferred) await loadAdminData();
    const j = await call('/admin/api/status');
    if (!authenticated) return;
    const build = j.build || {};
    const ingest = j.ingest || {};
    const ingestState = ingest.state || {};
    lastSchedulerState = ingestState;
    if (requestedRunID && !ingestState.running && ingestState.run_id >= requestedRunID) {
      status(ingestState.last_error ? `ingest finished with warnings: ${ingestState.last_error}` : 'manual ingest completed');
      requestedRunID = 0;
    }
    const lastMessages = Array.isArray(ingest.last_messages) ? ingest.last_messages.filter(Boolean) : [];
    const lastMessagesText = lastMessages.length > 0 ? lastMessages.join('\n') : (ingest.last_message || '-');
    const counts = j.counts || {};
    const running = manualIngestInFlight || Boolean(ingestState.running);
    const checking = engineCheckInFlight || Boolean(ingestState.search_check_running);
    const cooling = Date.parse(ingestState.cooldown_until) > Date.now();
    runIngestBtn.disabled = !authenticated || running || cooling || checking;
    runIngestBtn.classList.toggle('is-busy', running);
    runIngestBtn.textContent = running ? 'Run Now (Running...)' : checking ? 'Run Now (Engine check...)' : cooling ? 'Run Now (Cooling down...)' : 'Run Now';
    runDedupeBtn.disabled = !authenticated || running || checking || manualDedupeInFlight;
    runDedupeBtn.classList.toggle('is-busy', manualDedupeInFlight);
    runDedupeBtn.textContent = manualDedupeInFlight ? 'Run Retroactive Dedupe (Running...)' : 'Run Retroactive Dedupe';
    renderSchedule(ingestState, checking ? 'engine check' : '');
    renderEngineCheck(j.search_check || {}, ingestState);
    ingestStateEl.textContent =
      `build: ${build.version || '-'} (commit=${build.commit || '-'}, built=${build.built_at || '-'})\n` +
      `running: ${Boolean(ingestState.running)}\n` +
      `source: ${ingestState.current_source || ingestState.last_source || '-'}\n` +
      `started_at: ${ingestState.started_at || '-'}\n` +
      `last_completed_at: ${ingestState.last_completed_at || '-'}\n` +
      `last_duration_ms: ${ingestState.last_duration_ms || 0}\n` +
      `last_error: ${ingestState.last_error || '-'}\n` +
      `last_messages:\n${lastMessagesText}\n` +
      `last_message_at: ${ingest.last_message_at || '-'}`;
    countsEl.textContent =
      `build_version: ${build.version || '-'}\n` +
      `unread: ${counts.unread || 0}\n` +
      `seen: ${counts.seen || 0}\n` +
      `read: ${counts.read || 0}\n` +
      `useful: ${counts.useful || 0}\n` +
      `hidden: ${counts.hidden || 0}\n` +
      `archived: ${counts.archived || 0}\n` +
      `dedupe_hidden_total: ${Number(j.dedupe_hidden_total || 0)}`;
  } catch (e) {
    if (e.staleSession) return;
    if (e.status === 401 || e.status === 403) {
      authenticated = false;
      manualIngestInFlight = false;
      manualDedupeInFlight = false;
      setAuthUI();
      status('session expired; sign in again');
      return;
    }
    status(`status refresh failed: ${e.message}`);
  } finally {
    if (generation === sessionGeneration) statusInFlight = false;
  }
}

function localDate(value) {
  if (!value) return '';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) || date.getFullYear() < 1970 ? '' : date.toLocaleString();
}

function renderEngineCheck(state, ingestState = {}) {
  if (!authenticated || Number(state.id || 0) < latestEngineCheckID) return;
  latestEngineCheckID = Number(state.id || 0);
  const checking = engineCheckInFlight || state.status === 'running' || Boolean(ingestState.search_check_running);
  const cooling = Date.parse(ingestState.search_check_cooldown_until) > Date.now();
  checkEnginesBtn.disabled = checking || cooling || Boolean(ingestState.running) || manualIngestInFlight || manualDedupeInFlight || dedupeSubmitInFlight;
  checkEnginesBtn.classList.toggle('is-busy', checking);
  checkEnginesBtn.textContent = checking ? 'Checking Search Engines...' : cooling ? 'Check Search Engines (Cooling down...)' : ingestState.running ? 'Check Search Engines (Ingestion running...)' : 'Check Search Engines';
  const report = state.report || {};
  const rows = Array.isArray(report.categories) ? report.categories : [];
  const labels = {results:'Results returned', empty:'Empty response (not proof of failure)', warnings:'Engine warnings', failed:'Request failed', returned_results:'Returned results', rate_limited:'Rate limited', captcha:'CAPTCHA', access_denied:'Access denied', timeout:'Timeout', http_error:'HTTP error', suspended:'Suspended', engine_error:'Engine error'};
  document.getElementById('engineCheckState').textContent = checking ? 'Check accepted; running in the background (up to three minutes). You can leave this page and return.' : state.status === 'idle' || !state.id ? 'Not checked since this service started.' :
    `Last check: ${localDate(state.completed_at) || localDate(state.started_at)}. ${report.error ? 'Stopped: ' + report.error + '. ' : ''}Checked ${report.checked_instances || 0} of ${report.configured_instances || 0} configured instances. Engines absent from this report have unknown status; empty results do not mean unavailable.${cooling ? ' Next check allowed: ' + localDate(ingestState.search_check_cooldown_until) + '.' : ''}`;
  document.getElementById('engineRows').innerHTML = rows.map(row => {
    const engines = (Array.isArray(row.engines) ? row.engines : []).map(engine => `${engine.name}: ${labels[engine.status] || engine.status}`).join('; ');
    return `<tr><td data-label="Instance">${Number(row.instance || 0)}</td><td data-label="Category">${escHtml(row.category)}</td><td data-label="Response">${escHtml(labels[row.status] || row.status)}${row.code ? ' (' + escHtml(row.code) + ')' : ''}</td><td data-label="Results">${Number(row.results || 0)}</td><td data-label="Engines">${escHtml(engines || 'No engine observation available')}${row.truncated ? ' (report capped)' : ''}</td></tr>`;
  }).join('');
}

checkEnginesBtn.onclick = async () => {
  if (!authenticated || authInFlight || checkEnginesBtn.disabled || engineCheckInFlight) return;
  const generation = sessionGeneration;
  engineCheckInFlight = true;
  checkEnginesBtn.disabled = true;
  runIngestBtn.disabled = true;
  checkEnginesBtn.textContent = 'Checking Search Engines...';
  status('requesting search engine check...');
  try {
    const accepted = await call('/admin/api/search-check', {method:'POST', body:JSON.stringify({})});
    renderEngineCheck(accepted);
    status('search engine check accepted; results will appear in Search Engines');
  } catch (e) { if (!e.staleSession) status(`search engine check request failed: ${e.message}; refresh status before retrying`); }
  finally { if (generation === sessionGeneration) { engineCheckInFlight = false; await refreshStatus(); } }
};

function resetTopicEditor() {
  topicEditorRevision++;
  editingTopicID = 0;
  document.getElementById('topicQ').value = '';
  document.getElementById('topicW').value = '1';
  document.getElementById('topicE').checked = true;
  document.getElementById('addTopic').textContent = 'Add/Update';
}
function resetRuleEditor() {
  ruleEditorRevision++;
  editingRuleID = 0;
  document.getElementById('ruleP').value = '';
  document.getElementById('rulePenalty').value = '5';
  document.getElementById('ruleE').checked = true;
  document.getElementById('addRule').textContent = 'Add/Update';
}
document.getElementById('newTopic').onclick = resetTopicEditor;
document.getElementById('newRule').onclick = resetRuleEditor;

async function bootstrapAfterAuth() {
  try {
    if (await refreshDedupe()) return;
    await loadAdminData();
    await refreshStatus();
  } catch (e) {
    if (e.staleSession) return;
    status(e.message);
  }
}

async function loadAdminData() {
  const age = await call('/admin/api/archive');
  document.getElementById('archiveDays').value = String(age.active_days ?? 30);
  await loadTopics();
  await loadRules();
  adminDataDeferred = false;
}

function archiveDays() {
  const raw = document.getElementById('archiveDays').value.trim();
  const days = Number(raw);
  if (!raw || !Number.isInteger(days) || days < 0 || days > 36500) throw new Error('enter whole days from 0 to 36500');
  return days;
}

document.getElementById('previewArchive').onclick = async () => {
  if (!authenticated || authInFlight || manualDedupeInFlight || dedupeSubmitInFlight) return;
  try {
    const j = await call(`/admin/api/archive?days=${archiveDays()}`);
    document.getElementById('archiveResult').textContent = `Would newly archive ${j.stats.candidates} article rows; restore ${j.stats.restorable} age-archived rows. Active feed limit: ${j.active_days} days.`;
  } catch (e) { if (!e.staleSession) status(`archive preview failed: ${e.message}`); }
};

document.getElementById('applyArchive').onclick = async () => {
  if (!authenticated || authInFlight || manualDedupeInFlight || dedupeSubmitInFlight) return;
  const generation = sessionGeneration;
  const button = document.getElementById('applyArchive');
  if (button.disabled || archiveInFlight) return;
  try {
    const days = archiveDays();
    if (!confirm(`Set feed age limit to ${days} days and archive older unread stories? No deletion; increasing this limit can restore age-archived rows.`)) return;
    archiveInFlight = true;
    setAuthUI();
    const j = await call('/admin/api/archive', {method:'POST',body:JSON.stringify({days})});
    document.getElementById('archiveResult').textContent = `Archived ${j.stats.archived}; restored ${j.stats.restored}. Feed age limit: ${days} days.`;
    status('age limit saved and archive completed');
    await refreshStatus();
  } catch (e) { if (!e.staleSession) status(`archive failed: ${e.message}; reload to verify the active limit before retrying`); }
  finally { if (generation === sessionGeneration) { archiveInFlight = false; setAuthUI(); } }
};

document.getElementById('loadDomains').onclick = async () => {
  if (!authenticated || authInFlight || manualDedupeInFlight || dedupeSubmitInFlight) return;
  const generation = sessionGeneration;
  const button = document.getElementById('loadDomains');
  if (button.disabled || domainReportInFlight) return;
  domainReportInFlight = true;
  setAuthUI();
  try {
    const j = await call('/admin/api/domains');
    if (!authenticated) return;
    document.getElementById('domainRows').innerHTML = (j.items || []).map(d =>
      `<tr><td>${escHtml(d.domain || '(unknown)')}</td>${['positive','read','useful','hidden','seen','total'].map(k => `<td>${Number(d[k] || 0)}</td>`).join('')}</tr>`).join('');
    if (!j.items?.length) document.getElementById('domainRows').innerHTML = '<tr><td colspan="7">No domains with at least 2 positive reads yet.</td></tr>';
    status('domain report generated from retained database history');
  } catch (e) { if (!e.staleSession) status(`domain report failed: ${e.message}`); }
  finally { if (generation === sessionGeneration) { domainReportInFlight = false; setAuthUI(); } }
};

setAuthUI();
status('checking session...');

(async () => {
  try {
    const j = await call('/admin/api/session');
    csrfToken = j.csrf_token || '';
    authenticated = true;
    setAuthUI();
    status('session restored');
    await bootstrapAfterAuth();
  } catch (e) {
    if (e.staleSession) return;
    authenticated = false;
    csrfToken = '';
    setAuthUI();
    status('sign in to access admin actions');
  }
})();

setInterval(() => {
  refreshStatus().catch(() => {});
}, 3000);
