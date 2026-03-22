"""Conversation compact handler mixin."""

from __future__ import annotations

import json
from typing import TYPE_CHECKING

import structlog

from codeforge.config import get_settings
from codeforge.consumer._subjects import SUBJECT_CONVERSATION_COMPACT_COMPLETE

if TYPE_CHECKING:
    import nats.aio.msg

logger = structlog.get_logger()


class CompactHandlerMixin:
    """Handles conversation.compact.request — summarizes conversation history via LLM."""

    async def _handle_conversation_compact(self, msg: nats.aio.msg.Msg) -> None:
        """Compact a conversation's message history using LLM summarization."""
        try:
            payload = json.loads(msg.data)
            conversation_id = payload.get("conversation_id", "")
            tenant_id = payload.get("tenant_id", "")
            log = logger.bind(conversation_id=conversation_id, tenant_id=tenant_id)
            log.info("received conversation compact request")

            if not conversation_id:
                log.error("missing conversation_id in compact request")
                await msg.ack()
                return

            # Fetch conversation messages from the Go Core API
            messages = await self._fetch_conversation_messages(conversation_id, tenant_id, log)
            if not messages:
                log.info("no messages to compact")
                await msg.ack()
                return

            # Build summarization prompt
            summary = await self._summarize_messages(messages, log)

            # Publish compact complete with the summary
            result = {
                "conversation_id": conversation_id,
                "tenant_id": tenant_id,
                "summary": summary,
                "original_count": len(messages),
                "status": "completed",
            }
            if self._js is not None:
                await self._js.publish(
                    SUBJECT_CONVERSATION_COMPACT_COMPLETE,
                    json.dumps(result).encode(),
                )
            log.info("conversation compact complete", original_count=len(messages))
        except Exception as exc:
            logger.error("conversation compact failed", error=str(exc))
        finally:
            await msg.ack()

    async def _fetch_conversation_messages(
        self,
        conversation_id: str,
        tenant_id: str,
        log: structlog.stdlib.BoundLogger,
    ) -> list[dict[str, str]]:
        """Fetch messages from Go Core API."""
        import httpx

        core_url = get_settings().core_url
        url = f"{core_url}/api/v1/conversations/{conversation_id}/messages"
        headers: dict[str, str] = {}
        if tenant_id:
            headers["X-Tenant-ID"] = tenant_id
        try:
            async with httpx.AsyncClient() as client:
                resp = await client.get(url, headers=headers, timeout=10.0)
                if resp.status_code != 200:
                    log.warning("failed to fetch messages", status=resp.status_code)
                    return []
                data = resp.json()
                if isinstance(data, list):
                    return data
                return []
        except Exception as exc:
            log.error("error fetching messages", error=str(exc))
            return []

    async def _summarize_messages(
        self,
        messages: list[dict[str, str]],
        log: structlog.stdlib.BoundLogger,
    ) -> str:
        """Use LLM to create a concise summary of the conversation."""
        # Build the conversation text for summarization
        conversation_text: list[str] = []
        for msg in messages:
            role = msg.get("role", "unknown")
            content = msg.get("content", "")
            conversation_text.append(f"{role}: {content}")

        full_text = "\n".join(conversation_text)

        # Truncate if extremely long (avoid token limits)
        max_chars = 50000
        if len(full_text) > max_chars:
            full_text = full_text[:max_chars] + "\n... (truncated)"

        prompt = (
            "Summarize this conversation concisely. Preserve key decisions, "
            "code changes, errors encountered, and current state. Keep technical "
            "details but remove redundant back-and-forth.\n\n"
            f"{full_text}"
        )

        try:
            result = await self._llm.chat_completion(
                messages=[
                    {"role": "system", "content": "You are a conversation summarizer. Be concise but thorough."},
                    {"role": "user", "content": prompt},
                ],
            )
            summary = result.content
            log.info("summarization complete", summary_length=len(summary))
            return summary
        except Exception as exc:
            log.error("LLM summarization failed", error=str(exc))
            return f"[Compact failed: {exc}]"
