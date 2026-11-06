const { test: base, expect } = require('../../scripts/browser-tools.cjs');
const { startFixture } = require('./fixture.cjs');
const { readFileSync } = require('node:fs');
const path = require('node:path');
const version = readFileSync(path.resolve(__dirname, '../../internal/buildinfo/buildinfo.go'), 'utf8').match(/Version = "([^"]+)"/)[1];

const test = base.extend({
  app: async ({}, use) => {
    const app = await startFixture();
    try { await use(app); } finally { await app.close(); }
  },
});

for (const role of ['feed', 'admin']) {
  test(`${role}: aborted old session response cannot replace new login cookies`, async ({page,context,app}) => {
    const admin = role === 'admin';
    const prefix = admin ? '/admin/api' : '/api';
    const credentials = admin ? {secret:app.admin} : {username:'fixture-reader',secret:app.secret};
    expect((await context.request.post(app.url + prefix + '/login', {data:credentials})).ok()).toBe(true);
    let ready, release, finished;
    const started = new Promise(resolve => {ready=resolve});
    const gate = new Promise(resolve => {release=resolve});
    const delivered = new Promise(resolve => {finished=resolve});
    await page.route(`**${prefix}/session`, async route => {
      const old = await route.fetch();
      ready();
      await gate;
      try { await route.fulfill({response:old}); } finally { finished(); }
    });
    await page.goto(app.url + (admin ? '/admin' : '/'));
    await started;
    if (admin) {
      await page.locator('#secret').fill(app.admin);
      await page.locator('#loginBtn').click();
      await expect(page.locator('#countsPanel')).toBeVisible();
    } else {
      await page.locator('#userName').fill('fixture-reader');
      await page.locator('#userSecret').fill(app.secret);
      await page.locator('#userLoginBtn').click();
      await expect(page.locator('.card')).toHaveCount(5);
    }
    release(); await delivered;
    const csrf = await page.evaluate(() => csrfToken);
    const response = await context.request.post(app.url + prefix + (admin ? '/topics' : '/articles/action'), {
      headers:{'X-CSRF-Token':csrf}, data:admin ? {query:'fixture news',weight:10,enabled:true} : {id:1,action:'up'}
    });
    expect(response.status()).toBe(200);
    await expect(page.locator(admin ? '#logoutBtn' : '#userLogoutBtn')).toBeVisible();
  });
}

test('topic and rule saves retain later drafts in the rendered editor', async ({page,app}) => {
  await page.goto(app.url+'/admin');
  await page.locator('#secret').fill(app.admin);
  await page.locator('#loginBtn').click();
  await expect(page.locator('#addTopic')).toBeEnabled();
  for (const kind of ['Topic','Rule']) {
    const topic=kind==='Topic', endpoint=topic?'topics':'rules', field=topic?'topicQ':'ruleP';
    await page.locator(topic?'#topicsPanel summary':'#rulesPanel summary').click();
    let ready,release;
    const entered=new Promise(r=>{ready=r}), gate=new Promise(r=>{release=r});
    await page.route(`**/admin/api/${endpoint}`,async route=>{
      if(route.request().method()!=='POST')return route.continue();
      const response=await route.fetch(); ready(); await gate; await route.fulfill({response});
    });
    await page.locator('#'+field).fill('Saved '+kind);
    await page.locator('#add'+kind).click(); await entered;
    await expect(page.locator('#add'+kind)).toBeDisabled();
    await page.locator('#'+field).fill('New unsaved '+kind);
    release();
    await expect(page.locator('#add'+kind)).toBeEnabled();
    await expect(page.locator('#'+field)).toHaveValue('New unsaved '+kind);
  }
});

test('real embedded feed/admin assets, auth, stories and actions', async ({ page, context, app }, info) => {
  console.log(`Chromium ${context.browser().version()}`);
  const errors = [], external = [], assets = new Set();
  page.on('pageerror', err => errors.push(err.message));
  page.on('console', msg => {
    // Anonymous session probes intentionally return 401 before sign-in.
    if (msg.type() === 'error' && !/401 \(Unauthorized\)/.test(msg.text())) errors.push(msg.text());
  });
  page.on('response', response => {
    if (new URL(response.url()).pathname.startsWith('/assets/')) {
      expect(response.status()).toBe(200);
      assets.add(new URL(response.url()).pathname);
    }
  });
  await context.route('**/*', route => {
    if (new URL(route.request().url()).origin === app.url) return route.continue();
    external.push(route.request().url());
    return route.abort();
  });
  expect((await context.request.get(`${app.url}/api/feed`)).status()).toBe(401);
  await page.goto(app.url);
  await page.locator('#userName').fill('fixture-reader');
  await page.locator('#userSecret').fill(app.secret);
  await page.locator('#userLoginBtn').click();
  await expect(page.locator('.card')).toHaveCount(5);
  await expect(page.locator('#userBuildVersion')).toHaveText(version);
  await expect(page.locator('#userLoginBtn')).toBeHidden();
  await expect(page.locator('.card-source').first()).toContainText('today');
  const story = page.locator('.card').first();
  await story.locator('.story-sources summary').click();
  await expect(story.locator('.story-sources a')).toHaveCount(1);
  await expect.poll(() => story.locator('img').evaluate(img => img.complete && img.naturalWidth > 0)).toBe(true);
  await story.locator('[data-menu]').click();
  await story.locator('[data-action="up"]').click();
  await expect(story.locator('.menu')).not.toHaveClass(/open/);
  await expect(page.locator('.card')).toHaveCount(5);
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: info.outputPath('feed.png'), fullPage: true });
  const readIDs = await page.locator('.card').evaluateAll(cards => [Number(cards[0].dataset.id), Number(cards[2].dataset.id)]);
  // Exercise authenticated click bookkeeping without navigating to real publishers.
  await page.evaluate(async ids => {
    for (const id of ids) await api('/api/articles/click', {method:'POST',body:JSON.stringify({id})});
  }, readIDs);
  const hideCard = page.locator('.card').nth(1);
  const hiddenID = await hideCard.getAttribute('data-id');
  page.on('dialog', dialog => dialog.accept(dialog.type() === 'prompt' ? dialog.defaultValue() : undefined));
  await hideCard.locator('[data-menu]').click();
  await hideCard.locator('[data-action="domain"]').click();
  await expect(page.locator(`.card[data-id="${hiddenID}"]`)).toHaveCount(0);
  await page.locator('#nextBtn').click();
  await expect(page.locator('#nextBtn')).toBeEnabled();
  await expect.poll(() => page.evaluate(() => scrollY)).toBe(0);
  await page.reload();
  await expect(page.locator('#userLogoutBtn')).toBeVisible();

  await page.goto(`${app.url}/admin`);
  await expect(page.locator('#topicsPanel')).toBeHidden();
  await expect(page.locator('#enginePanel')).toBeHidden();
  await page.locator('#secret').fill(app.admin);
  await page.locator('#loginBtn').click();
  await expect(page.locator('#countsPanel')).toBeVisible();
  await expect(page.locator('#ingestState')).toContainText(version);
  await expect(page.locator('#nextScheduled')).toContainText('Next scheduled ingestion:');
  await expect(page.locator('#engineCheckState')).toContainText('Not checked');
  const countsBeforeCheck = await page.locator('#counts').textContent();
  await page.locator('#checkEngines').click();
  await expect(page.locator('#engineCheckState')).toContainText('running in the background');
  await expect(page.locator('#checkEngines')).toBeDisabled();
  await expect(page.locator('#engineRows tr')).toHaveCount(2, {timeout:15000});
  await expect(page.locator('#engineRows')).toContainText('fixture: Returned results');
  await expect(page.locator('#engineCheckState')).toContainText('Last check:');
  await expect(page.locator('#checkEngines')).toContainText('Cooling down');
  await expect(page.locator('#counts')).toHaveText(countsBeforeCheck);
  await expect(page.locator('#archiveDays')).toHaveValue('30');
  await page.locator('#archiveDays').fill('90');
  await page.locator('#previewArchive').click();
  await expect(page.locator('#archiveResult')).toContainText('Would newly archive');
  await expect(page.locator('#archiveResult')).toContainText('restore 0 age-archived rows');
  await page.locator('#applyArchive').click();
  await expect(page.locator('#archiveResult')).toContainText('Feed age limit: 90 days');
  await page.locator('#domainsPanel summary').click();
  await page.locator('#loadDomains').click();
  await expect(page.locator('#domainRows tr')).toHaveCount(1);
  await expect(page.locator('#domainRows')).toContainText('example.test');
  await expect(page.locator('#domainRows tr td').nth(2)).toHaveText('2');
  await expect(page.locator('#domainsPanel th').nth(4)).toHaveText('Hidden');
  await expect(page.locator('#domainsPanel th').nth(5)).toHaveText('Shown');
  await page.locator('#topicsPanel summary').click();
  await expect(page.locator('#topics')).toContainText('fixture news');
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: info.outputPath('admin.png'), fullPage: true });
  await page.locator('#logoutBtn').click();
  await expect(page.locator('#topicsPanel')).toBeHidden();
  await expect(page.locator('#enginePanel')).toBeHidden();
  await expect(page.locator('#engineRows tr')).toHaveCount(0);
  expect(assets).toEqual(new Set(['/assets/style.css', '/assets/feed.js', '/assets/admin.js']));
  expect(errors).toEqual([]);
  expect(external).toEqual([]);
  expect(app.requests).toHaveLength(10);
  for (const request of app.requests.slice(8)) {
    expect(request.searchParams.get('time_range')).toBe('day');
    expect(request.searchParams.get('pageno')).toBe('1');
    expect(request.searchParams.get('q')).toBe('intel cpu');
  }
  for (const request of app.requests) {
    expect(['day', 'week']).toContain(request.searchParams.get('time_range'));
    expect(['news', 'general']).toContain(request.searchParams.get('categories'));
  }
});
