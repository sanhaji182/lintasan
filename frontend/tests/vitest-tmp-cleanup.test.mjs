import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const frontendDir = join(dirname(fileURLToPath(import.meta.url)), '..');

/**
 * Vitest creates a per-run temp dir under TMPDIR and only removes it on a clean
 * close. A killed or timed-out run leaks it. Runs must go through
 * scripts/run-vitest.mjs so the temp dir is isolated and reaped.
 */
function vitestInvocationsWithoutWrapper(pkgJsonText) {
	const pkg = JSON.parse(pkgJsonText);
	const offenders = [];
	for (const [name, cmd] of Object.entries(pkg.scripts ?? {})) {
		if (typeof cmd !== 'string') continue;
		const segments = cmd.split('&&').map((s) => s.trim());
		for (const segment of segments) {
			const runsVitestDirectly = /(^|\s)vitest(\s|$)/.test(segment);
			const goesThroughWrapper = segment.includes('scripts/run-vitest.mjs');
			if (runsVitestDirectly && !goesThroughWrapper) offenders.push({ name, segment });
		}
	}
	return offenders;
}

test('test scripts never invoke vitest directly (regression: TMPDIR leak)', () => {
	const current = readFileSync(join(frontendDir, 'package.json'), 'utf8');
	assert.deepEqual(vitestInvocationsWithoutWrapper(current), []);
});

test('detects a pre-fix package.json that leaks TMPDIR', () => {
	const before = JSON.stringify({
		scripts: {
			test: 'node --test tests/*.test.mjs && vitest run tests/*.test.ts',
			'test:unit': 'vitest run tests/*.test.ts'
		}
	});
	const offenders = vitestInvocationsWithoutWrapper(before);
	assert.ok(
		offenders.length > 0,
		'checker must flag a direct vitest invocation, otherwise it cannot guard this bug'
	);
});

test('run-vitest.mjs isolates TMPDIR and removes it afterwards', () => {
	const src = readFileSync(join(frontendDir, 'scripts', 'run-vitest.mjs'), 'utf8');
	assert.match(src, /mkdtemp\(/, 'wrapper must create an isolated temp dir');
	assert.match(src, /TMPDIR/, 'wrapper must point the child at that isolated temp dir');
	assert.match(src, /await rm\(runTmpDir/, 'wrapper must remove the temp dir in a finally block');
	assert.doesNotMatch(
		src,
		/process\.kill\(process\.pid/,
		'wrapper must not terminate itself before the finally cleanup completes'
	);
});