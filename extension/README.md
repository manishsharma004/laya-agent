# Laya Page Control

Manifest V3 Chrome extension. The content script numbers the interactive controls on the active tab. The service worker posts that snapshot to the gateway and runs the commands that come back: click, type, select, scroll, navigate, or extract.

Load it unpacked from this directory (`chrome://extensions` → Developer mode → Load unpacked). The popup sets the gateway address (default `http://127.0.0.1:7501`), the session id (default `local`), and an optional API token. Use the machine's LAN address if the browser is on another computer; the gateway listens on `0.0.0.0`.

Password values are omitted from snapshots. Navigation only follows `http` and `https` URLs. Reload a tab once after installing so the content script attaches.

```bash
npm install
npm test
```
