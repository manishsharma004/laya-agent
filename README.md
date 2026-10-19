# Laya agent

A page-control agent in four independent projects. The chat takes a prompt, the Python brain asks [Laya](https://github.com/NandhaKishorM/laya) which action fits, the Go gateway queues that action, and the Chrome extension runs it on the active tab.

Sibling GitHub repositories could not be created with the available token, so the four projects live here, each with its own manifest, toolchain, and workflow:

| Directory | Stack | Role |
| --- | --- | --- |
| `extension/` | Chrome Manifest V3 | Snapshot the page and run a closed set of actions |
| `brain/` | Python | Turn the prompt plus the snapshot into one Laya decision |
| `gateway/` | Go | API the chat and the extension both call |
| `chat/` | Svelte | Talk to the agent |

```text
chat :7500  --->  gateway :7501  --->  brain :7502
                     ^   |
                     |   v
                  extension on the active tab
```

## Run

```bash
./scripts/dev.sh
```

That listens on `0.0.0.0`:

- Chat at `http://<host>:7500`
- Gateway at `http://<host>:7501` (practice page at `/demo`)
- Brain at `http://<host>:7502`

Load `extension/` as an unpacked extension. Match the popup session id with the chat session (`local` by default). If the browser is on another machine, point the popup API field at `http://<this-host>:7501`.

The first start installs CPU PyTorch and `laya` into `brain/.venv` and downloads a checkpoint when `LAYA_WARMUP=1`. Until that finishes, decisions use the heuristic and `backend` is `heuristic`. After the checkpoint is ready, `backend` is `laya`.

`API_TOKEN` on the gateway requires the same value in the chat Session panel and the extension popup (`X-Api-Token`).

## Tests

```bash
./scripts/test.sh
./scripts/smoke.sh   # servers already running
```

The wire format is in `docs/protocol.md`.
