# Laya gateway

Go API on `0.0.0.0:7505`. The command desk, the extension sidebar, and the extension service worker talk only to this process. It keeps one transcript and snapshot per open tab, asks the Python brain which action to run, and queues that action only when the tab is marked available.

Destructive-looking actions stay held until `POST /v1/chat` with `confirmed: true`. Navigations that are not `http://` or `https://` are dropped.

```bash
LISTEN=0.0.0.0:7505 AGENT_URL=http://127.0.0.1:7506 go run .
```

`GET /demo` is a practice page with a search box, a color menu, Login, and Delete account.

Set `API_TOKEN` to require the `X-Api-Token` header on every route except `/health` and `/demo`.
