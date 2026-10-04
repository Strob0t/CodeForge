#!/usr/bin/env python3
"""Small CodeForge API client for live end-to-end sessions (standard library only).

Usage (from any directory; the environment comes from env.example.sh or the shell):

    api.py login                              log in again, print the user
    api.py projects                           list projects
    api.py project-create NAME (--repo URL | --local PATH) [--config JSON]
    api.py goals PROJECT                      list a project's goals
    api.py goal-add PROJECT TITLE CONTENT_FILE [--kind requirement]
    api.py conv-create PROJECT [TITLE]        create a conversation
    api.py send CONV TEXT [--mode MODE] [--model MODEL]
    api.py messages CONV [--last N]           print the conversation (role, tools, text)
    api.py bypass CONV                        bypass approvals for the conversation (admin)
    api.py stop CONV                          stop the conversation's run
    api.py run-start PROJECT TITLE PROMPT [--mode MODE] [--policy PROFILE]
    api.py run RUN                            show a run
    api.py approvals [--last N]               approvals requested (from the Core log)
    api.py approve RUN CALL [allow|deny]      decide a pending tool call
    api.py call METHOD PATH [JSON]            any other endpoint

The access token is cached in $LIVE_DIR/run/api-token (mode 0600). A first login
with must_change_password sets LIVE_ADMIN_NEW_PASS, or keeps the password (as
frontend/e2e/global-setup.ts does).
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import tempfile
import urllib.error
import urllib.request
from pathlib import Path
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from collections.abc import Callable

JSONValue = None | bool | int | float | str | list["JSONValue"] | dict[str, "JSONValue"]

CORE_URL = os.environ.get("CODEFORGE_CORE_URL", "http://127.0.0.1:8080")
API = CORE_URL.rstrip("/") + "/api/v1"
LIVE_DIR = Path(os.environ.get("LIVE_DIR") or Path(tempfile.gettempdir()) / "codeforge-live")
TOKEN_FILE = LIVE_DIR / "run" / "api-token"
EMAIL = os.environ.get("CODEFORGE_AUTH_ADMIN_EMAIL", "admin@localhost")
PASSWORD = os.environ.get("LIVE_ADMIN_PASS") or os.environ.get("CODEFORGE_AUTH_ADMIN_PASS", "Changeme123")
TIMEOUT_S = 600


class APIError(Exception):
    def __init__(self, status: int, method: str, path: str, body: JSONValue) -> None:
        super().__init__(f"{method} {path}: HTTP {status}: {json.dumps(body)[:2000]}")
        self.status = status


def _out(text: str, *, err: bool = False) -> None:
    (sys.stderr if err else sys.stdout).write(text + "\n")


def _request(method: str, path: str, body: JSONValue = None, token: str | None = None) -> tuple[int, JSONValue]:
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(API + path, data=data, method=method)  # noqa: S310 - local dev API
    req.add_header("Content-Type", "application/json")
    if token:
        req.add_header("Authorization", f"Bearer {token}")
    try:
        with urllib.request.urlopen(req, timeout=TIMEOUT_S) as resp:  # noqa: S310 - local dev API
            raw = resp.read()
            return resp.status, json.loads(raw) if raw else None
    except urllib.error.HTTPError as exc:
        raw = exc.read()
        try:
            return exc.code, json.loads(raw)
        except ValueError:
            return exc.code, raw.decode(errors="replace")


def _password_login(password: str) -> dict[str, JSONValue]:
    status, body = _request("POST", "/auth/login", {"email": EMAIL, "password": password})
    if status != 200 or not isinstance(body, dict):
        raise APIError(status, "POST", "/auth/login", body)
    return body


def _login() -> str:
    body = _password_login(PASSWORD)
    token = str(body["access_token"])
    user = body.get("user")
    if isinstance(user, dict) and user.get("must_change_password"):
        new_password = os.environ.get("LIVE_ADMIN_NEW_PASS", PASSWORD)
        change: JSONValue = {"old_password": PASSWORD, "new_password": new_password}
        status, resp = _request("POST", "/auth/change-password", change, token)
        if status >= 400:
            raise APIError(status, "POST", "/auth/change-password", resp)
        _out("password changed (must_change_password)", err=True)
        token = str(_password_login(new_password)["access_token"])
    TOKEN_FILE.parent.mkdir(parents=True, exist_ok=True)
    fd = os.open(TOKEN_FILE, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, "w") as fh:
        fh.write(token)
    return token


def _token() -> str:
    if TOKEN_FILE.is_file():
        return TOKEN_FILE.read_text().strip()
    return _login()


def call(method: str, path: str, body: JSONValue = None) -> JSONValue:
    """Call the API with the cached token; logs in again once on 401."""
    status, data = _request(method, path, body, _token())
    if status == 401:
        status, data = _request(method, path, body, _login())
    if status >= 400:
        raise APIError(status, method, path, data)
    return data


def _dump(value: JSONValue) -> None:
    _out(json.dumps(value, indent=2, ensure_ascii=False))


def _as_list(value: JSONValue) -> list[JSONValue]:
    if isinstance(value, list):
        return value
    if isinstance(value, dict):
        for key in ("items", "data", "messages"):
            inner = value.get(key)
            if isinstance(inner, list):
                return inner
    return []


def _tool_names(calls: JSONValue) -> str:
    if not isinstance(calls, list) or not calls:
        return ""
    names = []
    for c in calls:
        fn = c.get("function") if isinstance(c, dict) else None
        names.append(str(fn.get("name", "?")) if isinstance(fn, dict) else "?")
    return " calls=" + ",".join(names)


def _cmd_login(_: argparse.Namespace) -> None:
    TOKEN_FILE.unlink(missing_ok=True)
    _dump(call("GET", "/auth/me"))


def _cmd_projects(_: argparse.Namespace) -> None:
    for proj in _as_list(call("GET", "/projects")):
        if isinstance(proj, dict):
            _out(f"{proj.get('id')} {proj.get('name')} {proj.get('workspace_path') or proj.get('repo_url')}")


def _cmd_project_create(args: argparse.Namespace) -> None:
    body: dict[str, JSONValue] = {"name": args.name, "config": json.loads(args.config)}
    if args.repo:
        body["repo_url"] = args.repo
    else:
        body["local_path"] = args.local
    _dump(call("POST", "/projects", body))


def _cmd_goal_add(args: argparse.Namespace) -> None:
    content = Path(args.content_file).read_text()
    _dump(call("POST", f"/projects/{args.project}/goals", {"kind": args.kind, "title": args.title, "content": content}))


def _cmd_send(args: argparse.Namespace) -> None:
    msg: dict[str, JSONValue] = {"content": args.text}
    if args.mode:
        msg["mode"] = args.mode
    if args.model:
        msg["model"] = args.model
    _dump(call("POST", f"/conversations/{args.conv}/messages", msg))


def _cmd_messages(args: argparse.Namespace) -> None:
    for msg in _as_list(call("GET", f"/conversations/{args.conv}/messages"))[-args.last :]:
        if not isinstance(msg, dict):
            continue
        tool = f" {msg['tool_name']}" if msg.get("tool_name") else ""
        _out(f"--- {msg.get('role', '?')}{tool}{_tool_names(msg.get('tool_calls'))} ({msg.get('created_at', '')})")
        _out(str(msg.get("content") or "")[:4000])


def _cmd_run_start(args: argparse.Namespace) -> None:
    """Starts an agent run: an agent with the session model, a task, then the run."""
    model = os.environ.get("LIVE_MODEL", "")
    agents = _as_list(call("GET", f"/projects/{args.project}/agents"))
    agent = next((a for a in agents if isinstance(a, dict) and a.get("name") == "live-e2e"), None)
    if agent is None:
        config: JSONValue = {"model": model} if model else {}
        agent = call(
            "POST", f"/projects/{args.project}/agents", {"name": "live-e2e", "backend": "aider", "config": config}
        )
    task = call("POST", f"/projects/{args.project}/tasks", {"title": args.title, "prompt": args.prompt})
    if not isinstance(agent, dict) or not isinstance(task, dict):
        raise SystemExit("unexpected agent or task response")
    run: dict[str, JSONValue] = {
        "task_id": task["id"],
        "agent_id": agent["id"],
        "project_id": args.project,
        "mode_id": args.mode,
    }
    if args.policy:
        run["policy_profile"] = args.policy
    _dump(call("POST", "/runs", run))


def _cmd_approvals(args: argparse.Namespace) -> None:
    log = LIVE_DIR / "logs" / "core.log"
    if not log.is_file():
        raise SystemExit(f"no Core log at {log}")
    found: list[dict[str, JSONValue]] = []
    for line in log.read_text(errors="replace").splitlines():
        if "HITL approval" not in line:
            continue
        try:
            entry = json.loads(line)
        except ValueError:
            continue
        if isinstance(entry, dict):
            found.append(entry)
    fields = ("time", "msg", "run_id", "call_id", "tool", "timeout")
    for entry in found[-args.last :]:
        _out(" ".join(f"{k}={entry.get(k)}" for k in fields if k in entry))


def _cmd_call(args: argparse.Namespace) -> None:
    _dump(call(args.method.upper(), args.path, json.loads(args.body) if args.body else None))


COMMANDS: dict[str, Callable[[argparse.Namespace], None]] = {
    "login": _cmd_login,
    "projects": _cmd_projects,
    "project-create": _cmd_project_create,
    "goals": lambda a: _dump(call("GET", f"/projects/{a.project}/goals")),
    "goal-add": _cmd_goal_add,
    "conv-create": lambda a: _dump(call("POST", f"/projects/{a.project}/conversations", {"title": a.title})),
    "send": _cmd_send,
    "messages": _cmd_messages,
    "bypass": lambda a: _dump(call("POST", f"/conversations/{a.conv}/bypass-approvals")),
    "stop": lambda a: _dump(call("POST", f"/conversations/{a.conv}/stop")),
    "run-start": _cmd_run_start,
    "run": lambda a: _dump(call("GET", f"/runs/{a.run}")),
    "approvals": _cmd_approvals,
    "approve": lambda a: _dump(call("POST", f"/runs/{a.run}/approve/{a.call_id}", {"decision": a.decision})),
    "call": _cmd_call,
}


def _parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="CodeForge API client for live end-to-end sessions")
    sub = parser.add_subparsers(dest="cmd", required=True)
    sub.add_parser("login")
    sub.add_parser("projects")
    p = sub.add_parser("project-create")
    p.add_argument("name")
    src = p.add_mutually_exclusive_group(required=True)
    src.add_argument("--repo")
    src.add_argument("--local")
    p.add_argument("--config", default="{}")
    sub.add_parser("goals").add_argument("project")
    p = sub.add_parser("goal-add")
    for name in ("project", "title", "content_file"):
        p.add_argument(name)
    p.add_argument("--kind", default="requirement")
    p = sub.add_parser("conv-create")
    p.add_argument("project")
    p.add_argument("title", nargs="?", default="live e2e")
    p = sub.add_parser("send")
    p.add_argument("conv")
    p.add_argument("text")
    p.add_argument("--mode")
    p.add_argument("--model")
    p = sub.add_parser("messages")
    p.add_argument("conv")
    p.add_argument("--last", type=int, default=20)
    sub.add_parser("bypass").add_argument("conv")
    sub.add_parser("stop").add_argument("conv")
    p = sub.add_parser("run-start")
    for name in ("project", "title", "prompt"):
        p.add_argument(name)
    p.add_argument("--mode", default="coder")
    p.add_argument("--policy")
    sub.add_parser("run").add_argument("run")
    sub.add_parser("approvals").add_argument("--last", type=int, default=10)
    p = sub.add_parser("approve")
    p.add_argument("run")
    p.add_argument("call_id")
    p.add_argument("decision", nargs="?", default="allow", choices=("allow", "deny"))
    p = sub.add_parser("call")
    p.add_argument("method")
    p.add_argument("path")
    p.add_argument("body", nargs="?")
    return parser


def main(argv: list[str]) -> int:
    args = _parser().parse_args(argv)
    try:
        COMMANDS[args.cmd](args)
    except APIError as exc:
        _out(str(exc), err=True)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
