#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
export PATH="$ROOT/.tools/go/bin:$PATH"
(cd "$ROOT/brain" && PYTHONPATH=. python3 -m unittest discover -s tests -v)
(cd "$ROOT/gateway" && go test ./...)
(cd "$ROOT/extension" && npm test)
(cd "$ROOT/chat" && npm run build)
