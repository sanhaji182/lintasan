# Muse Cookie Bridge (Chrome extension)

Load-unpacked Chrome/Edge extension for importing muse.ai session cookies into
the Lintasan **Muse Accounts** panel.

It reads only these cookies from a logged-in muse.ai session:

- `hatch_sess`
- `hatch_gw`
- `hatch_vml`
- `hatch_native_auth_device`

The extension sends them to your Lintasan server with a **one-time import
token**. It never sees the muse2api admin key and does not store the token or
cookie values.

## Install

1. Open `chrome://extensions` (or `edge://extensions`).
2. Enable **Developer mode**.
3. Click **Load unpacked** and select this `tools/muse-cookie-extension` folder.
4. Pin **Lintasan — Muse Cookie Bridge** to the toolbar.

The extension is not published to the Chrome Web Store; load it unpacked from
the Lintasan repository.

## Use

1. Log in to <https://muse.ai> in the same browser.
2. In Lintasan open **Settings → Account Connection → Muse Accounts**.
3. Click **Generate import token** (valid ~10 minutes, single use).
4. Open the extension, set the Lintasan URL, paste the token, optionally set a
   label, then click **Capture & import**.
5. Return to the Muse panel and click **Renew session**. The account should show
   **Active**.

The first import may ask for permission to call your Lintasan origin; this is
the browser's optional host-permission prompt.

## Notes

- Tokens are single use. If an import fails, generate a new token.
- If the popup reports missing cookies, log in to muse.ai again and retry; the
  token is not consumed when local cookies are incomplete.
- Cookies are stored by muse2api, not by the extension or by Lintasan.
- Use only accounts you are authorized to connect. muse.ai session cookies grant
  full access to the account.
