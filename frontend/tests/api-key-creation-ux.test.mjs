import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';

import {
  captureOneTimeSecret,
  clearOneTimeSecret,
  maskedKeyLabel,
} from '../src/lib/api-key-secret.ts';

const pageSource = await readFile(new URL('../src/routes/dashboard/keys/+page.svelte', import.meta.url), 'utf8');
const quickstartSource = await readFile(new URL('../src/routes/dashboard/quickstart/+page.svelte', import.meta.url), 'utf8');
const docsSource = await readFile(new URL('../src/routes/dashboard/docs/+page.svelte', import.meta.url), 'utf8');

test('successful creation captures the POST plaintext only in ephemeral state', () => {
  const response = {
    id: 'key-1',
    name: 'Deploy bot',
    key: 'sk-lintasan-full-one-time-secret',
    created_at: '2026-09-15T12:00:00Z',
  };

  const secret = captureOneTimeSecret(response);

  assert.deepEqual(secret, {
    id: 'key-1',
    name: 'Deploy bot',
    value: 'sk-lintasan-full-one-time-secret',
  });
  assert.equal(clearOneTimeSecret(), null);
});

test('a creation response without a plaintext key cannot produce a fake credential', () => {
  assert.equal(captureOneTimeSecret({ id: 'key-1', prefix: 'sk-l…cret' }), null);
  assert.equal(captureOneTimeSecret({ id: 'key-1', key: '' }), null);
});

test('existing key rows are explicitly masked and never represented as credentials', () => {
  assert.equal(maskedKeyLabel({ prefix: 'sk-l…cret' }), 'sk-l…cret (masked)');
  assert.equal(maskedKeyLabel({}), 'Configured (masked)');
});

test('API Keys page owns the one-time dialog lifecycle without browser persistence', () => {
  assert.match(pageSource, /role="dialog"/);
  assert.match(pageSource, /cannot be retrieved again/i);
  assert.match(pageSource, /captureOneTimeSecret\(created\)/);
  assert.match(pageSource, /oneTimeSecret = clearOneTimeSecret\(\)/);
  assert.match(pageSource, /if \(oneTimeSecret\) await copySecret\(oneTimeSecret\.value\)/);
  assert.doesNotMatch(pageSource, /localStorage|sessionStorage|URLSearchParams|console\./);
  assert.doesNotMatch(pageSource, /copyKey\(k\.key|copyKey\(k\.prefix/);
});

test('Quickstart and docs direct key creation to the canonical API Keys page', () => {
  assert.doesNotMatch(quickstartSource, /api\.post<any>\('\/api\/keys'/);
  assert.match(quickstartSource, /href="\/dashboard\/keys"/);
  assert.match(docsSource, /Dashboard → API Keys → Create Key/);
  assert.match(docsSource, /shown once/i);
});
