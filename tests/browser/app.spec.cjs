const { test: base, expect } = require('@playwright/test');
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

test('real embedded feed/admin assets, auth, stories and actions', async ({ page, context, app }, info) => {
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
  await page.locator('#secret').fill(app.admin);
  await page.locator('#loginBtn').click();
  await expect(page.locator('#countsPanel')).toBeVisible();
  await expect(page.locator('#ingestState')).toContainText(version);
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
  expect(assets).toEqual(new Set(['/assets/style.css', '/assets/feed.js', '/assets/admin.js']));
  expect(errors).toEqual([]);
  expect(external).toEqual([]);
  expect(app.requests).toHaveLength(8);
  for (const request of app.requests) {
    expect(['day', 'week']).toContain(request.searchParams.get('time_range'));
    expect(['news', 'general']).toContain(request.searchParams.get('categories'));
  }
});
