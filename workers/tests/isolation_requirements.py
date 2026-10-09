"""What the tool-isolation tests need from the host, and what happens without it (KI-96 O10).

The tests that start real tool processes need root (to start processes as
other users), setpriv, POSIX ACLs on /tmp and Landlock; the Docker suite
needs docker. A development machine without them skips those tests. The CI
steps that must run them set ``CODEFORGE_ISOLATION_TESTS=required``, and a
missing requirement then fails the test instead of skipping it. (``CI`` is
not the switch: GitHub Actions sets it for every step, including the
unprivileged test run, where these tests skip by design.)
"""

from __future__ import annotations

import contextlib
import errno
import os
import shutil
import tempfile

import pytest

REQUIRED = os.environ.get("CODEFORGE_ISOLATION_TESTS", "").strip().lower() == "required"


def require(available: bool, reason: str) -> None:
    """Skip the test when *available* is false; fail it with CODEFORGE_ISOLATION_TESTS=required."""
    if available:
        return
    if REQUIRED:
        pytest.fail(f"CODEFORGE_ISOLATION_TESTS=required, but {reason}")
    pytest.skip(reason)


def acls_on(directory: str) -> bool:
    """Whether a default POSIX ACL can be set below *directory*."""
    from codeforge import posix_acl

    probe = tempfile.mkdtemp(prefix=".cf-acl-probe-", dir=directory)
    try:
        posix_acl.set_acl(probe, posix_acl.DEFAULT, posix_acl.tenant_default(20000))
    except OSError as exc:
        if exc.errno in (errno.EOPNOTSUPP, errno.ENOTSUP):
            return False
        raise
    finally:
        with contextlib.suppress(OSError):
            os.rmdir(probe)
    return True


def root_isolation_problem() -> str:
    """Why real tool processes cannot be started here ("" when they can)."""
    from codeforge.tool_exec import landlock_abi

    if not hasattr(os, "geteuid") or os.geteuid() != 0:
        return "the tests need root to start processes as the tool users"
    if shutil.which("setpriv") is None:
        return "the tests need setpriv (util-linux)"
    if not acls_on("/tmp"):
        return "the tests need POSIX ACLs on /tmp"
    if landlock_abi() < 2:
        return "the tests need Landlock ABI 2 or later (kernel 5.19+, landlock in the lsm= list)"
    return ""
