# Laya brain

Python service that decides the next page action. It builds typed Laya questions (`intent`, `operation`, `target`, `safe_to_run`, `fit`) from the user prompt and the extension's element list, then calls `laya.Router.predict` in one forward pass.

Laya does not write the chat reply. This service maps the typed answers onto one closed action: `click`, `type`, `select`, `scroll`, `navigate`, or `extract`. Text that gets typed, options that get chosen, and addresses that get opened are taken from the user's own prompt.

Until the checkpoint is imported and warmed up, the same question shape is answered by a small heuristic so the gateway can keep working. `backend` in the response says which one ran.

```bash
python3 -m venv .venv
.venv/bin/pip install torch --index-url https://download.pytorch.org/whl/cpu
.venv/bin/pip install "transformers>=4.48" "safetensors>=0.4" "huggingface_hub>=0.20" "numpy>=1.20"
.venv/bin/pip install -r requirements-laya.txt --no-deps
LAYA_WARMUP=1 BRAIN_HOST=0.0.0.0 BRAIN_PORT=7502 PYTHONPATH=. .venv/bin/python -m laya_brain.server
```

`LAYA_MODE=off` skips the checkpoint. `LAYA_MODE=on` loads it before listening. The default `auto` loads it on a background thread.

`GET /health` and `POST /decide` are the only routes. See `../docs/protocol.md`.
