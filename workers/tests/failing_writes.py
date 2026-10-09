"""os.write that writes short or fails, for files below one directory only (KI-96 review).

A full or failing volume makes a write fail after the file was created, or
write fewer bytes than asked. Writes to any other file go through unchanged.
"""

from __future__ import annotations

import errno
import os
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from collections.abc import Buffer
    from pathlib import Path

    import pytest


class FailingWrites:
    """os.write for files below *below*: at most *max_bytes* per call; with *fail_after*, every call
    after that many raises OSError(*error*)."""

    def __init__(
        self, below: Path, *, max_bytes: int | None = None, fail_after: int | None = None, error: int = errno.ENOSPC
    ) -> None:
        self.below = f"{below}/"
        self.max_bytes = max_bytes
        self.fail_after = fail_after
        self.error = error
        self.calls = 0
        self._write = os.write

    def install(self, monkeypatch: pytest.MonkeyPatch) -> FailingWrites:
        monkeypatch.setattr(os, "write", self)
        return self

    def __call__(self, fd: int, data: Buffer) -> int:
        try:
            target = os.readlink(f"/proc/self/fd/{fd}")
        except OSError:
            target = ""
        if not target.startswith(self.below):
            return self._write(fd, data)
        self.calls += 1
        if self.fail_after is not None and self.calls > self.fail_after:
            raise OSError(self.error, os.strerror(self.error))
        view = memoryview(data)
        return self._write(fd, view if self.max_bytes is None else view[: self.max_bytes])
