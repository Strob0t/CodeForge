"""Cancels of work that may still wait in NATS for a free worker (KI-65).

A cancel (tasks.cancel, runs.cancel, conversation.run.cancel) reaches the
listener of work that already runs only. A cancel for work whose start
message still waits in the stream (every worker busy) used to be lost, and
the work ran later anyway. Every worker records the cancels it sees with
their stream sequence: a start published before a cancel of its work is not
executed, one published after it (a task dispatched again, a conversation's
next turn) runs. The registry is bounded in size and time.
"""

from __future__ import annotations

import asyncio
import time
from collections import OrderedDict
from typing import TYPE_CHECKING

import structlog

from codeforge.consumer._delivery import stream_sequence
from codeforge.runtime import cancel_ids

if TYPE_CHECKING:
    from collections.abc import Callable, Iterable

    from nats.js.client import JetStreamContext

logger = structlog.get_logger()

# A task waits in NATS at most this long for a free worker in practice; older
# cancels are forgotten.
CANCEL_TTL_SECONDS = 24 * 3600.0
MAX_CANCELS = 10_000


def task_key(task_id: str) -> str:
    """Registry key of the cancels of a backend task (tasks.cancel)."""
    return f"task:{task_id}" if task_id else ""


def run_key(run_id: str) -> str:
    """Registry key of the cancels of a run (runs.cancel)."""
    return f"run:{run_id}" if run_id else ""


def conversation_key(conversation_id: str) -> str:
    """Registry key of the cancels of a conversation's run (conversation.run.cancel)."""
    return f"conversation:{conversation_id}" if conversation_id else ""


class CancelRegistry:
    """The latest cancel per key (task_key, run_key, conversation_key) with its stream sequence, oldest first."""

    def __init__(
        self,
        max_entries: int = MAX_CANCELS,
        ttl: float = CANCEL_TTL_SECONDS,
        clock: Callable[[], float] = time.monotonic,
    ) -> None:
        self._max_entries = max_entries
        self._ttl = ttl
        self._clock = clock
        # key -> (stream sequence of its latest cancel, time it was recorded)
        self._cancels: OrderedDict[str, tuple[int, float]] = OrderedDict()

    def __len__(self) -> int:
        self._forget_expired()
        return len(self._cancels)

    def record(self, key: str, seq: int) -> None:
        """Record a cancel of the work named *key* published at stream sequence *seq*."""
        if not key:
            return
        previous = self._cancels.pop(key, None)
        if previous is not None:
            seq = max(seq, previous[0])
        self._cancels[key] = (seq, self._clock())
        while len(self._cancels) > self._max_entries:
            self._cancels.popitem(last=False)
        self._forget_expired()

    def cancelled(self, key: str, seq: int) -> bool:
        """Whether a cancel of the work named *key* was published after its start at stream sequence *seq*."""
        self._forget_expired()
        entry = self._cancels.get(key)
        return entry is not None and seq < entry[0]

    def cancelled_any(self, keys: Iterable[str], seq: int) -> bool:
        """Whether any of *keys* was cancelled after stream sequence *seq*."""
        return any(self.cancelled(key, seq) for key in keys if key)

    def _forget_expired(self) -> None:
        cutoff = self._clock() - self._ttl
        while self._cancels:
            _, (_, recorded_at) = next(iter(self._cancels.items()))
            if recorded_at >= cutoff:
                return
            self._cancels.popitem(last=False)


async def record_cancels(
    sub: JetStreamContext.PushSubscription,
    registry: CancelRegistry,
    key_of: Callable[[str, str], str] = lambda _run_id, task_id: task_key(task_id),
) -> None:
    """Record every cancel on *sub* until the subscription closes or the listener is cancelled.

    *key_of(run_id, task_id)* names the cancelled work from the message's IDs.
    Malformed messages and messages without a stream position (which cannot
    be ordered against a start message) are skipped.
    """
    while True:
        try:
            msg = await sub.next_msg(timeout=1.0)
        except TimeoutError:
            continue
        except Exception as exc:
            logger.debug("task cancel registry stopped", error=str(exc))
            return
        ids = cancel_ids(msg.data)
        seq = stream_sequence(msg)
        if ids is not None and seq is not None:
            registry.record(key_of(*ids), seq)
        # Let other work run even if cancels arrive back to back.
        await asyncio.sleep(0)
