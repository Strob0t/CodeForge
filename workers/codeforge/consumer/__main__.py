"""Allow ``python -m codeforge.consumer`` to start the NATS consumer."""

import asyncio

from codeforge.consumer import main

asyncio.run(main())
