// Shared development tooling only. Requiring an absolute path avoids a stale
// repository-local installation silently shadowing the operator's selected one.
const path = require('node:path');
const os = require('node:os');
const fs = require('node:fs');
const location = process.env.PLAYWRIGHT_MODULE || path.join(os.homedir(), 'dev/playwright-tools/node_modules/playwright');
if (!path.isAbsolute(location)) throw new Error('PLAYWRIGHT_MODULE must be an absolute Playwright package directory.');
const manifest = require(path.join(location, 'package.json'));
if (manifest.name !== 'playwright') throw new Error('Expected the shared playwright package.');
const toolkit = require(path.join(location, 'test.js'));
module.exports = toolkit;
if (require.main === module) {
  const browser = toolkit.chromium.executablePath();
  console.log(`Shared Playwright ${manifest.version}; Node ${process.version}`);
  if (!fs.existsSync(browser)) throw new Error('Matching shared Chromium is missing; ask the operator to install it in playwright-tools.');
  console.log('Matching Chromium is installed. No tools or OS packages changed.');
}
