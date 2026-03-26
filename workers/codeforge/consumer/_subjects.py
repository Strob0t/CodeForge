"""NATS stream and subject constants for the TaskConsumer.

Re-exports from the top-level ``codeforge.nats_subjects`` module so existing
imports (``from codeforge.consumer._subjects import ...``) continue to work.
New code should prefer importing directly from ``codeforge.nats_subjects``.
"""

from codeforge.nats_subjects import *  # noqa: F403 -- intentional re-export
from codeforge.nats_subjects import consumer_name as consumer_name  # explicit re-export for type checkers
