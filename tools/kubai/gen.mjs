// Draws the Kubai icons the app shows (https://github.com/kubai087/kubai-icons,
// its icons and packages MIT) as bitmaps: MyGo's SVG leaves out filters,
// which give these icons their soft shadow and highlight. Each icon is
// drawn at rest, soft, and as it is under the pointer or chosen, in the
// app's accent and in its hover pose.
//
//   node tools/kubai/gen.mjs <path to a checkout of kubai-icons>
//
// Needs Playwright's Chromium. The names are those of names.txt.
import { readFileSync, writeFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { pathToFileURL } from 'node:url';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const src = resolve(process.argv[2] || '');
const out = join(here, '..', '..', 'kubai');
const accent = '#5B5BD6', px = 96;
const { renderSVG } = await import(pathToFileURL(join(src, 'packages/core/src/render.js')));
const { chromium } = createRequire(import.meta.url)('playwright');
const names = readFileSync(join(here, 'names.txt'), 'utf8').split(/\s+/).filter(Boolean);
const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: px, height: px }, deviceScaleFactor: 1 });
for (const name of names) {
  const def = (await import(pathToFileURL(join(src, 'icons', name + '.js')))).default;
  for (const [suffix, opts] of [['', { variant: 'soft' }], ['-on', { variant: 'color', hover: true }]]) {
    const svg = renderSVG(def, { accent, size: px, ...opts });
    await page.setContent(`<body style="margin:0;background:transparent">${svg}</body>`);
    writeFileSync(join(out, name + suffix + '.png'), await page.screenshot({ omitBackground: true }));
  }
}
await browser.close();
console.log(names.length, 'icons');
