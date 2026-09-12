const { defineConfig, devices } = require('@playwright/test');

module.exports = defineConfig({
  testDir: './tests/browser',
  timeout: 45000,
  workers: 1,
  retries: 0,
  outputDir: `.browser-artifacts/${process.env.DISCOVER_BROWSER_MODE || 'production'}`,
  reporter: 'list',
  use: { actionTimeout: 10000, trace: 'off', video: 'off', screenshot: 'only-on-failure', serviceWorkers: 'block' },
  projects: [
    { name: 'desktop', use: { ...devices['Desktop Chrome'], viewport: { width: 1280, height: 900 } } },
    { name: 'mobile', use: { ...devices['Pixel 7'] } },
  ],
});
