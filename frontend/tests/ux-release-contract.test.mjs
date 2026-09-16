import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';

const read = path => readFile(new URL(path, import.meta.url), 'utf8');

test('mobile command-center controls declare 44px touch targets', async () => {
  const [header, sidebar, palette, connections, overview] = await Promise.all([
    read('../src/lib/components/Header.svelte'), read('../src/lib/components/Sidebar.svelte'),
    read('../src/lib/components/CommandPalette.svelte'), read('../src/routes/dashboard/connections/+page.svelte'),
    read('../src/routes/dashboard/+page.svelte'),
  ]);
  assert.match(header, /@media \(max-width: 768px\)[\s\S]*min-(?:width|height):\s*44px/);
  assert.match(sidebar, /@media \(max-width: 768px\)[\s\S]*min-height:\s*44px/);
  assert.match(palette, /@media \(max-width:\s*640px\)[\s\S]*44px/);
  assert.match(connections, /@media \(max-width: 640px\)[\s\S]*\.filter-chip[\s\S]*min-height:\s*44px/);
  assert.match(connections, /@media \(max-width: 640px\)[\s\S]*\.conn-action-btn[\s\S]*min-(?:width|height):\s*44px/);
  assert.match(overview, /@media\(max-width:600px\)[\s\S]*\.refresh\{[^}]*width:44px[^}]*min-height:44px[^}]*flex-shrink:0/);
});

test('header interactions use semantic theme tokens instead of light-only literals', async () => {
  const header = await read('../src/lib/components/Header.svelte');
  const style = header.slice(header.indexOf('<style>'));
  for (const literal of ['#e2e8f0', '#64748b', '#f8fafc', '#1e293b']) {
    assert.equal(style.includes(literal), false, `found hard-coded ${literal}`);
  }
  assert.match(style, /var\(--color-border\)/);
  assert.match(style, /var\(--color-fg-2\)/);
  assert.match(style, /var\(--color-bg-hover\)/);
});