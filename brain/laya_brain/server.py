"""HTTP API for the decision engine. Binds to BRAIN_HOST:BRAIN_PORT."""

from __future__ import annotations

import json
import os
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Any

from laya_brain.engine import Engine


class BrainHandler(BaseHTTPRequestHandler):
    engine: Engine
    server_version = "LayaBrain/0.1"

    def do_GET(self) -> None:  # noqa: N802
        path = self.path.split("?", 1)[0]
        if path == "/health":
            self._json(200, self.engine.health())
            return
        self._json(404, {"error": "not found"})

    def do_POST(self) -> None:  # noqa: N802
        path = self.path.split("?", 1)[0]
        if path != "/decide":
            self._json(404, {"error": "not found"})
            return
        length = int(self.headers.get("Content-Length") or "0")
        if length > 1_000_000:
            self._json(413, {"error": "body too large"})
            return
        raw = self.rfile.read(length) if length else b"{}"
        try:
            payload: Any = json.loads(raw.decode("utf-8"))
        except json.JSONDecodeError:
            self._json(400, {"error": "invalid json"})
            return
        if not isinstance(payload, dict):
            self._json(400, {"error": "body must be an object"})
            return
        if not str(payload.get("prompt") or "").strip():
            self._json(400, {"error": "prompt is required"})
            return
        try:
            result = self.engine.decide(payload)
        except Exception as exc:  # noqa: BLE001
            self._json(500, {"error": str(exc)})
            return
        action = result.get("action") or {}
        sys.stderr.write(
            f"decide backend={result.get('backend')} op={action.get('type', 'none')} "
            f"target={action.get('elementId', '-')}\n"
        )
        self._json(200, result)

    def log_message(self, fmt: str, *args: Any) -> None:
        sys.stderr.write("%s - %s\n" % (self.address_string(), fmt % args))

    def _json(self, status: int, payload: dict[str, Any]) -> None:
        body = json.dumps(payload).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)


def serve(engine: Engine, host: str, port: int) -> None:
    BrainHandler.engine = engine
    httpd = ThreadingHTTPServer((host, port), BrainHandler)
    sys.stderr.write(f"laya brain listening on http://{host}:{port}\n")
    httpd.serve_forever()


def main() -> None:
    host = os.environ.get("BRAIN_HOST", "0.0.0.0")
    port = int(os.environ.get("BRAIN_PORT", "7506"))
    serve(Engine(), host, port)


if __name__ == "__main__":
    main()
