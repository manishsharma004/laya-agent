# Architecture

Laya Agent is one repository with four programs. The Chrome extension operates the page. The command desk is where a person picks tabs and types commands. The Go gateway keeps the session. The Python brain asks [Laya](https://github.com/NandhaKishorM/laya) which action fits, then turns that typed answer into one command.

Laya is a decision model. It picks among choices, scores, and yes/no questions. It does not write the chat reply. The reply is a template filled from the choice.

| Document | What it explains |
| --- | --- |
| [System](system.md) | Processes, ports, trust boundaries, and how the pieces connect |
| [Sessions and tabs](sessions-and-tabs.md) | Inventory, availability, transcripts, and the websocket |
| [Decision](decision.md) | The Laya passes, grouping, projection, and confirmation |
| [Flows](flows.md) | End-to-end sequences for a message, a page change, and a command |

The JSON shapes shared by those programs are in [`docs/protocol.md`](../../docs/protocol.md).
