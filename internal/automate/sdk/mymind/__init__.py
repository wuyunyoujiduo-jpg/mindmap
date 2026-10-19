"""mymind: what an automation asks of MyMind.

    import mymind

    text = mymind.llm("总结成三条要点", input=page)   # the model this automation names
    mymind.notify("网页有变化", text)
    if mymind.confirm("要提交吗？"):
        ...
    mymind.output(text)                           # shown as the run's result
    mymind.clip(text, title="周报")                # kept in the library
    for hit in mymind.search("绩效"):             # the library's search
        print(hit["title"])
    seen = mymind.state.get("seen", [])           # kept between runs
    mymind.state["seen"] = seen

The model is the automation's own: `model` in automation.toml, `# 模型:` in
a file's header, or llm(..., model=...). It may be an API model such as
"deepseek-chat" or "claude-sonnet-4-5" (the keys are MyMind's settings),
or a command: "cli:claude -p" gets the prompt on its input and answers on
its output.
"""

import json as _json
import os as _os
import socket as _socket
import sys as _sys

__all__ = ["llm", "notify", "confirm", "output", "clip", "search", "state", "log", "name", "here"]

name = _os.environ.get("MYMIND_NAME", "")
"""The automation's name."""

here = _os.getcwd()
"""The automation's folder, where it runs."""

_next_id = 0


class MyMindError(Exception):
    """What MyMind answered when it could not do what was asked."""


def _call(method, **params):
    global _next_id
    path = _os.environ.get("MYMIND_SOCKET")
    if not path:
        raise MyMindError("没有在 MyMind 里运行：mymind 只在自动化里可用")
    _next_id += 1
    s = _socket.socket(_socket.AF_UNIX, _socket.SOCK_STREAM)
    try:
        s.connect(path)
        s.sendall((_json.dumps({"id": _next_id, "method": method, "params": params}, ensure_ascii=False) + "\n").encode("utf-8"))
        buf = b""
        while not buf.endswith(b"\n"):
            chunk = s.recv(65536)
            if not chunk:
                break
            buf += chunk
    finally:
        s.close()
    resp = _json.loads(buf.decode("utf-8"))
    if resp.get("error"):
        raise MyMindError(resp["error"])
    return resp.get("result")


def llm(prompt, input=None, model=None, system=None, json=False, max_tokens=8000):
    """Asks a model and returns its answer: text, or with json=True the
    object it answered."""
    if not isinstance(input, (str, type(None))):
        input = _json.dumps(input, ensure_ascii=False, indent=2)
    answer = _call("llm", prompt=prompt, input=input or "", model=model or "", system=system or "",
                   json=bool(json), max_tokens=int(max_tokens))
    if json:
        return _json.loads(answer)
    return answer


def notify(title, body=""):
    """Shows a notification."""
    _call("notify", title=str(title), body=str(body))


def confirm(message, detail=""):
    """Asks in MyMind and waits: True when it was allowed."""
    return bool(_call("confirm", message=str(message), detail=str(detail)))


def output(text, append=False):
    """Gives the run's result, shown with it (Markdown)."""
    _call("output", text=str(text), append=bool(append))


def clip(text, title="", url="", tags=None, note=""):
    """Keeps text in the library; returns the clip's ID."""
    return _call("clip", text=str(text), title=str(title), url=str(url), tags=list(tags or []), note=str(note))


def search(query, limit=5):
    """Searches the library: a list of {"id", "title", "text", "url"}."""
    return _call("search", query=str(query), limit=int(limit))


def log(*args):
    """Prints into the run's log."""
    print(*args, flush=True)


class _State(dict):
    """What the automation keeps between runs: a dict, saved as it changes."""

    def __init__(self):
        super().__init__()
        self._path = _os.environ.get("MYMIND_STATE", "")
        if self._path and _os.path.exists(self._path):
            try:
                with open(self._path, encoding="utf-8") as f:
                    super().update(_json.load(f))
            except (OSError, ValueError):
                pass

    def save(self):
        if not self._path:
            return
        _os.makedirs(_os.path.dirname(self._path), exist_ok=True)
        tmp = self._path + ".tmp"
        with open(tmp, "w", encoding="utf-8") as f:
            _json.dump(dict(self), f, ensure_ascii=False, indent=2)
        _os.replace(tmp, self._path)

    def set(self, key, value):
        self[key] = value

    def __setitem__(self, key, value):
        super().__setitem__(key, value)
        self.save()

    def __delitem__(self, key):
        super().__delitem__(key)
        self.save()

    def update(self, *args, **kwargs):
        super().update(*args, **kwargs)
        self.save()

    def clear(self):
        super().clear()
        self.save()


state = _State()
