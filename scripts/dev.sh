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

UV="${UV:-}"
if [[ -z "$UV" ]]; then
  if command -v uv >/dev/null 2>&1; then
    UV="$(command -v uv)"
  elif [[ -x "$ROOT/.tools/uv" ]]; then
    UV="$ROOT/.tools/uv"
  else
    echo "Installing uv into .tools"
    curl -LsSf https://astral.sh/uv/install.sh | env UV_INSTALL_DIR="$ROOT/.tools" sh
    UV="$ROOT/.tools/uv"
  fi
fi

PY="$ROOT/brain/.venv/bin/python"
if [[ ! -x "$PY" ]]; then
  "$UV" python install 3.12
  "$UV" venv "$ROOT/brain/.venv" --python 3.12
fi
# ROCm wheels live on AMD's index. A device extra (device-gfx1100, device-all, ...)
# is optional: the +rocm build already runs Laya on CPU, and this machine may have no GPU.
ROCM_INDEX="${ROCM_INDEX:-https://stable.repo.amd.com/rocm/whl-next/}"
ROCM_TORCH_SPEC="${ROCM_TORCH_SPEC:-torch==2.14.0+rocm10.1.0}"
if [[ -n "${ROCM_TORCH_EXTRA:-}" ]]; then
  ROCM_TORCH_SPEC="torch[${ROCM_TORCH_EXTRA}]==2.14.0+rocm10.1.0"
fi
if ! "$PY" -c "import torch; raise SystemExit(0 if 'rocm' in torch.__version__ else 1)" >/dev/null 2>&1 \
  || ! "$PY" -c "import laya" >/dev/null 2>&1; then
  echo "Installing ROCm torch and laya into brain/.venv"
  # --no-deps on laya keeps a later PyPI solve from replacing the ROCm wheel.
  if "$UV" pip install --python "$PY" --index-url "$ROCM_INDEX" "$ROCM_TORCH_SPEC" \
    && "$UV" pip install --python "$PY" "transformers>=4.48" "safetensors>=0.4" "huggingface_hub>=0.20" "numpy>=1.20" \
    && "$UV" pip install --python "$PY" -r "$ROOT/brain/requirements-laya.txt" --no-deps; then
    echo "Laya is installed on the ROCm torch build. The first decision downloads a checkpoint."
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
