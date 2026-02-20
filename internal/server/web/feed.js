let currentIds = [];
let authenticated = false;
let csrfToken = '';
let defaultHidePenalty = 10;

const feed = document.getElementById('feed');
const nextBtn = document.getElementById('nextBtn');
const statusEl = document.getElementById('status');
const userAuthTitle = document.getElementById('userAuthTitle');
const userNameEl = document.getElementById('userName');
const userSecretEl = document.getElementById('userSecret');
const userLoginBtn = document.getElementById('userLoginBtn');
const userLogoutBtn = document.getElementById('userLogoutBtn');

async function api(url, opts = {}) {
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
  const j = await res.json().catch(() => ({}));
  if (!res.ok) {
    const err = new Error(j.error || res.statusText || `HTTP ${res.status}`);
    err.status = res.status;
    throw err;
  }
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
  const days = Math.floor((now.getTime() - d.getTime()) / dayMs);
  if (days <= 0) return 'today';
  if (days === 1) return 'yesterday';
  if (days < 14) return `${days} days ago`;
  if (days < 60) return `${Math.floor(days / 7)} weeks ago`;
  if (days < 365) return `${Math.floor(days / 30)} months ago`;
  return `${Math.floor(days / 365)} years ago`;
}

function setAuthUI() {
  userAuthTitle.hidden = authenticated;
  userNameEl.hidden = authenticated;
  userSecretEl.hidden = authenticated;
  userLoginBtn.hidden = authenticated;
  userLogoutBtn.hidden = !authenticated;
  userNameEl.disabled = authenticated;
  userSecretEl.disabled = authenticated;
  nextBtn.disabled = !authenticated;
  if (!authenticated) {
    feed.innerHTML = '';
    currentIds = [];
  }
}

function card(item) {
  const img = item.thumbnail_url ? `<img class="thumb" src="${esc(item.thumbnail_url)}" alt="">` : '';
  const pub = publishedLabel(item.published_at);
  const pubPart = pub ? ` | ${esc(pub)}` : '';
  return `<article class="card" data-id="${item.id}">
    ${img}
    <a class="card-link" href="${esc(item.url)}" target="_blank" rel="noopener" data-click="1">
      <div class="card-main">
        <h3 class="card-title">${esc(item.title)}</h3>
        <div class="card-source">${esc(item.source_domain || 'unknown')} | score ${Number(item.score).toFixed(2)}${pubPart}</div>
      </div>
    </a>
    <div class="menu"><button data-menu="1">⋯</button><div class="menu-panel">
      <button data-action="up">👍 Useful</button>
      <button data-action="down">👎 Hide</button>
      <button data-action="dont" class="danger">🚫 Hide This</button>
      <button data-action="domain" class="danger">🌐 Hide Domain</button>
    </div></div>
  </article>`;
}

async function loadFeed() {
  if (!authenticated) return 0;
  try {
    const data = await api('/api/feed');
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
      return 0;
    }
    statusEl.textContent = `${new Date().toISOString()} feed load failed: ${e.message}`;
    return 0;
  }
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
    authenticated = true;
    userSecretEl.value = '';
    setAuthUI();
    statusEl.textContent = `${new Date().toISOString()} signed in`;
    await loadFeed();
  } catch (e) {
    statusEl.textContent = `${new Date().toISOString()} sign in failed: ${e.message}`;
  }
});

userLogoutBtn.addEventListener('click', async () => {
  try {
    await api('/api/logout', { method: 'POST', body: JSON.stringify({}) });
  } catch (_) {
    // best effort
  }
  authenticated = false;
  csrfToken = '';
  setAuthUI();
  statusEl.textContent = `${new Date().toISOString()} signed out`;
});

nextBtn.addEventListener('click', async () => {
  if (!authenticated) return;
  try {
    if (currentIds.length) await api('/api/feed/seen', { method: 'POST', body: JSON.stringify({ ids: currentIds }) });
    let count = await loadFeed();
    if (count === 0) {
      statusEl.textContent = `${new Date().toISOString()} no cards left; trying ingest refresh...`;
      try {
        await api('/api/feed/refresh', { method: 'POST', body: JSON.stringify({}) });
        statusEl.textContent = `${new Date().toISOString()} ingest refresh completed; loading feed`;
      } catch (refreshErr) {
        statusEl.textContent = `${new Date().toISOString()} ingest refresh skipped: ${refreshErr.message}`;
      }
      count = await loadFeed();
      if (count === 0) {
        statusEl.textContent = `${new Date().toISOString()} no cards available right now`;
      }
    }
    window.scrollTo({ top: 0, behavior: 'smooth' });
  } catch (e) {
    statusEl.textContent = `${new Date().toISOString()} next batch failed: ${e.message}`;
  }
});

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
    try {
      const action = e.target.dataset.action;
      if (action === 'dont') {
        const suggested = (cardEl.querySelector('.card-title')?.textContent || '').trim();
        const pattern = prompt('Pattern to hide (text/domain):', suggested);
        if (!pattern) return;
        const penaltyIn = prompt('Penalty weight:', String(defaultHidePenalty));
        const penalty = Number(penaltyIn);
        if (!Number.isFinite(penalty) || penalty <= 0) return;
        await api('/api/articles/dontshow', { method: 'POST', body: JSON.stringify({ id, pattern, penalty }) });
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
        await api('/api/articles/dontshow', { method: 'POST', body: JSON.stringify({ id, pattern, penalty }) });
      } else {
        await api('/api/articles/action', { method: 'POST', body: JSON.stringify({ id, action }) });
      }
      cardEl.remove();
      currentIds = currentIds.filter(v => v !== id);
      statusEl.textContent = `${new Date().toISOString()} action applied`;
    } catch (err) {
      statusEl.textContent = `${new Date().toISOString()} action failed: ${err.message}`;
    }
    return;
  }

  if (e.target.closest('[data-click]')) {
    try {
      await api('/api/articles/click', { method: 'POST', body: JSON.stringify({ id }) });
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
    authenticated = true;
    setAuthUI();
    await loadFeed();
  } catch (_) {
    authenticated = false;
    csrfToken = '';
    setAuthUI();
    statusEl.textContent = `${new Date().toISOString()} sign in to load your feed`;
  }
})();
