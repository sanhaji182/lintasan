const REQUIRED_COOKIES = ['hatch_sess', 'hatch_gw', 'hatch_vml', 'hatch_native_auth_device'];
const DEFAULT_BASE = 'https://lintasan.sans.biz.id';

const baseInput = document.getElementById('base');
const tokenInput = document.getElementById('token');
const labelInput = document.getElementById('label');
const runButton = document.getElementById('run');
const statusBox = document.getElementById('status');

function setStatus(kind, message) {
  statusBox.className = kind || '';
  statusBox.textContent = message || '';
}

async function restoreBase() {
  const stored = await chrome.storage.local.get('lintasanUrl');
  baseInput.value = stored.lintasanUrl || DEFAULT_BASE;
  baseInput.addEventListener('change', () => {
    chrome.storage.local.set({ lintasanUrl: baseInput.value.trim() });
  });
}

async function readMuseCookies() {
  const found = {};
  const queries = [{ domain: 'muse.ai' }, { url: 'https://muse.ai' }, { url: 'https://www.muse.ai' }];
  for (const query of queries) {
    try {
      const cookies = await chrome.cookies.getAll(query);
      for (const cookie of cookies) {
        if (REQUIRED_COOKIES.includes(cookie.name) && !found[cookie.name]) {
          found[cookie.name] = cookie.value;
        }
      }
    } catch {
      // A single failed query must not abort the others.
    }
  }
  return found;
}

function errorMessage(data, fallback) {
  if (!data) return fallback;
  if (typeof data.error === 'string') return data.error;
  if (data.error && typeof data.error.message === 'string') return data.error.message;
  if (typeof data.detail === 'string') return data.detail;
  if (data.detail && typeof data.detail.message === 'string') return data.detail.message;
  return fallback;
}

async function captureAndImport() {
  setStatus('', '');
  const rawBase = baseInput.value.trim();
  const token = tokenInput.value.trim();
  const label = labelInput.value.trim();

  let origin;
  try {
    origin = new URL(rawBase).origin;
  } catch {
    setStatus('err', 'Lintasan URL is invalid.');
    return;
  }
  if (!token) {
    setStatus('err', 'Paste a one-time import token generated in the Muse panel.');
    return;
  }

  runButton.disabled = true;
  try {
    // Must be requested from the click gesture, before any other await.
    const granted = await chrome.permissions.request({ origins: [origin + '/*'] });
    if (!granted) {
      setStatus('err', `Permission to call ${origin} was not granted.`);
      return;
    }

    setStatus('', 'Reading muse.ai cookies…');
    const cookies = await readMuseCookies();
    const missing = REQUIRED_COOKIES.filter((name) => !cookies[name]);
    if (missing.length > 0) {
      setStatus(
        'warn',
        `Missing cookies: ${missing.join(', ')}. Log in to muse.ai in this browser, then retry. The token was not used.`
      );
      return;
    }

    setStatus('', 'Importing into Lintasan…');
    const response = await fetch(origin + '/muse/import', {
      method: 'POST',
      credentials: 'omit',
      headers: {
        'Content-Type': 'application/json',
        Authorization: 'Bearer ' + token
      },
      body: JSON.stringify({ label, cookies })
    });

    let data = null;
    try {
      data = await response.json();
    } catch {
      // Keep a null body; the status text is used below.
    }

    tokenInput.value = '';

    if (!response.ok) {
      const hint = response.status === 401 ? ' The token is invalid, expired, or already used — generate a new one.' : '';
      setStatus('err', errorMessage(data, `Import failed (HTTP ${response.status}).`) + hint);
      return;
    }

    const upstreamMissing = Array.isArray(data && data.missing_cookies) ? data.missing_cookies : [];
    const accountID = data && data.account && data.account.id ? data.account.id : 'account';
    if (upstreamMissing.length > 0) {
      setStatus('warn', `Imported ${accountID}, but muse2api still needs: ${upstreamMissing.join(', ')}.`);
    } else {
      setStatus('ok', `Imported ${accountID}. Check the Muse panel for account status.`);
    }
  } catch (error) {
    tokenInput.value = '';
    setStatus('err', 'Request failed: ' + (error && error.message ? error.message : String(error)) + '. Generate a new token to retry.');
  } finally {
    runButton.disabled = false;
  }
}

runButton.addEventListener('click', captureAndImport);
restoreBase();
