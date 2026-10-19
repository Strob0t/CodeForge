"""At-most-once work a worker has accepted and still owes a completion for (ADR-016).

A run, task or benchmark acked on accept is never redelivered. If the worker
has to stop while such work is running, it publishes a failed completion for
it, so the Go Core does not wait for its own timeout (or forever).
"""

from __future__ import annotations

import asyncio
import contextlib
import dataclasses
from typing import TYPE_CHECKING

import structlog

if TYPE_CHECKING:
    from collections.abc import Awaitable, Callable, Coroutine, Iterable, Iterator

logger = structlog.get_logger()

# How long cancelled handlers may take to run their cleanup, and how long
# publishing the failed completions may take, before the worker exits anyway.
CANCEL_WAIT_SECONDS = 10.0
REPORT_TIMEOUT_SECONDS = 15.0


@dataclasses.dataclass(eq=False)
class AcceptedWork:
    """One accepted piece of work; *completed* is set once its completion was published."""

    label: str
    report_failure: Callable[[str], Awaitable[None]]
    completed: bool = False


class InFlightWork:
    """The accepted work and the background tasks of one worker."""

    def __init__(self) -> None:
        self._accepted: set[AcceptedWork] = set()
        self._background: set[asyncio.Task[None]] = set()

    def accept(self, label: str, report_failure: Callable[[str], Awaitable[None]]) -> AcceptedWork:
        """Register accepted work; *report_failure(reason)* publishes its failed completion."""
        work = AcceptedWork(label, report_failure)
        self._accepted.add(work)
        return work

    def release(self, work: AcceptedWork) -> None:
        """Forget *work*: it ended and published its completion (or failed trying)."""
        self._accepted.discard(work)

    @contextlib.contextmanager
    def track(self, label: str, report_failure: Callable[[str], Awaitable[None]]) -> Iterator[AcceptedWork]:
        """Register accepted work for the duration of the block."""
        work = self.accept(label, report_failure)
        try:
            yield work
        finally:
            self.release(work)

    def start_background(self, coro: Coroutine[object, object, None], name: str) -> asyncio.Task[None]:
        """Run *coro* as a task that is cancelled when the worker aborts."""
        task = asyncio.create_task(coro, name=name)
        self._background.add(task)
        task.add_done_callback(self._background.discard)
        return task

    async def abort(self, tasks: Iterable[asyncio.Task[None]], grace: float, reason: str) -> None:
        """Stop the worker's work: wait up to *grace* seconds for accepted work to finish,
        then cancel *tasks* and the background tasks and fail the work that is still unfinished.
        """
        loop = asyncio.get_running_loop()
        deadline = loop.time() + grace
        while self._accepted and loop.time() < deadline:
            await asyncio.sleep(0.01)

        unfinished = list(self._accepted)
        running = [task for task in (*tasks, *self._background) if not task.done()]
        for task in running:
            task.cancel()
        if running:
            _, still_running = await asyncio.wait(running, timeout=CANCEL_WAIT_SECONDS)
            if still_running:
                logger.error("tasks did not stop after cancellation", count=len(still_running))

        unreported = [work for work in unfinished if not work.completed]
        if not unreported:
            return
        logger.warning("failing accepted work that did not finish", work=[work.label for work in unreported])
        try:
            await asyncio.wait_for(
                asyncio.gather(*(self._report(work, reason) for work in unreported)),
                timeout=REPORT_TIMEOUT_SECONDS,
            )
        except TimeoutError:
            logger.error("reporting failed work timed out", work=[work.label for work in unreported])

    @staticmethod
    async def _report(work: AcceptedWork, reason: str) -> None:
        try:
            await work.report_failure(reason)
        except Exception as exc:
            logger.exception("failed to report unfinished work", work=work.label, error=str(exc))
        else:
            work.completed = True
