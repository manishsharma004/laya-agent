#!/usr/bin/env bash
# Start the brain, gateway, and chat bound to 0.0.0.0.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
mkdir -p "$ROOT/.tools"
export PATH="$ROOT/.tools/go/bin:$PATH"

if ! command -v go >/dev/null 2>&1; then
  echo "Downloading a local Go toolchain into .tools/go"
  curl -fsSL "https://go.dev/dl/go1.23.6.linux-amd64.tar.gz" -o "$ROOT/.tools/go.tgz"
  rm -rf "$ROOT/.tools/go"
  tar -C "$ROOT/.tools" -xzf "$ROOT/.tools/go.tgz"
  export PATH="$ROOT/.tools/go/bin:$PATH"
fi

PY="$ROOT/brain/.venv/bin/python"
if [[ ! -x "$PY" ]]; then
  if ! python3 -m venv "$ROOT/brain/.venv"; then
    python3 -m venv --without-pip "$ROOT/brain/.venv"
    curl -fsSL https://bootstrap.pypa.io/get-pip.py | "$PY"
  fi
fi
if ! "$PY" -c "import laya" >/dev/null 2>&1; then
  echo "Installing CPU torch and laya into brain/.venv"
  "$ROOT/brain/.venv/bin/pip" install -U pip
  # --no-deps keeps pip from replacing the CPU torch wheel with a CUDA build.
  if "$ROOT/brain/.venv/bin/pip" install "torch>=2.2" --index-url https://download.pytorch.org/whl/cpu \
    && "$ROOT/brain/.venv/bin/pip" install "transformers>=4.48" "safetensors>=0.4" "huggingface_hub>=0.20" "numpy>=1.20" \
    && "$ROOT/brain/.venv/bin/pip" install -r "$ROOT/brain/requirements-laya.txt" --no-deps; then
    echo "Laya is installed. The first decision downloads a checkpoint."
  else
    echo "Laya did not install. The brain will answer with the heuristic backend."
  fi
fi

if [[ ! -d "$ROOT/chat/node_modules" ]]; then
  (cd "$ROOT/chat" && npm install)
fi

export BRAIN_HOST="${BRAIN_HOST:-0.0.0.0}"
export BRAIN_PORT="${BRAIN_PORT:-7506}"
export LISTEN="${LISTEN:-0.0.0.0:7505}"
export AGENT_URL="${AGENT_URL:-http://127.0.0.1:7506}"
export LAYA_MODE="${LAYA_MODE:-auto}"
export LAYA_WARMUP="${LAYA_WARMUP:-1}"
export PYTHONPATH="$ROOT/brain"

cleanup() {
  if [[ -n "${BRAIN_PID:-}" ]]; then kill "$BRAIN_PID" 2>/dev/null || true; fi
  if [[ -n "${GATEWAY_PID:-}" ]]; then kill "$GATEWAY_PID" 2>/dev/null || true; fi
}
trap cleanup EXIT INT TERM

"$PY" -m laya_brain.server >"$ROOT/.tools/brain.log" 2>&1 &
BRAIN_PID=$!
(cd "$ROOT/gateway" && go build -o "$ROOT/.tools/laya-gateway" .)
"$ROOT/.tools/laya-gateway" >"$ROOT/.tools/gateway.log" 2>&1 &
GATEWAY_PID=$!

for _ in 1 2 3 4 5 6 7 8 9 10; do
  GW_PORT="${LISTEN##*:}"
  if curl -sf "http://127.0.0.1:${BRAIN_PORT}/health" >/dev/null && curl -sf "http://127.0.0.1:${GW_PORT}/health" >/dev/null; then
    break
  fi
  sleep 0.3
done

echo "Brain   http://0.0.0.0:${BRAIN_PORT}"
echo "Gateway http://${LISTEN}"
echo "Chat    http://0.0.0.0:7504"
echo "Practice page http://127.0.0.1:7505/demo"
cd "$ROOT/chat"
exec npm run dev
