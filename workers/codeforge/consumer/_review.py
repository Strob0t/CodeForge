"""Review trigger handler mixin."""

from __future__ import annotations

import json
import uuid
from typing import TYPE_CHECKING

import structlog

from codeforge.consumer._subjects import (
    SUBJECT_CONVERSATION_RUN_START,
    SUBJECT_REVIEW_TRIGGER_COMPLETE,
)
from codeforge.models import (
    ConversationMessagePayload,
    ConversationRunStartMessage,
    ModeConfig,
    ReviewTriggerRequestPayload,
    TerminationConfig,
)

if TYPE_CHECKING:
    import nats.aio.msg

logger = structlog.get_logger()

_BOUNDARY_ANALYZER_SYSTEM_PROMPT = (
    "You are a boundary-analyzer agent. Your task is to analyze the project's codebase "
    "and identify all architectural boundaries: API boundaries, data boundaries, "
    "inter-service boundaries, and cross-language boundaries.\n\n"
    "Instructions:\n"
    "1. Use Read, Glob, Grep, and ListDir tools to explore the project structure.\n"
    "2. Identify files and modules that form layer boundaries.\n"
    "3. Produce a BOUNDARIES.json artifact containing the detected boundaries.\n"
    "4. Each boundary entry should include: path, type, counterpart (if any), "
    "and whether it was auto-detected.\n\n"
    "Output format: A JSON array of boundary objects saved as BOUNDARIES.json.\n"
    "You may only read files. Do not write, edit, or execute commands."
)


class ReviewHandlerMixin:
    """Handles review.trigger.request NATS messages."""

    async def _handle_review_trigger(self, msg: nats.aio.msg.Msg) -> None:
        await self._handle_request(
            msg=msg,
            request_model=ReviewTriggerRequestPayload,
            dedup_key=lambda r: f"review-{r.project_id}-{r.commit_sha}",
            handler=self._do_review_trigger,
            log_context=lambda r: {
                "project_id": r.project_id,
                "commit_sha": r.commit_sha,
                "source": r.source,
            },
        )

    async def _do_review_trigger(self, request: ReviewTriggerRequestPayload, log: structlog.BoundLogger) -> None:
        """Dispatch a boundary-analyzer conversation run via NATS."""
        log.info("review trigger received")

        if self._js is None:
            log.warning("JetStream not available, cannot dispatch boundary-analyzer")
            return

        run_id = str(uuid.uuid4())

        user_content = (
            f"Analyze the architectural boundaries in project {request.project_id} "
            f"at commit {request.commit_sha}. "
            "Identify all API, data, inter-service, and cross-language boundaries. "
            "Produce a BOUNDARIES.json artifact with the results."
        )

        run_msg = ConversationRunStartMessage(
            run_id=run_id,
            conversation_id=run_id,
            project_id=request.project_id,
            tenant_id=request.tenant_id,
            messages=[
                ConversationMessagePayload(role="user", content=user_content),
            ],
            system_prompt=_BOUNDARY_ANALYZER_SYSTEM_PROMPT,
            model="",
            policy_profile="standard",
            agentic=True,
            mode=ModeConfig(
                id="boundary-analyzer",
                tools=["Read", "Glob", "Grep", "ListDir"],
                denied_tools=["Write", "Edit", "Bash"],
                required_artifact="BOUNDARIES.json",
                llm_scenario="plan",
            ),
            termination=TerminationConfig(
                max_steps=30,
                timeout_seconds=300,
                max_cost=2.0,
            ),
        )

        stamped = self._stamp_trust(run_msg.model_dump())

        await self._js.publish(
            SUBJECT_CONVERSATION_RUN_START,
            json.dumps(stamped).encode(),
            headers={"Nats-Msg-Id": f"review-boundary-{uuid.uuid4()}"},
        )

        log.info("boundary-analyzer run dispatched", run_id=run_id)

        # Publish completion event so Go Core knows the trigger was processed.
        complete_payload = {
            "project_id": request.project_id,
            "tenant_id": request.tenant_id,
            "commit_sha": request.commit_sha,
            "status": "dispatched",
            "run_id": run_id,
        }
        await self._js.publish(
            SUBJECT_REVIEW_TRIGGER_COMPLETE,
            json.dumps(complete_payload).encode(),
            headers={"Nats-Msg-Id": f"review-trigger-complete-{uuid.uuid4()}"},
        )

        log.info("review trigger complete event published")
