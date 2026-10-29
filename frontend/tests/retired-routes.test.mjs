import test from 'node:test';
import assert from 'node:assert/strict';
import { readdir, readFile } from 'node:fs/promises';
import { join } from 'node:path';
import {
  dashboardRoutes,
  navigationGroups,
  searchNavigation
} from '../src/lib/navigation.ts';
import { load as loadProvidersPage } from '../src/routes/dashboard/providers/+page.ts';

test('retired route /dashboard/providers is absent from dashboardRoutes', () => {
  assert.equal(
    dashboardRoutes.includes('/dashboard/providers'),
    false,
    '/dashboard/providers must not be in active dashboardRoutes'
  );
});

test('retired route /dashboard/providers is absent from all navigationGroups items', () => {
  const allItemPaths = navigationGroups.flatMap(g => g.items.map(i => i.path));
  assert.equal(
    allItemPaths.includes('/dashboard/providers'),
    false,
    '/dashboard/providers must not be in navigationGroups items'
  );
});

test('command palette searchNavigation does not return retired /dashboard/providers route', () => {
  const providerResults = searchNavigation('provider');
  assert.equal(
    providerResults.some(item => item.path === '/dashboard/providers'),
    false,
    'searchNavigation("provider") must not include /dashboard/providers'
  );
  // Canonical destinations should still be found
  assert.equal(
    providerResults.some(item => item.path === '/dashboard/connections'),
    true,
    'searchNavigation("provider") should include /dashboard/connections'
  );
});

test('no active links to /dashboard/providers exist in frontend components and pages', async () => {
  const root = new URL('../src', import.meta.url).pathname;

  async function checkDir(dir) {
    const entries = await readdir(dir, { withFileTypes: true });
    for (const entry of entries) {
      const fullPath = join(dir, entry.name);
      if (entry.isDirectory()) {
        await checkDir(fullPath);
      } else if (entry.name.endsWith('.svelte') || entry.name.endsWith('.ts')) {
        // Skip the retired page directory itself
        if (fullPath.includes('/routes/dashboard/providers/')) continue;
        const content = await readFile(fullPath, 'utf8');
        // Match href="/dashboard/providers"
        assert.equal(
          /href=["']\/dashboard\/providers["']/.test(content),
          false,
          `Found dead link to /dashboard/providers in ${fullPath}`
        );
      }
    }
  }

  await checkDir(root);
});

test('client-side route /dashboard/providers/+page.ts redirects to canonical /dashboard/connections', () => {
  let redirected = false;
  try {
    loadProvidersPage();
  } catch (err) {
    // SvelteKit redirect throws an object or Response with status and location
    if (err && (err.status === 308 || err.status === 307 || err.status === 301 || err.status === 302)) {
      assert.equal(err.location, '/dashboard/connections');
      redirected = true;
    } else {
      throw err;
    }
  }
  assert.equal(redirected, true, 'load() must throw redirect to /dashboard/connections');
});
