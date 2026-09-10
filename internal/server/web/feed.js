let currentIds = [];
let authenticated = false;
let csrfToken = '';
let defaultHidePenalty = 10;
let buildVersion = '';
let nextInFlight = false;
let sessionGeneration = 0;
let hideInFlight = 0;

const feed = document.getElementById('feed');
const nextBtn = document.getElementById('nextBtn');
const statusEl = document.getElementById('status');
const userAuthTitle = document.getElementById('userAuthTitle');
const userNameEl = document.getElementById('userName');
const userSecretEl = document.getElementById('userSecret');
const userLoginBtn = document.getElementById('userLoginBtn');
const userLogoutBtn = document.getElementById('userLogoutBtn');
const userVersionSpacer = document.getElementById('userVersionSpacer');
const userBuildVersionEl = document.getElementById('userBuildVersion');

async function api(url, opts = {}) {
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
  const res = await fetch(url, { ...opts, headers });
  let j = {}, bodyError;
  try { j = await res.json(); } catch (err) { bodyError = err; }
  if (generation !== sessionGeneration) {
    const err = new Error('Session changed; ignoring an old response.');
    err.staleSession = true;
    throw err;
  }
  if (!res.ok) {
    if (res.status === 401) {
      authenticated = false;
      csrfToken = '';
      sessionGeneration++;
      setAuthUI();
      statusEl.textContent = 'Session expired; sign in again.';
    }
    const err = new Error(j?.error || res.statusText || `HTTP ${res.status}`);
    err.status = res.status;
    throw err;
  }
  if (bodyError) throw bodyError;
  return j;
}

function esc(s) {
  return String(s || '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}

function publishedLabel(v) {
  if (!v) return '';
  const d = new Date(v);
  if (!Number.isFinite(d.getTime()) || d.getUTCFullYear() < 2000) return '';
  const now = new Date();
  const dayMs = 24 * 60 * 60 * 1000;
  if (d.getTime() > now.getTime() + dayMs) return '';
  const days = Math.round((Date.UTC(now.getFullYear(), now.getMonth(), now.getDate()) - Date.UTC(d.getFullYear(), d.getMonth(), d.getDate())) / dayMs);
  if (days <= 0) return 'today';
  if (days === 1) return 'yesterday';
  if (days < 14) return `${days} days ago`;
  if (days < 60) return `${Math.floor(days / 7)} weeks ago`;
  if (days < 365) { const n = Math.floor(days / 30); return `${n} month${n === 1 ? '' : 's'} ago`; }
  const n = Math.floor(days / 365); return `${n} year${n === 1 ? '' : 's'} ago`;
}

function safeLink(raw) {
  try { const u = new URL(raw); return ['http:', 'https:'].includes(u.protocol) && !u.username && !u.password ? u.href : ''; }
  catch (_) { return ''; }
}

function setAuthUI() {
  userAuthTitle.hidden = authenticated;
  userNameEl.hidden = authenticated;
  userSecretEl.hidden = authenticated;
  userLoginBtn.hidden = authenticated;
  userLogoutBtn.hidden = !authenticated;
  userVersionSpacer.hidden = !authenticated || !buildVersion;
  userBuildVersionEl.hidden = !authenticated || !buildVersion;
  userBuildVersionEl.textContent = buildVersion || '';
  userNameEl.disabled = authenticated;
  userSecretEl.disabled = authenticated;
  nextBtn.disabled = !authenticated || nextInFlight || hideInFlight > 0;
  if (!authenticated) {
    feed.innerHTML = '';
    currentIds = [];
  }
}

function card(item) {
  const img = item.thumbnail_url ? `<img class="thumb" src="${esc(item.thumbnail_url)}" alt="" loading="lazy" referrerpolicy="no-referrer">` : '';
  const pub = publishedLabel(item.published_at);
  const pubPart = pub ? ` | ${esc(pub)}` : '';
  const sources = (item.sources || []).filter(s => safeLink(s.url));
  const otherSources = sources.length ? `<details class="story-sources"><summary>Other sources (${sources.length})</summary><ul>${sources.map(s => `<li><a href="${esc(safeLink(s.url))}" target="_blank" rel="noopener noreferrer" data-click="1" data-source-id="${s.id}">${esc(s.source_domain || s.title)}</a></li>`).join('')}</ul></details>` : '';
  return `<article class="card" data-id="${item.id}">
    ${img}
    <div class="card-body"><a class="card-link" href="${esc(safeLink(item.url))}" target="_blank" rel="noopener noreferrer" data-click="1">
      <div class="card-main">
        <h3 class="card-title">${esc(item.title)}</h3>
        <div class="card-source">${esc(item.source_domain || 'unknown')} | score ${Number(item.score).toFixed(2)}${pubPart}</div>
      </div>
    </a>${otherSources}<p class="card-action-status" role="status" hidden></p></div>
    <div class="menu"><button data-menu="1">⋯</button><div class="menu-panel">
      <button data-action="up">👍 Useful</button>
      <button data-action="down">👎 Hide</button>
      <button data-action="dont" class="danger">🚫 Hide This</button>
      <button data-action="domain" class="danger">🌐 Hide Domain</button>
    </div></div>
  </article>`;
}

async function loadFeed() {
  if (!authenticated) throw new Error('sign in required');
  const generation = sessionGeneration;
  try {
    const data = await api('/api/feed');
    if (!authenticated || generation !== sessionGeneration) throw new Error('session changed');
    const items = data.items || [];
    currentIds = items.map(i => i.id);
    feed.innerHTML = items.map(card).join('');
    statusEl.textContent = `${new Date().toISOString()} loaded ${items.length} cards`;
    return items.length;
  } catch (e) {
    if (e.status === 401) {
      authenticated = false;
      setAuthUI();
      statusEl.textContent = `${new Date().toISOString()} sign in required`;
      throw e;
    }
    statusEl.textContent = `${new Date().toISOString()} feed load failed: ${e.message}`;
    throw e;
  }
}

function cardActionStatus(cardEl, message) {
  const local = cardEl?.querySelector('.card-action-status');
  if (local) { local.textContent = message; local.hidden = false; }
  statusEl.textContent = message;
}

async function hideAPI(url, opts = {}) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 10000);
  try { return await api(url, { ...opts, signal: controller.signal }); }
  finally { clearTimeout(timer); }
}

function waitForHidePoll() { return new Promise(resolve => setTimeout(resolve, 1000)); }

async function waitForHide(state, cardEl) {
  const generation = sessionGeneration;
  for (;;) {
    if (!state || typeof state.id !== 'string' || !state.id || !['running', 'completed', 'failed'].includes(state.status)) {
      throw new Error('Could not confirm hide status; retry the same action to check.');
    }
    if (state.status === 'completed') return state.matched_ids || [];
    if (state.status === 'failed') {
      if (cardEl) delete cardEl.hideRequest;
      throw new Error(state.error || 'Hide was not completed; please retry.');
    }
    cardActionStatus(cardEl, 'Hide accepted. Applying hide in the background; you can keep reading or close this page.');
    await waitForHidePoll();
    if (!authenticated || generation !== sessionGeneration) throw new Error('Session changed; reload to check the hide result.');
    state = await hideAPI(`/api/articles/dontshow/status?id=${encodeURIComponent(state.id)}`);
  }
}

async function applyHideRequest(cardEl, id, pattern, penalty) {
  // Keep the same request ID on an uncertain network failure. A manual retry
  // checks/reuses the accepted job rather than applying another mutation.
  if (!cardEl.hideRequest || cardEl.hideRequest.pattern !== pattern || cardEl.hideRequest.penalty !== penalty) {
    const bytes = crypto.getRandomValues(new Uint8Array(16));
    const requestID = Array.from(bytes, b => b.toString(16).padStart(2, '0')).join('');
    cardEl.hideRequest = { id, pattern, penalty, request_id: requestID, visible_ids: currentIds.slice(0, 100) };
  }
  cardEl.querySelector('.menu')?.classList.remove('open');
  cardActionStatus(cardEl, 'Submitting hide...');
  hideInFlight++;
  setAuthUI();
  try {
    const state = await hideAPI('/api/articles/dontshow', { method: 'POST', body: JSON.stringify(cardEl.hideRequest) });
    return await waitForHide(state, cardEl);
  } catch (err) {
    if (err.status === 404) delete cardEl.hideRequest;
    if (!err.status && ['TypeError', 'AbortError', 'SyntaxError'].includes(err.name)) {
      throw new Error('Could not confirm hide completion; it may still be running. Retry the same action to check.');
    }
    throw err;
  } finally {
    hideInFlight--;
    setAuthUI();
  }
}

async function resumeHide() {
  const state = await hideAPI('/api/articles/dontshow/status');
  if (state.status !== 'running') return;
  hideInFlight++;
  setAuthUI();
  try { await waitForHide(state, null); }
  finally { hideInFlight--; setAuthUI(); }
}

userLoginBtn.addEventListener('click', async () => {
  const username = userNameEl.value.trim();
  const secret = userSecretEl.value.trim();
  if (!username || !secret) {
    statusEl.textContent = `${new Date().toISOString()} enter username and secret`;
    return;
  }
  try {
    const j = await api('/api/login', { method: 'POST', body: JSON.stringify({ username, secret }) });
    csrfToken = j.csrf_token || '';
    defaultHidePenalty = Number(j.hide_rule_default_penalty || 10);
    buildVersion = String(j.build_version || '');
    authenticated = true;
    userSecretEl.value = '';
    setAuthUI();
    statusEl.textContent = `${new Date().toISOString()} signed in`;
    await resumeHide();
    await loadFeed();
  } catch (e) {
    statusEl.textContent = `${new Date().toISOString()} ${authenticated ? 'feed load failed' : 'sign in failed'}: ${e.message}`;
  }
});

userLogoutBtn.addEventListener('click', async () => {
  try {
    await api('/api/logout', { method: 'POST', body: JSON.stringify({}) });
  } catch (e) {
    if (e.status !== 401) { statusEl.textContent = `Sign out failed: ${e.message}. Please retry.`; return; }
  }
  sessionGeneration++;
  authenticated = false;
  csrfToken = '';
  buildVersion = '';
  setAuthUI();
  statusEl.textContent = `${new Date().toISOString()} signed out`;
});

nextBtn.addEventListener('click', async () => {
  if (!authenticated || nextInFlight || hideInFlight > 0) return;
  nextInFlight = true;
  setAuthUI();
  const generation = sessionGeneration;
  try {
    if (currentIds.length) await api('/api/feed/seen', { method: 'POST', body: JSON.stringify({ ids: currentIds }) });
    let count = await loadFeed();
    if (count === 0) {
      statusEl.textContent = `${new Date().toISOString()} no cards left; trying ingest refresh...`;
      try {
        const accepted = await api('/api/feed/refresh', { method: 'POST', body: JSON.stringify({}) });
        statusEl.textContent = 'Ingestion running; waiting for new cards...';
        let state;
        do {
          await new Promise(resolve => setTimeout(resolve, 3000));
          if (!authenticated || generation !== sessionGeneration) return;
          state = await api('/api/feed/refresh/status');
        } while (state.running && state.run_id === accepted.state.run_id);
        count = await loadFeed();
        statusEl.textContent = `Ingestion ${state.failed ? 'finished with warnings' : 'completed'}; loaded ${count} cards.`;
      } catch (refreshErr) {
        statusEl.textContent = `${new Date().toISOString()} ingest refresh skipped: ${refreshErr.message}`;
      }
    }
    window.scrollTo({ top: 0, behavior: 'smooth' });
  } catch (e) {
    statusEl.textContent = `${new Date().toISOString()} next batch failed: ${e.message}`;
  } finally {
    nextInFlight = false;
    setAuthUI();
  }
});

feed.addEventListener('error', e => { if (e.target.matches('img.thumb')) e.target.remove(); }, true);

feed.addEventListener('click', async (e) => {
  if (!authenticated) return;
  const cardEl = e.target.closest('.card');
  if (!cardEl) return;
  const id = Number(cardEl.dataset.id);

  if (e.target.matches('[data-menu]')) {
    e.preventDefault();
    const menu = cardEl.querySelector('.menu');
    document.querySelectorAll('.menu.open').forEach((m) => {
      if (m !== menu) m.classList.remove('open');
    });
    menu.classList.toggle('open');
    return;
  }

  if (e.target.matches('[data-action]')) {
    if (e.target.disabled) return;
    const actionButton = e.target;
    actionButton.disabled = true;
    const action = e.target.dataset.action;
    let matchedIds = [];
    try {
      if (action === 'dont') {
        const suggested = (cardEl.querySelector('.card-title')?.textContent || '').trim();
        const pattern = prompt('Pattern to hide (text/domain):', suggested);
        if (!pattern) return;
        const penaltyIn = prompt('Penalty weight:', String(defaultHidePenalty));
        const penalty = Number(penaltyIn);
        if (!Number.isFinite(penalty) || penalty <= 0) return;
        matchedIds = await applyHideRequest(cardEl, id, pattern, penalty);
      } else if (action === 'domain') {
        const link = cardEl.querySelector('.card-link');
        let suggestedDomain = '';
        try {
          suggestedDomain = new URL(link?.href || '').hostname || '';
        } catch (_) {
          suggestedDomain = '';
        }
        const pattern = prompt('Domain to hide:', suggestedDomain);
        if (!pattern) return;
        const penaltyIn = prompt('Penalty weight:', String(defaultHidePenalty));
        const penalty = Number(penaltyIn);
        if (!Number.isFinite(penalty) || penalty <= 0) return;
        matchedIds = await applyHideRequest(cardEl, id, pattern, penalty);
      } else {
        await api('/api/articles/action', { method: 'POST', body: JSON.stringify({ id, action }) });
      }
      if (action === 'up') {
        cardEl.querySelector('.menu')?.classList.remove('open');
      }
      if (action === 'down' || action === 'dont' || action === 'domain' || action === 'hide') {
        cardEl.remove();
        currentIds = currentIds.filter(v => v !== id);
        const removed = new Set(matchedIds.map(Number));
        feed.querySelectorAll('.card').forEach(c => { if (removed.has(Number(c.dataset.id))) c.remove(); });
        currentIds = currentIds.filter(v => !removed.has(v));
      }
      statusEl.textContent = `${new Date().toISOString()} action applied`;
    } catch (err) {
      if (!err.staleSession) {
        const label = action === 'dont' || action === 'domain' ? 'Hide status' : 'Action failed';
        cardActionStatus(cardEl, `${label}: ${err.message}`);
      }
    } finally {
      actionButton.disabled = false;
    }
    return;
  }

  if (e.target.closest('[data-click]')) {
    try {
      const sourceID = Number(e.target.closest('[data-click]').dataset.sourceId) || id;
      await api('/api/articles/click', { method: 'POST', body: JSON.stringify({ id: sourceID }) });
    } catch (err) {
      statusEl.textContent = `${new Date().toISOString()} click tracking failed: ${err.message}`;
    }
  }
});

document.addEventListener('click', (e) => {
  if (!e.target.closest('.menu')) {
    document.querySelectorAll('.menu.open').forEach((m) => m.classList.remove('open'));
  }
});

setAuthUI();
statusEl.textContent = `${new Date().toISOString()} checking session...`;
(async () => {
  try {
    const j = await api('/api/session');
    csrfToken = j.csrf_token || '';
    defaultHidePenalty = Number(j.hide_rule_default_penalty || 10);
    buildVersion = String(j.build_version || '');
    authenticated = true;
    setAuthUI();
    await resumeHide();
    await loadFeed();
  } catch (err) {
    if (err.status === 401 || err.status === 403) {
      authenticated = false;
      csrfToken = '';
    }
    setAuthUI();
    statusEl.textContent = authenticated ? `Feed load failed: ${err.message}. Try Load Next.` : 'Sign in to load your feed.';
  }
})();
