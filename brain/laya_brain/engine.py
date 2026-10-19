"""Ask Laya which page action fits a prompt, then turn that into one command."""

from __future__ import annotations

import os
import re
import threading
from typing import Any, Mapping

ACTIONS = ("click", "type", "select", "scroll", "navigate", "extract", "done")
DESTRUCTIVE = (
    "delete",
    "remove",
    "purchase",
    "pay",
    "buy",
    "submit",
    "send",
    "order",
    "unsubscribe",
    "transfer",
    "drop",
)
STOPWORDS = {
    "a",
    "an",
    "the",
    "to",
    "into",
    "in",
    "on",
    "of",
    "for",
    "and",
    "please",
    "click",
    "press",
    "tap",
    "open",
    "hit",
    "type",
    "enter",
    "fill",
    "write",
    "input",
    "select",
    "choose",
    "pick",
    "scroll",
    "go",
    "navigate",
    "page",
    "button",
    "link",
    "field",
    "box",
}


def build_state(prompt: str, page: Mapping[str, Any] | None, history: list[Mapping[str, Any]] | None) -> str:
    lines = [f"User request: {prompt.strip()}"]
    for item in (history or [])[-4:]:
        role = str(item.get("role") or "user")
        content = str(item.get("content") or "").replace("\n", " ")[:240]
        if content:
            lines.append(f"- {role}: {content}")
    if not page:
        lines.append("Page: unavailable. The browser extension has not reported a tab.")
        return "\n".join(lines)
    lines.append(f"Page title: {page.get('title') or ''}")
    lines.append(f"Page URL: {page.get('url') or ''}")
    elements = list(page.get("elements") or [])
    lines.append(f"Actionable elements ({len(elements)}):")
    for element in elements[:24]:
        bits = [
            str(element.get("id") or ""),
            str(element.get("role") or element.get("tag") or "element"),
            str(element.get("name") or "(unnamed)"),
        ]
        if element.get("editable"):
            bits.append("editable")
        if element.get("value"):
            bits.append("has-value")
        lines.append("- " + " | ".join(bits))
    if len(elements) > 24:
        lines.append(f"- {len(elements) - 24} more elements were omitted")
    return "\n".join(lines)[:4000]


def build_questions(page: Mapping[str, Any] | None) -> dict[str, Any]:
    elements = list((page or {}).get("elements") or [])[:24]
    target: dict[str, str] = {}
    for element in elements:
        element_id = str(element.get("id") or "").strip()
        if not element_id or not re.fullmatch(r"[A-Za-z0-9_-]+", element_id):
            continue
        label = f"{element.get('role') or element.get('tag') or 'element'} {element.get('name') or ''}".strip()
        target[element_id] = (label or element_id)[:180]
    target["none"] = "no element, or the operation applies to the whole page"
    return {
        "intent": {
            "type": "choice",
            "instructions": "What should the assistant do with the user request?",
            "criteria": {
                "control_page": "change or read something on the current page",
                "answer": "talk about the page or the assistant without changing the page",
                "clarify": "the request is too vague to choose an action",
            },
        },
        "operation": {
            "type": "choice",
            "instructions": "Which single page operation best fulfills the request? Use done when the page should stay as it is.",
            "criteria": {
                "click": "activate a button, link, or other control",
                "type": "enter text into an editable field",
                "select": "choose an option in a dropdown",
                "scroll": "scroll the page or bring an element into view",
                "navigate": "open a different http or https address",
                "extract": "read text from an element without changing the page",
                "done": "do not change the page",
            },
        },
        "target": {
            "type": "choice",
            "instructions": "Which element should receive the operation?",
            "criteria": target,
        },
        "safe_to_run": {
            "type": "noul",
            "instructions": "Is the requested action safe to perform immediately, without paying, deleting, sending a message, or submitting a form?",
        },
        "fit": {
            "type": "score",
            "instructions": "How precisely can the reported page fulfill the request?",
            "criteria": ["impossible", "poor", "partial", "good", "exact"],
        },
    }


def _tokens(text: str) -> set[str]:
    return {token for token in re.findall(r"[a-z0-9]+", text.lower()) if token not in STOPWORDS and len(token) > 1}


def _best_element(prompt: str, elements: list[Mapping[str, Any]]) -> str | None:
    wanted = _tokens(prompt)
    prompt_lower = prompt.lower()
    best_id = None
    best_score = 0
    for element in elements:
        name = str(element.get("name") or "")
        haystack = f"{name} {element.get('role') or ''} {element.get('tag') or ''}"
        score = len(wanted & _tokens(haystack)) * 2
        lowered = name.lower().strip()
        if lowered and lowered in prompt_lower:
            score += 3
        if score > best_score:
            best_score = score
            best_id = str(element.get("id"))
    return best_id


def _quoted(prompt: str) -> str:
    match = re.search(r"[\"“'](.+?)[\"”']", prompt)
    return match.group(1).strip() if match else ""


def extract_typed_text(prompt: str) -> str:
    quoted = _quoted(prompt)
    if quoted:
        return quoted
    match = re.search(
        r"\b(?:type|enter|fill|write|input)\s+(.+?)\s+(?:into|in|on)\b",
        prompt,
        flags=re.IGNORECASE,
    )
    return match.group(1).strip() if match else ""


def extract_option(prompt: str) -> str:
    match = re.search(
        r"\b(?:select|choose|pick)\s+[\"']?(.+?)[\"']?\s+(?:from|in|on)\b",
        prompt,
        flags=re.IGNORECASE,
    )
    if match:
        return match.group(1).strip()
    return _quoted(prompt)


def extract_url(prompt: str) -> str:
    match = re.search(r"https?://[^\s\"']+", prompt)
    return match.group(0).rstrip(".,)") if match else ""


def looks_destructive(prompt: str, element: Mapping[str, Any] | None) -> bool:
    blob = prompt.lower()
    if element:
        blob += " " + str(element.get("name") or "").lower()
    return any(re.search(rf"\b{re.escape(word)}\b", blob) for word in DESTRUCTIVE)


def heuristic_predict(state: str, questions: Mapping[str, Any]) -> dict[str, Any]:
    """Same answer shape as Laya, used until a checkpoint is ready."""
    prompt = ""
    for line in state.splitlines():
        if line.startswith("User request:"):
            prompt = line.split(":", 1)[1].strip()
            break
    lowered = prompt.lower()
    criteria = ((questions.get("target") or {}).get("criteria") or {})
    known_ids = [key for key in criteria if key != "none"]
    # The state lists elements, but the heuristic only needs ids that were offered.
    elements = []
    for line in state.splitlines():
        if not line.startswith("- ") or "|" not in line:
            continue
        parts = [part.strip() for part in line[2:].split("|")]
        if not parts or parts[0] not in known_ids:
            continue
        elements.append(
            {
                "id": parts[0],
                "role": parts[1] if len(parts) > 1 else "",
                "name": parts[2] if len(parts) > 2 else "",
                "editable": "editable" in parts,
            }
        )

    if re.search(r"\b(click|press|tap|hit)\b", lowered):
        operation = "click"
    elif re.search(r"\b(type|enter|fill|write|input)\b", lowered):
        operation = "type"
    elif re.search(r"\b(select|choose|pick)\b", lowered):
        operation = "select"
    elif "scroll" in lowered:
        operation = "scroll"
    elif extract_url(prompt):
        operation = "navigate"
    elif re.search(r"\b(read|extract|copy)\b", lowered):
        operation = "extract"
    elif re.search(r"\b(what|who|why|how|status|help)\b", lowered) or "?" in prompt:
        operation = "done"
    else:
        operation = "done"

    if operation == "done" and (re.search(r"\b(what|who|why|how|status|help)\b", lowered) or "?" in prompt):
        intent = "answer"
    elif operation == "done":
        intent = "clarify"
    else:
        intent = "control_page"

    pool = elements
    if operation == "type":
        editable = [element for element in elements if element.get("editable")]
        pool = editable or elements
    target = "none"
    if operation in {"click", "type", "select", "scroll", "extract"}:
        target = _best_element(prompt, pool) or "none"
    safe = 0.15 if looks_destructive(prompt, next((element for element in elements if element["id"] == target), None)) else 0.92
    fit = 3.4 if target != "none" or operation in {"navigate", "scroll", "done"} else 1.1
    return {
        "answers": {
            "intent": {"choice": intent},
            "operation": {"choice": operation},
            "target": {"choice": target},
            "safe_to_run": {"noul": safe},
            "fit": {"score": fit},
        },
        "routing": {"model": "heuristic"},
    }


def _choice(answers: Mapping[str, Any], key: str, default: str) -> str:
    value = (answers.get(key) or {}).get("choice")
    return str(value) if isinstance(value, str) and value else default


def _noul(answers: Mapping[str, Any], key: str) -> float | None:
    value = (answers.get(key) or {}).get("noul")
    return float(value) if isinstance(value, (int, float)) else None


def _score(answers: Mapping[str, Any], key: str) -> float | None:
    value = (answers.get(key) or {}).get("score")
    return float(value) if isinstance(value, (int, float)) else None


def _confidence(score: float | None) -> float:
    if score is None:
        return 0.5
    return max(0.0, min(1.0, score / 4.0))


def project_decision(
    prompt: str,
    page: Mapping[str, Any] | None,
    answers: Mapping[str, Any],
    *,
    confirmed: bool,
    backend: str,
) -> dict[str, Any]:
    elements = {str(element.get("id")): element for element in ((page or {}).get("elements") or [])}
    intent = _choice(answers, "intent", "clarify")
    operation = _choice(answers, "operation", "done")
    if operation not in ACTIONS:
        operation = "done"
    target = _choice(answers, "target", "none")
    element = elements.get(target)
    safe = _noul(answers, "safe_to_run")
    confidence = _confidence(_score(answers, "fit"))
    action = None
    reply = ""

    if not page and operation not in {"navigate", "done"}:
        reply = "The extension has not reported a page for this session yet. Open a tab and check the popup session id."
    elif operation == "done":
        reply = _describe_page(page) if intent == "answer" else (
            "Tell me the control to use, for example “click Login” or “type “hello” into Search”."
        )
    elif operation == "navigate":
        url = extract_url(prompt)
        if url:
            action = {"type": "navigate", "elementId": "", "text": "", "url": url}
        else:
            reply = "Include the http(s) address you want opened."
    elif operation == "scroll" and element is None:
        action = {"type": "scroll", "elementId": "", "text": "", "url": ""}
    elif element is None:
        reply = "I don't see a matching control in the latest page snapshot."
    elif operation == "type":
        text = extract_typed_text(prompt)
        if not text:
            reply = "Put the text to enter in quotes, for example type “hello” into Search."
        elif not element.get("editable"):
            reply = f"“{_name(element)}” is not an editable field."
        else:
            action = {"type": "type", "elementId": target, "text": text, "url": ""}
    elif operation == "select":
        option = extract_option(prompt)
        if not option:
            reply = "Tell me which option to choose, for example choose Red in Color."
        else:
            action = {"type": "select", "elementId": target, "text": option, "url": ""}
    elif operation in {"click", "scroll", "extract"}:
        action = {"type": operation, "elementId": target, "text": "", "url": ""}
    else:
        reply = _describe_page(page)

    destructive = looks_destructive(prompt, element)
    unsafe = safe is not None and safe < 0.5
    needs_confirmation = bool(action) and (destructive or unsafe) and operation in {"click", "type", "select", "navigate"}
    if action and not reply:
        reply = _describe_action(action, element)
    if needs_confirmation and not confirmed:
        reply = f"{reply} This looks hard to undo. Confirm and I will run it."
    elif needs_confirmation and confirmed:
        reply = f"Confirmed. {reply}"
        needs_confirmation = False

    return {
        "reply": reply,
        "backend": backend,
        "needsConfirmation": needs_confirmation,
        "action": action,
        "decision": {
            "intent": intent,
            "operation": operation,
            "target": target if element is not None else "none",
            "confidence": round(confidence, 4),
            "safeToRun": safe,
            "answers": answers,
        },
    }


def _name(element: Mapping[str, Any] | None) -> str:
    if not element:
        return "the page"
    return str(element.get("name") or element.get("id") or "element")


def _describe_action(action: Mapping[str, Any], element: Mapping[str, Any] | None) -> str:
    name = _name(element)
    kind = action.get("type")
    if kind == "click":
        return f"Click “{name}”."
    if kind == "type":
        return f"Type “{action.get('text', '')}” into “{name}”."
    if kind == "select":
        return f"Choose “{action.get('text', '')}” in “{name}”."
    if kind == "scroll":
        return f"Scroll “{name}” into view." if element else "Scroll the page."
    if kind == "navigate":
        return f"Open {action.get('url')}."
    if kind == "extract":
        return f"Read “{name}”."
    return "No page change."


def _describe_page(page: Mapping[str, Any] | None) -> str:
    if not page:
        return "No page is connected. Load the extension and match its session id to this chat."
    title = page.get("title") or "Untitled"
    url = page.get("url") or "an unknown address"
    count = len(page.get("elements") or [])
    return (
        f"This tab is “{title}” at {url}, with {count} reported controls. "
        "Ask me to click, type, choose, scroll, or open an address."
    )


class Engine:
    def __init__(self, mode: str | None = None) -> None:
        self.mode = (mode or os.environ.get("LAYA_MODE") or "auto").strip().lower()
        if self.mode not in {"auto", "on", "off"}:
            self.mode = "auto"
        self._router: Any = None
        self._status = "disabled" if self.mode == "off" else "loading"
        self._error: str | None = None
        self._lock = threading.Lock()
        if self.mode == "on":
            self._load()
        elif self.mode == "auto":
            threading.Thread(target=self._load, name="laya-load", daemon=True).start()

    def _load(self) -> None:
        try:
            runner = build_runner()
            if os.environ.get("LAYA_WARMUP", "0") == "1":
                ask_runner(
                    runner,
                    "warmup",
                    {"ok": {"type": "noul", "instructions": "Is this a warmup ping?"}},
                )
            with self._lock:
                self._router = runner
                self._status = "ready"
                self._error = None
        except Exception as exc:  # noqa: BLE001 - surfaced through /health
            with self._lock:
                self._router = None
                self._status = "unavailable"
                self._error = f"{type(exc).__name__}: {exc}"

    def health(self) -> dict[str, Any]:
        with self._lock:
            return {"ok": True, "laya": self._status, "mode": self.mode, "error": self._error}

    def decide(self, payload: Mapping[str, Any]) -> dict[str, Any]:
        prompt = str(payload.get("prompt") or "").strip()
        page = payload.get("page") if isinstance(payload.get("page"), Mapping) else None
        history = payload.get("history") if isinstance(payload.get("history"), list) else []
        confirmed = bool(payload.get("confirmed"))
        state = build_state(prompt, page, history)
        questions = build_questions(page)
        backend = "heuristic"
        with self._lock:
            runner = self._router
            status = self._status
            error = self._error
        raw: dict[str, Any]
        if runner is not None:
            try:
                raw = ask_runner(runner, state, questions)
                backend = "laya"
            except Exception as exc:  # noqa: BLE001 - fall back keeps the API usable
                error = f"{type(exc).__name__}: {exc}"
                status = "error"
                raw = heuristic_predict(state, questions)
        else:
            raw = heuristic_predict(state, questions)
        answers = raw.get("answers") if isinstance(raw, Mapping) else None
        if not isinstance(answers, Mapping):
            answers = heuristic_predict(state, questions)["answers"]
            backend = "heuristic"
        projected = project_decision(prompt, page, answers, confirmed=confirmed, backend=backend)
        projected["layaStatus"] = status
        if error and backend != "laya":
            projected["layaError"] = error
        if isinstance(raw.get("routing"), Mapping):
            projected["decision"]["routing"] = raw["routing"]
        return projected


def build_runner() -> Any:
    import laya

    if hasattr(laya, "Router"):
        return laya.Router()
    if hasattr(laya, "load"):
        return laya.load("convaiinnovations/laya")
    raise RuntimeError("installed laya package has neither Router nor load")


def ask_runner(runner: Any, state: str, questions: Mapping[str, Any]) -> dict[str, Any]:
    if hasattr(runner, "predict"):
        result = runner.predict(state, questions)
    elif hasattr(runner, "system_one"):
        result = runner.system_one({"text": state}, questions)
    else:
        raise RuntimeError("laya runner has neither predict nor system_one")
    if not isinstance(result, Mapping):
        raise RuntimeError("laya runner returned a non-object result")
    return dict(result)
