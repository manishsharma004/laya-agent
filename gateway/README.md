# Laya gateway

Go API on `0.0.0.0:7501`. The chat UI and the Chrome extension talk only to this process. It stores the latest page snapshot per session, asks the Python brain which action to run, and queues that action for the extension.

Destructive-looking actions stay held until `POST /v1/chat` with `confirmed: true`. Navigations that are not `http://` or `https://` are dropped.

```bash
LISTEN=0.0.0.0:7501 AGENT_URL=http://127.0.0.1:7502 go run .
```

`GET /demo` is a practice page with a search box, a color menu, Login, and Delete account.

Set `API_TOKEN` to require the `X-Api-Token` header on every route except `/health` and `/demo`.
