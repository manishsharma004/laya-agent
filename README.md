# Laya agent

A page-control agent in four independent projects. The Chrome side panel talks to the agent about the active tab. The command desk lists every open tab, chooses which of those the extension may control, and sends commands to one of them. The Python brain asks [Laya](https://github.com/NandhaKishorM/laya) which action fits, and the extension runs it.

Sibling GitHub repositories could not be created with the available token, so the four projects live here, each with its own manifest and toolchain:

| Directory | Stack | Role |
| --- | --- | --- |
| `extension/` | Chrome Manifest V3 | Sidebar for the active tab, plus page control |
| `brain/` | Python | Turn the prompt plus the snapshot into one Laya decision |
| `gateway/` | Go | API the desk, the sidebar, and the extension share |
| `chat/` | Svelte | Command desk: pick tabs and send commands |

```text
command desk :7504  --->  gateway :7505  --->  brain :7506
sidebar (active tab)  ---^         |
                                   v
                    extension, only on tabs marked available
```

## Run

```bash
./scripts/dev.sh
```

That listens on `0.0.0.0`. These ports sit in the 7500–7510 range; 7500–7503 were already taken on this machine:

- Chat at `http://<host>:7504`
- Gateway at `http://<host>:7505` (practice page at `/demo`)
- Brain at `http://<host>:7506`

Load `extension/` as an unpacked extension. The toolbar icon opens the sidebar, which talks to the agent about the active tab. The command desk at port 7504 lists the tabs the extension can see; uncheck a tab and the extension stops acting on it. Match the sidebar session with the desk (`local` by default). If the browser is on another machine, set the sidebar API field to `http://<this-host>:7505`.

The first start installs uv (if it is missing), Python 3.12, ROCm PyTorch (`2.14.0+rocm10.1.0`), and `laya` into `brain/.venv`, then downloads a checkpoint when `LAYA_WARMUP=1`. Until that finishes, decisions use the heuristic and `backend` is `heuristic`. After the checkpoint is ready, `backend` is `laya`. Pass `ROCM_TORCH_EXTRA=device-gfx1100` (or another `device-gfx*` extra) when creating the venv if that GPU should receive kernels.

`API_TOKEN` on the gateway requires the same value in the command desk Session panel and the sidebar connection form (`X-Api-Token`).

## Tests

```bash
./scripts/test.sh
./scripts/smoke.sh   # servers already running
```

The wire format is in `docs/protocol.md`. How the four programs cooperate, including the decision passes, is in `specs/architecture/`.
