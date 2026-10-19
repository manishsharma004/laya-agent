# Laya chat

Svelte chat for the page agent. It talks to the Go gateway, shows whether Laya is loaded, and lists the controls the extension last reported.

```bash
npm install
npm run dev
```

The dev server binds to `0.0.0.0:7500`. The API address defaults to `http://<this-host>:7501` and can be changed from Session. Keep the session id aligned with the extension popup (`local` unless you change both).
