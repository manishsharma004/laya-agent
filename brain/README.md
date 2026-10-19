# Laya brain

Python service that decides the next page action. It builds typed Laya questions (`intent`, `operation`, `target`, `safe_to_run`, `fit`) from the user prompt and the extension's element list, then calls `laya.Router.predict` in one forward pass.

Laya does not write the chat reply. This service maps the typed answers onto one closed action: `click`, `type`, `select`, `scroll`, `navigate`, or `extract`. Text that gets typed, options that get chosen, and addresses that get opened are taken from the user's own prompt.

Until the checkpoint is imported and warmed up, the same question shape is answered by a small heuristic so the gateway can keep working. `backend` in the response says which one ran.

```bash
uv python install 3.12
uv venv .venv --python 3.12
uv pip install --python .venv/bin/python --index-url https://stable.repo.amd.com/rocm/whl-next/ "torch==2.14.0+rocm10.1.0"
uv pip install --python .venv/bin/python "transformers>=4.48" "safetensors>=0.4" "huggingface_hub>=0.20" "numpy>=1.20"
uv pip install --python .venv/bin/python -r requirements-laya.txt --no-deps
LAYA_WARMUP=1 BRAIN_HOST=0.0.0.0 BRAIN_PORT=7506 PYTHONPATH=. .venv/bin/python -m laya_brain.server
```

That torch build is `2.14.0+rocm10.1.0` (HIP is reported on `torch.version.hip`). It runs the Laya checkpoint on CPU when no AMD GPU is visible. To add GPU kernels for one architecture, set the extra before installing, for example `torch[device-gfx1100]==2.14.0+rocm10.1.0`. `device-all` pulls every architecture and needs tens of gigabytes. The GPU also needs `/dev/kfd` and `/dev/dri` mounted into the process. `--no-deps` on `laya` keeps PyPI from replacing the ROCm wheel.

`LAYA_MODE=off` skips the checkpoint. `LAYA_MODE=on` loads it before listening. The default `auto` loads it on a background thread.

`GET /health` and `POST /decide` are the only routes. See `../docs/protocol.md`.
