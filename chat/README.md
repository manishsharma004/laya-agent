# Laya command desk

Svelte UI for sending commands and choosing tabs. The left column lists every tab the extension has reported. A checked tab is available for the extension to control. Selecting a row addresses the composer at that tab.

The extension sidebar is separate: it always talks about the active tab. Unchecking the active tab there stops the extension from acting on it until it is checked again.

```bash
npm install
npm run dev
```

The dev server binds to `0.0.0.0:7504`. The API address defaults to `http://<this-host>:7505` and can be changed from Session. Keep the session id aligned with the extension popup (`local` unless you change both).
