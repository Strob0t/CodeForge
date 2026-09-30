"""Cancels of tasks that may still wait in NATS for a free worker (KI-65).

A tasks.cancel reaches the listener of a running task only. A cancel for a
task whose message still waits in the stream (every worker busy) used to be
lost, and the task ran later anyway. Every worker records the task cancels it
sees with their stream sequence: a task message published before the cancel
is not started, one published after it (the task dispatched again) runs.
The registry is bounded in size and time.
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
    from collections.abc import Callable

    from nats.js.client import JetStreamContext

logger = structlog.get_logger()

# A task waits in NATS at most this long for a free worker in practice; older
# cancels are forgotten.
CANCEL_TTL_SECONDS = 24 * 3600.0
MAX_CANCELS = 10_000


class CancelRegistry:
    """The latest cancel per task ID with its stream sequence, oldest record first."""

    def __init__(
        self,
        max_entries: int = MAX_CANCELS,
        ttl: float = CANCEL_TTL_SECONDS,
        clock: Callable[[], float] = time.monotonic,
    ) -> None:
        self._max_entries = max_entries
        self._ttl = ttl
        self._clock = clock
        # task ID -> (stream sequence of its latest cancel, time it was recorded)
        self._cancels: OrderedDict[str, tuple[int, float]] = OrderedDict()

    def __len__(self) -> int:
        self._forget_expired()
        return len(self._cancels)

    def record(self, task_id: str, seq: int) -> None:
        """Record a cancel of *task_id* published at stream sequence *seq*."""
        if not task_id:
            return
        previous = self._cancels.pop(task_id, None)
        if previous is not None:
            seq = max(seq, previous[0])
        self._cancels[task_id] = (seq, self._clock())
        while len(self._cancels) > self._max_entries:
            self._cancels.popitem(last=False)
        self._forget_expired()

    def cancelled(self, task_id: str, seq: int) -> bool:
        """Whether a cancel of *task_id* was published after its message at stream sequence *seq*."""
        self._forget_expired()
        entry = self._cancels.get(task_id)
        return entry is not None and seq < entry[0]

    def _forget_expired(self) -> None:
        cutoff = self._clock() - self._ttl
        while self._cancels:
            _, (_, recorded_at) = next(iter(self._cancels.items()))
            if recorded_at >= cutoff:
                return
            self._cancels.popitem(last=False)


async def record_cancels(sub: JetStreamContext.PushSubscription, registry: CancelRegistry) -> None:
    """Record every task cancel on *sub* until the subscription closes or the listener is cancelled.

    Malformed messages and messages without a stream position (which cannot
    be ordered against a task message) are skipped.
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
        if ids is not None and ids[1] and seq is not None:
            registry.record(ids[1], seq)
        # Let other work run even if cancels arrive back to back.
        await asyncio.sleep(0)
