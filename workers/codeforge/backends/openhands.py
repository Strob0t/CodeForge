"""OpenHands backend executor — HTTP API wrapper."""

from __future__ import annotations

import asyncio
import contextlib
import json
import logging
import os
from typing import Any

import httpx

from codeforge.backends._base import BackendInfo, ConfigField, OutputCallback, TaskResult
from codeforge.config import resolve_backend_path
from codeforge.constants import DEFAULT_BACKEND_TIMEOUT_SECONDS

logger = logging.getLogger(__name__)

_DEFAULT_TIMEOUT = DEFAULT_BACKEND_TIMEOUT_SECONDS


def _env_float(key: str, default: float) -> float:
    """Read a float from env var, falling back to default."""
    raw = os.environ.get(key, "")
    if raw:
        try:
            return float(raw)
        except ValueError:
            pass
    return default


_POLL_INTERVAL = _env_float("CODEFORGE_OPENHANDS_POLL_INTERVAL", 2.0)
_HTTP_TIMEOUT = _env_float("CODEFORGE_OPENHANDS_HTTP_TIMEOUT", 30.0)
_HEALTH_TIMEOUT = _env_float("CODEFORGE_OPENHANDS_HEALTH_TIMEOUT", 5.0)
_CANCEL_TIMEOUT = _env_float("CODEFORGE_OPENHANDS_CANCEL_TIMEOUT", 5.0)

# Terminal states returned by the OpenHands API.
_SUCCESS_STATES = frozenset({"completed", "finished", "done"})
_FAILURE_STATES = frozenset({"failed", "error", "cancelled"})


class OpenHandsExecutor:
    """Execute tasks using the OpenHands HTTP API."""

    def __init__(self, url: str | None = None) -> None:
        self._url = resolve_backend_path(url, "CODEFORGE_OPENHANDS_URL", "http://localhost:3000")
        self._active_tasks: dict[str, str] = {}  # task_id -> conversation_id

    @property
    def info(self) -> BackendInfo:
        return BackendInfo(
            name="openhands",
            display_name="OpenHands",
            cli_command=self._url,
            requires_docker=True,
            capabilities=["code-edit", "browser", "sandbox"],
            config_schema=(
                ConfigField(key="model", type=str, description="LLM model name"),
                ConfigField(key="timeout", type=int, default=_DEFAULT_TIMEOUT, description="Timeout in seconds"),
                ConfigField(key="api_key", type=str, description="OpenHands API key"),
                ConfigField(key="workspace", type=str, description="Workspace path override"),
            ),
        )

    async def check_available(self) -> bool:
        """Check if the OpenHands HTTP API is reachable."""
        try:
            async with httpx.AsyncClient(timeout=_HEALTH_TIMEOUT) as client:
                resp = await client.get(f"{self._url}/api/health")
                return resp.status_code == 200
        except (httpx.ConnectError, httpx.TimeoutException, httpx.HTTPError):
            return False

    async def execute(
        self,
        task_id: str,
        prompt: str,
        workspace_path: str,
        config: dict[str, Any] | None = None,
        on_output: OutputCallback | None = None,
    ) -> TaskResult:
        """Submit a task to OpenHands and poll for completion."""
        config = config or {}
        timeout = config.get("timeout", _DEFAULT_TIMEOUT)

        base = self._url.rstrip("/")
        headers = self._build_headers(config)
        payload = self._build_payload(prompt, workspace_path, config)

        logger.info("openhands exec task=%s url=%s", task_id, base)

        async with httpx.AsyncClient(timeout=_HTTP_TIMEOUT, headers=headers) as client:
            conversation_id, err = await self._start_conversation(client, base, payload)
            if err:
                return err

            self._active_tasks[task_id] = conversation_id
            try:
                return await self._poll_until_done(client, base, conversation_id, task_id, timeout, on_output)
            finally:
                self._active_tasks.pop(task_id, None)

    async def cancel(self, task_id: str) -> None:
        conversation_id = self._active_tasks.get(task_id)
        if conversation_id is None:
            return
        try:
            base = self._url.rstrip("/")
            async with httpx.AsyncClient(timeout=_CANCEL_TIMEOUT) as client:
                await client.delete(f"{base}/api/conversations/{conversation_id}")
        except (httpx.HTTPStatusError, httpx.ConnectError, httpx.TimeoutException) as exc:
            logger.warning("openhands cancel failed task=%s: %s", task_id, exc)
        finally:
            self._active_tasks.pop(task_id, None)
            logger.info("openhands task cancelled task=%s", task_id)

    # -- private helpers --

    @staticmethod
    def _build_headers(config: dict[str, Any]) -> dict[str, str]:
        headers: dict[str, str] = {}
        api_key = config.get("api_key")
        if api_key:
            headers["Authorization"] = f"Bearer {api_key}"
        return headers

    @staticmethod
    def _build_payload(prompt: str, workspace_path: str, config: dict[str, Any]) -> dict[str, Any]:
        payload: dict[str, Any] = {"task": prompt}
        if workspace_path:
            payload["workspace"] = workspace_path
        model = config.get("model")
        if model:
            payload["model"] = model
        return payload

    @staticmethod
    async def _start_conversation(
        client: httpx.AsyncClient, base: str, payload: dict[str, Any]
    ) -> tuple[str, TaskResult | None]:
        """Start a conversation and return (conversation_id, None) or ("", TaskResult)."""
        try:
            resp = await client.post(f"{base}/api/conversations", json=payload)
            resp.raise_for_status()
            data = resp.json()
            conversation_id = data.get("conversation_id", data.get("id", ""))
        except httpx.HTTPStatusError as exc:
            return "", TaskResult(status="failed", error=f"OpenHands HTTP {exc.response.status_code}: {exc}")
        except (httpx.ConnectError, httpx.TimeoutException) as exc:
            return "", TaskResult(status="failed", error=f"OpenHands unreachable: {exc}")
        except (json.JSONDecodeError, KeyError) as exc:
            return "", TaskResult(status="failed", error=f"OpenHands malformed response: {exc}")
        if not conversation_id:
            return "", TaskResult(status="failed", error="OpenHands returned no conversation ID")
        return conversation_id, None

    async def _poll_until_done(
        self,
        client: httpx.AsyncClient,
        base: str,
        conversation_id: str,
        task_id: str,
        timeout: int,
        on_output: OutputCallback | None,
    ) -> TaskResult:
        output_lines: list[str] = []
        elapsed = 0.0

        while elapsed < timeout:
            await asyncio.sleep(_POLL_INTERVAL)
            elapsed += _POLL_INTERVAL

            status_data = await self._poll_once(client, base, conversation_id, task_id)
            if status_data is None:
                continue

            await self._stream_messages(status_data, output_lines, on_output)

            state = status_data.get("status", status_data.get("state", ""))
            if state in _SUCCESS_STATES:
                return TaskResult(status="completed", output="\n".join(output_lines))
            if state in _FAILURE_STATES:
                error = status_data.get("error", f"OpenHands task {state}")
                return TaskResult(status="failed", output="\n".join(output_lines), error=error)

        # Timeout — attempt to cancel the remote task.
        with contextlib.suppress(Exception):
            await client.delete(f"{base}/api/conversations/{conversation_id}")
        return TaskResult(
            status="failed",
            output="\n".join(output_lines),
            error=f"OpenHands timed out after {timeout}s",
        )

    @staticmethod
    async def _poll_once(
        client: httpx.AsyncClient, base: str, conversation_id: str, task_id: str
    ) -> dict[str, Any] | None:
        try:
            resp = await client.get(f"{base}/api/conversations/{conversation_id}")
            resp.raise_for_status()
            return resp.json()
        except (httpx.HTTPStatusError, httpx.ConnectError, httpx.TimeoutException) as exc:
            logger.warning("openhands poll error task=%s: %s", task_id, exc)
            return None

    @staticmethod
    async def _stream_messages(
        status_data: dict[str, Any],
        output_lines: list[str],
        on_output: OutputCallback | None,
    ) -> None:
        messages = status_data.get("messages", [])
        for msg in messages[len(output_lines) :]:
            text = msg.get("content", str(msg))
            output_lines.append(text)
            if on_output is not None:
                await on_output(text)
