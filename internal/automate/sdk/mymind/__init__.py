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
    for t in mymind.maps.tasks(status="open"):    # the maps' tasks
        print(t["map"], t["title"])
    mymind.agent("实现登录页", repo="~/code/app")  # the coding tool this automation names

The model is the automation's own: `model` in automation.toml, `# 模型:` in
a file's header, or llm(..., model=...). It may be an API model such as
"deepseek-chat" or "claude-sonnet-4-5" (the keys are MyMind's settings),
or a command: "cli:claude -p" gets the prompt on its input and answers on
its output.

The coding tool of mymind.agent is the automation's own too: `agent` in
automation.toml (`# 编码工具:` in a header), or agent(..., agent=...): a
command such as "claude -p", "codex exec" or "aider --yes --message {prompt}",
run in the repository, given the task in place of {prompt} or on its input.

mymind.settings are the automation's settings, its own keys included.
"""

import json as _json
import os as _os
import socket as _socket
import sys as _sys

__all__ = ["llm", "agent", "notify", "confirm", "output", "clip", "search", "maps", "state", "settings", "log", "name", "here"]

name = _os.environ.get("MYMIND_NAME", "")
"""The automation's name."""

here = _os.getcwd()
"""The automation's folder, where it runs."""

settings = _json.loads(_os.environ.get("MYMIND_SETTINGS") or "{}") or {}
"""The automation's settings: automation.toml's keys, or its header's."""

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


def agent(task, repo=None, agent=None):
    """Has the coding tool this automation names do a task in a repository
    (the automation's folder when none), and returns what it answered.
    What it prints shows in the run's log as it goes."""
    return _call("agent", task=str(task), repo=_os.path.expanduser(str(repo or "")), agent=agent or "")


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


class _Maps:
    """The mind maps of MyMind. A map is named by its name or path in the
    folder of maps ("项目/官网"); nodes by their IDs."""

    def list(self):
        """The maps: [{"name", "path", "tasks", "open"}]."""
        return _call("maps.list")

    def read(self, map):
        """A map, whole: {"name", "path", "project", "sheets": [{"title",
        "root": node, "floating": [node]}]}; a node is {"id", "title",
        "note", "tags", "links", "status", "priority", "start", "due",
        "done_at", "children"}."""
        return _call("maps.read", map=str(map))

    def tasks(self, map=None, status=None, done_since=None):
        """Tasks of a map, or of all: status "idea", "todo", "doing", "done"
        or "open" (not done); done_since a date or datetime. Each is a node
        with "map", "sheet" and "where" (the titles above it)."""
        if done_since is not None and not isinstance(done_since, str):
            done_since = done_since.isoformat()
        return _call("maps.tasks", map=str(map or ""), status=str(status or ""), done_since=done_since or "")

    def add(self, map, parent_id, outline):
        """Adds nodes under a node: outline is Markdown's list ("- 标题",
        indented below)."""
        return _call("maps.edit", tool="add_nodes", args={"map": map, "parent_id": parent_id, "outline": outline})

    def update(self, map, id, **fields):
        """Changes a node: title, note, append_note, status (idea, todo,
        doing, done, none), priority (none, low, medium, high), start, due
        (YYYY-MM-DD), estimate, add_tags, remove_tags, add_links,
        remove_links."""
        return _call("maps.edit", tool="update_node", args=dict(fields, map=map, id=id))

    def move(self, map, id, parent_id, index=-1):
        return _call("maps.edit", tool="move_node", args={"map": map, "id": id, "parent_id": parent_id, "index": index})

    def delete(self, map, id):
        return _call("maps.edit", tool="delete_node", args={"map": map, "id": id})

    def create(self, name, root_title="", outline="", folder=""):
        """Makes a map, its center topic and the nodes of an outline."""
        return _call("maps.edit", tool="create_map", args={"name": name, "root_title": root_title or name,
                                                          "outline": outline, "folder": folder})


maps = _Maps()


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
