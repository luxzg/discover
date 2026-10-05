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
let authenticated = false;
let csrfToken = '';
let editingTopicID = 0;
let editingRuleID = 0;
let requestedRunID = 0;
let statusInFlight = false;

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
  const headers = { ...(opts.headers || {}) };
  const method = String(opts.method || 'GET').toUpperCase();
  const hasBody = typeof opts.body !== 'undefined';
  if (hasBody && !headers['Content-Type']) {
    headers['Content-Type'] = 'application/json';
  }
  if (csrfToken && method !== 'GET' && method !== 'HEAD' && method !== 'OPTIONS') {
    headers['X-CSRF-Token'] = csrfToken;
  }
  const r = await fetch(url, { ...opts, headers });
  const j = await r.json().catch(() => ({}));
  if (!r.ok) {
    if (r.status === 401) {
      authenticated = false;
      csrfToken = '';
      manualIngestInFlight = false;
      setAuthUI();
    }
    const err = new Error(j.error || r.statusText);
    err.status = r.status;
    throw err;
  }
  return j;
}

function setAuthUI() {
  loginBtn.disabled = authenticated;
  logoutBtn.disabled = !authenticated;
  secretEl.disabled = authenticated;
  runIngestBtn.disabled = !authenticated || manualIngestInFlight;
  runDedupeBtn.disabled = !authenticated || manualDedupeInFlight || manualIngestInFlight;
  topicsPanel.hidden = !authenticated;
  rulesPanel.hidden = !authenticated;
  ingestionPanel.hidden = !authenticated;
  countsPanel.hidden = !authenticated;
  document.getElementById('archivePanel').hidden = !authenticated;
  document.getElementById('domainsPanel').hidden = !authenticated;
  document.getElementById('enginePanel').hidden = !authenticated;
  checkEnginesBtn.disabled = !authenticated || engineCheckInFlight;
  if (!authenticated) {
    latestEngineCheckID = 0;
    document.getElementById('engineRows').innerHTML = '';
    document.getElementById('engineCheckState').textContent = '';
    document.getElementById('domainRows').innerHTML = '';
    document.getElementById('archiveResult').textContent = '';
  }
}

loginBtn.onclick = async () => {
  const secret = secretEl.value.trim();
  if (!secret) {
    status('enter admin secret first');
    return;
  }
  try {
    const res = await call('/admin/api/login', { method: 'POST', body: JSON.stringify({ secret }) });
    csrfToken = res.csrf_token || '';
    authenticated = true;
    setAuthUI();
    secretEl.value = '';
    status('signed in');
    await bootstrapAfterAuth();
  } catch (e) {
    status(`sign in failed: ${e.message}`);
  }
};

logoutBtn.onclick = async () => {
  try {
    await call('/admin/api/logout', { method: 'POST', body: JSON.stringify({}) });
  } catch (e) {
    if (e.status !== 401) { status(`sign out failed: ${e.message}; please retry`); return; }
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

document.getElementById('addTopic').onclick = async () => {
  if (!authenticated) {
    status('sign in first');
    return;
  }
  try {
    await call('/admin/api/topics', { method: 'POST', body: JSON.stringify({ id: editingTopicID, query: document.getElementById('topicQ').value, weight: Number(document.getElementById('topicW').value || 1), enabled: document.getElementById('topicE').checked }) });
    resetTopicEditor();
    await loadTopics();
    status('topic saved');
  } catch (e) {
    status(`topic save failed: ${e.message}`);
  }
};

document.getElementById('addRule').onclick = async () => {
  if (!authenticated) {
    status('sign in first');
    return;
  }
  try {
    await call('/admin/api/rules', { method: 'POST', body: JSON.stringify({ id: editingRuleID, pattern: document.getElementById('ruleP').value, penalty: Number(document.getElementById('rulePenalty').value || 5), enabled: document.getElementById('ruleE').checked }) });
    resetRuleEditor();
    await loadRules();
    status('rule saved');
  } catch (e) {
    status(`rule save failed: ${e.message}`);
  }
};

runIngestBtn.onclick = async () => {
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
    if (String(e.message).includes('just completed')) {
      status(`manual ingest cooldown: ${e.message}`);
    } else {
      status(`manual ingest failed: ${e.message}`);
    }
  } finally {
    manualIngestInFlight = false;
    await refreshStatus().catch(() => {});
  }
};

runDedupeBtn.onclick = async () => {
  if (manualDedupeInFlight || runDedupeBtn.disabled) {
    status('retroactive dedupe ignored: already running');
    return;
  }
  try {
    manualDedupeInFlight = true;
    runDedupeBtn.disabled = true;
    runDedupeBtn.classList.add('is-busy');
    runDedupeBtn.textContent = 'Run Retroactive Dedupe (Running...)';
    status('retroactive dedupe requested (running...)');
    const res = await call('/admin/api/dedupe', { method: 'POST', body: JSON.stringify({}) });
    const st = res.stats || {};
    status(`retroactive dedupe completed: hidden=${Number(st.same_run_hidden || 0) + Number(st.historical_hidden || 0)} (highest_only=${Number(st.same_run_hidden || 0)}, historical=${Number(st.historical_hidden || 0)})`);
    await refreshStatus();
  } catch (e) {
    status(`retroactive dedupe failed: ${e.message}`);
  } finally {
    manualDedupeInFlight = false;
    await refreshStatus().catch(() => {});
  }
};

document.body.addEventListener('click', async (e) => {
  if (e.target.matches('[data-edit-topic]')) {
    editingTopicID = Number(e.target.dataset.editTopic);
    document.getElementById('addTopic').textContent = 'Update Topic';
    document.getElementById('topicQ').value = e.target.dataset.topicQuery || '';
    document.getElementById('topicW').value = e.target.dataset.topicWeight || '1';
    document.getElementById('topicE').checked = String(e.target.dataset.topicEnabled) === 'true';
    document.getElementById('topicQ').focus();
    status('topic loaded into editor');
  }
  if (e.target.matches('[data-edit-rule]')) {
    editingRuleID = Number(e.target.dataset.editRule);
    document.getElementById('addRule').textContent = 'Update Rule';
    document.getElementById('ruleP').value = e.target.dataset.rulePattern || '';
    document.getElementById('rulePenalty').value = e.target.dataset.rulePenalty || '5';
    document.getElementById('ruleE').checked = String(e.target.dataset.ruleEnabled) === 'true';
    document.getElementById('ruleP').focus();
    status('rule loaded into editor');
  }
  if (e.target.matches('[data-del-topic]')) {
    try {
      await call(`/admin/api/topics?id=${e.target.dataset.delTopic}`, { method: 'DELETE' });
      if (editingTopicID === Number(e.target.dataset.delTopic)) resetTopicEditor();
      await loadTopics();
      status('topic deleted');
    } catch (err) {
      status(`topic delete failed: ${err.message}`);
    }
  }
  if (e.target.matches('[data-del-rule]')) {
    try {
      await call(`/admin/api/rules?id=${e.target.dataset.delRule}`, { method: 'DELETE' });
      if (editingRuleID === Number(e.target.dataset.delRule)) resetRuleEditor();
      await loadRules();
      status('rule deleted');
    } catch (err) {
      status(`rule delete failed: ${err.message}`);
    }
  }
});

async function refreshStatus() {
  if (!authenticated || statusInFlight) return;
  statusInFlight = true;
  try {
    const j = await call('/admin/api/status');
    if (!authenticated) return;
    const build = j.build || {};
    const ingest = j.ingest || {};
    const ingestState = ingest.state || {};
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
    runDedupeBtn.disabled = !authenticated || running || manualDedupeInFlight;
    runDedupeBtn.classList.toggle('is-busy', manualDedupeInFlight);
    runDedupeBtn.textContent = manualDedupeInFlight ? 'Run Retroactive Dedupe (Running...)' : 'Run Retroactive Dedupe';
    const next = localDate(ingestState.next_scheduled_at);
    document.getElementById('nextScheduled').textContent = next ?
      `Next scheduled ingestion: ${next} (${ingestState.schedule_mode || 'scheduled'}${checking && Date.parse(ingestState.next_scheduled_at) <= Date.now() ? '; due, waiting for engine check' : ''}).` :
      (ingestState.running && ingestState.current_source === 'scheduled' ? 'Next scheduled ingestion will be set after this scheduled run finishes.' : 'No next scheduled ingestion is currently available.');
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
    statusInFlight = false;
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
  checkEnginesBtn.disabled = checking || cooling || Boolean(ingestState.running) || manualIngestInFlight;
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
  if (!authenticated || checkEnginesBtn.disabled || engineCheckInFlight) return;
  engineCheckInFlight = true;
  checkEnginesBtn.disabled = true;
  runIngestBtn.disabled = true;
  checkEnginesBtn.textContent = 'Checking Search Engines...';
  status('requesting search engine check...');
  try {
    const accepted = await call('/admin/api/search-check', {method:'POST', body:JSON.stringify({})});
    renderEngineCheck(accepted);
    status('search engine check accepted; results will appear in Search Engines');
  } catch (e) { status(`search engine check request failed: ${e.message}; refresh status before retrying`); }
  finally { engineCheckInFlight = false; await refreshStatus(); }
};

function resetTopicEditor() {
  editingTopicID = 0;
  document.getElementById('topicQ').value = '';
  document.getElementById('topicW').value = '1';
  document.getElementById('topicE').checked = true;
  document.getElementById('addTopic').textContent = 'Add/Update';
}
function resetRuleEditor() {
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
    const age = await call('/admin/api/archive');
    document.getElementById('archiveDays').value = String(age.active_days ?? 30);
    await loadTopics();
    await loadRules();
    await refreshStatus();
  } catch (e) {
    status(e.message);
  }
}

function archiveDays() {
  const raw = document.getElementById('archiveDays').value.trim();
  const days = Number(raw);
  if (!raw || !Number.isInteger(days) || days < 0 || days > 36500) throw new Error('enter whole days from 0 to 36500');
  return days;
}

document.getElementById('previewArchive').onclick = async () => {
  try {
    const j = await call(`/admin/api/archive?days=${archiveDays()}`);
    document.getElementById('archiveResult').textContent = `Would newly archive ${j.stats.candidates} article rows; restore ${j.stats.restorable} age-archived rows. Active feed limit: ${j.active_days} days.`;
  } catch (e) { status(`archive preview failed: ${e.message}`); }
};

document.getElementById('applyArchive').onclick = async () => {
  const button = document.getElementById('applyArchive');
  if (button.disabled) return;
  try {
    const days = archiveDays();
    if (!confirm(`Set feed age limit to ${days} days and archive older unread stories? No deletion; increasing this limit can restore age-archived rows.`)) return;
    button.disabled = true;
    const j = await call('/admin/api/archive', {method:'POST',body:JSON.stringify({days})});
    document.getElementById('archiveResult').textContent = `Archived ${j.stats.archived}; restored ${j.stats.restored}. Feed age limit: ${days} days.`;
    status('age limit saved and archive completed');
    await refreshStatus();
  } catch (e) { status(`archive failed: ${e.message}; reload to verify the active limit before retrying`); }
  finally { button.disabled = false; }
};

document.getElementById('loadDomains').onclick = async () => {
  const button = document.getElementById('loadDomains');
  if (button.disabled) return;
  button.disabled = true;
  try {
    const j = await call('/admin/api/domains');
    if (!authenticated) return;
    document.getElementById('domainRows').innerHTML = (j.items || []).map(d =>
      `<tr><td>${escHtml(d.domain || '(unknown)')}</td>${['positive','read','useful','hidden','seen','total'].map(k => `<td>${Number(d[k] || 0)}</td>`).join('')}</tr>`).join('');
    if (!j.items?.length) document.getElementById('domainRows').innerHTML = '<tr><td colspan="7">No domains with at least 2 positive reads yet.</td></tr>';
    status('domain report generated from retained database history');
  } catch (e) { status(`domain report failed: ${e.message}`); }
  finally { button.disabled = false; }
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
  } catch (_) {
    authenticated = false;
    csrfToken = '';
    setAuthUI();
    status('sign in to access admin actions');
  }
})();

setInterval(() => {
  refreshStatus().catch(() => {});
}, 3000);
