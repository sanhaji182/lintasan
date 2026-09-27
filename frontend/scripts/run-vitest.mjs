import { mkdtemp, rm } from 'node:fs/promises';
import { constants as osConstants, tmpdir } from 'node:os';
import { join } from 'node:path';
import { spawn } from 'node:child_process';

const runTmpDir = await mkdtemp(join(tmpdir(), 'lintasan-vitest-'));

function forward(signal, child) {
	if (!child.killed) child.kill(signal);
}

try {
	const child = spawn(process.execPath, ['./node_modules/vitest/vitest.mjs', 'run', ...process.argv.slice(2)], {
		cwd: process.cwd(),
		env: { ...process.env, TMPDIR: runTmpDir },
		stdio: 'inherit'
	});

	const onSigint = () => forward('SIGINT', child);
	const onSigterm = () => forward('SIGTERM', child);
	process.once('SIGINT', onSigint);
	process.once('SIGTERM', onSigterm);

	const result = await new Promise((resolve, reject) => {
		child.once('error', reject);
		child.once('exit', (code, signal) => resolve({ code, signal }));
	});

	process.removeListener('SIGINT', onSigint);
	process.removeListener('SIGTERM', onSigterm);

	if (result.signal) {
		process.exitCode = 128 + ((osConstants.signals[result.signal] ?? 0) || 1);
	} else {
		process.exitCode = result.code ?? 1;
	}
} finally {
	await rm(runTmpDir, { recursive: true, force: true });
}
