# Laya Page Control

Manifest V3 Chrome extension. The toolbar icon opens a side panel. That sidebar is the conversation with the agent for the active tab.

The service worker also reports every open http(s) tab to the gateway. The command desk decides which of those tabs are available. The extension collects controls and runs actions only on available tabs. Actions are click, type, select, scroll, navigate, and extract.

Load it unpacked from this directory (`chrome://extensions` → Developer mode → Load unpacked). Connection settings live at the bottom of the sidebar: gateway address (default `http://127.0.0.1:7505`), session id (default `local`), and an optional API token. Use the machine's LAN address if the browser is on another computer; the gateway listens on `0.0.0.0`.

Password values are omitted from snapshots. Navigation only follows `http` and `https` URLs. Reload a tab once after installing so the content script attaches.

```bash
npm install
npm test
```
