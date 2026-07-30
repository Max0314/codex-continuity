import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

test('shows the ICP filing link on the public login screen', async () => {
  const [app, styles] = await Promise.all([
    readFile(new URL('../src/App.tsx', import.meta.url), 'utf8'),
    readFile(new URL('../src/styles.css', import.meta.url), 'utf8'),
  ]);

  assert.match(app, /渝ICP备2026016967号-1/);
  assert.match(app, /https:\/\/beian\.miit\.gov\.cn\//);
  assert.match(styles, /\.filing-footer/);
});
