# Wire protocol

The chat UI, Chrome extension, Go gateway, and Python brain share one JSON contract. Session ids match `[A-Za-z0-9_-]{1,64}`. The default session is `local`.

Ports when started with `scripts/dev.sh`:

| Service | Address |
| --- | --- |
| Chat | `http://0.0.0.0:7504` |
| Gateway | `http://0.0.0.0:7505` |
| Brain | `http://0.0.0.0:7506` |

If `API_TOKEN` is set on the gateway, clients send it as `X-Api-Token`.

## Extension → gateway

`POST /v1/extension/tabs` replaces the open-tab inventory. New tabs start available. Tabs omitted from the next post are closed. Omit nothing: send every http(s) tab. `elements` is the control list when the tab is available, and an empty list when it is not.

```json
{
  "sessionId": "local",
  "activeTabId": 12,
  "tabs": [
    {"tabId": 12, "windowId": 1, "url": "https://example.com", "title": "Example", "active": true, "elements": []}
  ]
}
```

The response includes each tab's `available` flag. The extension stores those ids and only snapshots and executes on available tabs.

`POST /v1/tabs/availability` with `{ "sessionId", "tabId", "available" }` is how the command desk turns a tab on or off.

`POST /v1/extension/snapshot` still records one tab and marks it active. Prefer `/v1/extension/tabs`.

Legacy `POST /v1/extension/snapshot`

```json
{
  "sessionId": "local",
  "tabId": 12,
  "url": "https://example.com",
  "title": "Example",
  "elements": [
    {
      "id": "e1",
      "role": "button",
      "name": "Login",
      "tag": "button",
      "editable": false,
      "inputType": "",
      "value": "",
      "href": ""
    }
  ]
}
```

`GET /v1/extension/commands?sessionId=local` returns queued commands and marks them delivered:

```json
{ "commands": [{ "id": "…", "tabId": 12, "type": "click", "elementId": "e1", "text": "", "url": "" }] }
```

`POST /v1/extension/result`

```json
{ "sessionId": "local", "commandId": "…", "ok": true, "detail": "Clicked Login." }
```

Action `type` is one of `click`, `type`, `select`, `scroll`, `navigate`, `extract`.

## Chat → gateway

`POST /v1/chat`

```json
{ "sessionId": "local", "tabId": 12, "message": "Click Login", "confirmed": false }
```

`tabId` selects the tab. When it is omitted, the gateway uses the active tab. A tab that is not available returns 409. Transcripts are stored per tab. `GET /v1/sessions/{id}?tabId=12` returns that tab's messages plus the full tab list.

A confirmation resend uses `{ "sessionId": "local", "confirmed": true }` and runs the action the brain held back.

`GET /v1/sessions/{id}` returns messages, the latest page summary, extension freshness, the held action, and the last command result.

`GET /v1/status` reports gateway health and the brain's Laya status.

## Gateway → brain

`POST /decide`

```json
{
  "prompt": "Click Login",
  "confirmed": false,
  "history": [{ "role": "user", "content": "Click Login" }],
  "page": { "url": "https://example.com", "title": "Example", "elements": [] }
}
```

```json
{
  "reply": "Click “Login”.",
  "backend": "laya",
  "layaStatus": "ready",
  "needsConfirmation": false,
  "action": { "type": "click", "elementId": "e1", "text": "", "url": "" },
  "decision": { "intent": "control_page", "operation": "click", "target": "e1", "confidence": 0.8 }
}
```

`backend` is `laya` when a Laya checkpoint answered the questions, and `heuristic` when the checkpoint is still loading or unavailable. The action shape stays the same either way.

`GET /health` on the brain returns `{ "ok": true, "laya": "ready|loading|unavailable|disabled", "mode": "auto" }`.
