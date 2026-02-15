let currentIds = [];
const feed = document.getElementById('feed');
const nextBtn = document.getElementById('nextBtn');
const statusEl = document.getElementById('status');

async function api(url, opts = {}) {
  const res = await fetch(url, { ...opts, headers: { 'Content-Type': 'application/json', ...(opts.headers || {}) } });
  const j = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(j.error || res.statusText || `HTTP ${res.status}`);
  return j;
}

function esc(s) {
  return String(s || '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}

function card(item) {
  const img = item.thumbnail_url ? `<img class="thumb" src="${esc(item.thumbnail_url)}" alt="">` : '';
  return `<article class="card" data-id="${item.id}">
    ${img}
    <a class="card-link" href="${esc(item.url)}" target="_blank" rel="noopener" data-click="1">
      <div class="card-main">
        <h3 class="card-title">${esc(item.title)}</h3>
        <div class="card-source">${esc(item.source_domain || 'unknown')} | score ${Number(item.score).toFixed(2)}</div>
      </div>
    </a>
    <div class="menu"><button data-menu="1">⋯</button><div class="menu-panel">
      <button data-action="up">👍 Useful</button>
      <button data-action="down">👎 Hide</button>
      <button data-action="dont" class="danger">🚫 Don't show</button>
    </div></div>
  </article>`;
}

async function loadFeed() {
  try {
    const data = await api('/api/feed');
    const items = data.items || [];
    currentIds = items.map(i => i.id);
    feed.innerHTML = items.map(card).join('');
    statusEl.textContent = `${new Date().toISOString()} loaded ${items.length} cards`;
  } catch (e) {
    statusEl.textContent = `${new Date().toISOString()} feed load failed: ${e.message}`;
  }
}

nextBtn.addEventListener('click', async () => {
  try {
    if (currentIds.length) await api('/api/feed/seen', { method: 'POST', body: JSON.stringify({ ids: currentIds }) });
    await loadFeed();
  } catch (e) {
    statusEl.textContent = `${new Date().toISOString()} next batch failed: ${e.message}`;
  }
});

feed.addEventListener('click', async (e) => {
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
        const pattern = prompt('Negative pattern to block:');
        if (!pattern) return;
        await api('/api/articles/dontshow', { method: 'POST', body: JSON.stringify({ id, pattern, penalty: 10 }) });
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

loadFeed();

document.addEventListener('click', (e) => {
  if (!e.target.closest('.menu')) {
    document.querySelectorAll('.menu.open').forEach((m) => m.classList.remove('open'));
  }
});
