#!/usr/bin/env bash
# Exercise the gateway and brain with a fake page snapshot.
set -euo pipefail
API="${API:-http://127.0.0.1:7501}"
curl -sf "$API/health" >/dev/null
curl -sf -X POST "$API/v1/extension/snapshot" \
  -H 'content-type: application/json' \
  -d '{"sessionId":"smoke","url":"https://example.com/account","title":"Account","tabId":3,"elements":[{"id":"e1","role":"button","name":"Login","tag":"button","editable":false},{"id":"e2","role":"textbox","name":"Search","tag":"input","editable":true},{"id":"e3","role":"button","name":"Delete account","tag":"button","editable":false}]}' >/dev/null
CLICK="$(curl -sf -X POST "$API/v1/chat" -H 'content-type: application/json' -d '{"sessionId":"smoke","message":"click Login"}')"
python3 - "$CLICK" <<'PY'
import json, sys
body = json.loads(sys.argv[1])
action = body.get("command") or {}
assert action.get("type") == "click" and action.get("elementId") == "e1", body
assert body["needsConfirmation"] is False
print("click", body["backend"], body["reply"])
PY
HELD="$(curl -sf -X POST "$API/v1/chat" -H 'content-type: application/json' -d '{"sessionId":"smoke","message":"click Delete account"}')"
python3 - "$HELD" <<'PY'
import json, sys
body = json.loads(sys.argv[1])
assert body["needsConfirmation"] is True, body
assert body.get("command") is None
print("held", body["reply"])
PY
COMMANDS="$(curl -sf "$API/v1/extension/commands?sessionId=smoke")"
python3 - "$COMMANDS" <<'PY'
import json, sys
body = json.loads(sys.argv[1])
kinds = [item["type"] + ":" + item["elementId"] for item in body["commands"]]
assert kinds == ["click:e1"], kinds
print("queued", kinds)
PY
echo "smoke ok"
