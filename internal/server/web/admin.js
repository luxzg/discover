const statusEl = document.getElementById('status');
const ingestStateEl = document.getElementById('ingestState');
const countsEl = document.getElementById('counts');
const secretEl = document.getElementById('secret');
const runIngestBtn = document.getElementById('runIngest');
const loginBtn = document.getElementById('loginBtn');
const logoutBtn = document.getElementById('logoutBtn');

let manualIngestInFlight = false;
let authenticated = false;

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

async function call(url, opts = {}) {
  const headers = { ...(opts.headers || {}) };
  const hasBody = typeof opts.body !== 'undefined';
  if (hasBody && !headers['Content-Type']) {
    headers['Content-Type'] = 'application/json';
  }
  const r = await fetch(url, { ...opts, headers });
  const j = await r.json().catch(() => ({}));
  if (!r.ok) {
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
}

loginBtn.onclick = async () => {
  const secret = secretEl.value.trim();
  if (!secret) {
    status('enter admin secret first');
    return;
  }
  try {
    await call('/admin/api/login', { method: 'POST', body: JSON.stringify({ secret }) });
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
  } catch (_) {
    // Best-effort logout.
  }
  authenticated = false;
  manualIngestInFlight = false;
  setAuthUI();
  document.getElementById('topics').innerHTML = '';
  document.getElementById('rules').innerHTML = '';
  ingestStateEl.textContent = '';
  countsEl.textContent = '';
  status('signed out');
};

async function loadTopics() {
  const j = await call('/admin/api/topics');
  document.getElementById('topics').innerHTML = (j.items || []).map(t => `<li>${t.query} (w=${t.weight}, enabled=${t.enabled}) <button data-edit-topic="1" data-topic-query="${escAttr(t.query)}" data-topic-weight="${t.weight}" data-topic-enabled="${t.enabled}">edit</button> <button data-del-topic="${t.id}">delete</button></li>`).join('');
}

async function loadRules() {
  const j = await call('/admin/api/rules');
  document.getElementById('rules').innerHTML = (j.items || []).map(r => `<li>${r.pattern} (-${r.penalty}, enabled=${r.enabled}) <button data-edit-rule="1" data-rule-pattern="${escAttr(r.pattern)}" data-rule-penalty="${r.penalty}" data-rule-enabled="${r.enabled}">edit</button> <button data-del-rule="${r.id}">delete</button></li>`).join('');
}

document.getElementById('addTopic').onclick = async () => {
  if (!authenticated) {
    status('sign in first');
    return;
  }
  try {
    await call('/admin/api/topics', { method: 'POST', body: JSON.stringify({ query: document.getElementById('topicQ').value, weight: Number(document.getElementById('topicW').value || 1), enabled: document.getElementById('topicE').checked }) });
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
    await call('/admin/api/rules', { method: 'POST', body: JSON.stringify({ pattern: document.getElementById('ruleP').value, penalty: Number(document.getElementById('rulePenalty').value || 5), enabled: document.getElementById('ruleE').checked }) });
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
    await call('/admin/api/ingest', { method: 'POST', body: JSON.stringify({}) });
    status('manual ingest completed');
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

document.body.addEventListener('click', async (e) => {
  if (e.target.matches('[data-edit-topic]')) {
    document.getElementById('topicQ').value = e.target.dataset.topicQuery || '';
    document.getElementById('topicW').value = e.target.dataset.topicWeight || '1';
    document.getElementById('topicE').checked = String(e.target.dataset.topicEnabled) === 'true';
    document.getElementById('topicQ').focus();
    status('topic loaded into editor');
  }
  if (e.target.matches('[data-edit-rule]')) {
    document.getElementById('ruleP').value = e.target.dataset.rulePattern || '';
    document.getElementById('rulePenalty').value = e.target.dataset.rulePenalty || '5';
    document.getElementById('ruleE').checked = String(e.target.dataset.ruleEnabled) === 'true';
    document.getElementById('ruleP').focus();
    status('rule loaded into editor');
  }
  if (e.target.matches('[data-del-topic]')) {
    try {
      await call(`/admin/api/topics?id=${e.target.dataset.delTopic}`, { method: 'DELETE' });
      await loadTopics();
      status('topic deleted');
    } catch (err) {
      status(`topic delete failed: ${err.message}`);
    }
  }
  if (e.target.matches('[data-del-rule]')) {
    try {
      await call(`/admin/api/rules?id=${e.target.dataset.delRule}`, { method: 'DELETE' });
      await loadRules();
      status('rule deleted');
    } catch (err) {
      status(`rule delete failed: ${err.message}`);
    }
  }
});

async function refreshStatus() {
  if (!authenticated) return;
  try {
    const j = await call('/admin/api/status');
    const ingest = j.ingest || {};
    const ingestState = ingest.state || {};
    const counts = j.counts || {};
    const running = manualIngestInFlight || Boolean(ingestState.running);
    runIngestBtn.disabled = !authenticated || running;
    runIngestBtn.classList.toggle('is-busy', running);
    runIngestBtn.textContent = running ? 'Run Now (Running...)' : 'Run Now';
    ingestStateEl.textContent =
      `running: ${Boolean(ingestState.running)}\n` +
      `source: ${ingestState.current_source || ingestState.last_source || '-'}\n` +
      `started_at: ${ingestState.started_at || '-'}\n` +
      `last_completed_at: ${ingestState.last_completed_at || '-'}\n` +
      `last_duration_ms: ${ingestState.last_duration_ms || 0}\n` +
      `last_error: ${ingestState.last_error || '-'}\n` +
      `last_message: ${ingest.last_message || '-'}\n` +
      `last_message_at: ${ingest.last_message_at || '-'}`;
    countsEl.textContent =
      `unread: ${counts.unread || 0}\n` +
      `seen: ${counts.seen || 0}\n` +
      `read: ${counts.read || 0}\n` +
      `useful: ${counts.useful || 0}\n` +
      `hidden: ${counts.hidden || 0}`;
  } catch (e) {
    if (e.status === 401 || e.status === 403) {
      authenticated = false;
      manualIngestInFlight = false;
      setAuthUI();
      status('session expired; sign in again');
      return;
    }
    status(`status refresh failed: ${e.message}`);
  }
}

async function bootstrapAfterAuth() {
  try {
    await loadTopics();
    await loadRules();
    await refreshStatus();
  } catch (e) {
    status(e.message);
  }
}

setAuthUI();
status('sign in to access admin actions');

setInterval(() => {
  refreshStatus().catch(() => {});
}, 3000);
