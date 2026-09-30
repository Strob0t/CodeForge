# ADR-016: NATS Delivery, Acknowledgement and Retry Semantics

> **Status:** accepted (implemented in milestone S2 of the [fix plan](../../known-issues-fix-plan.md#s2---reliable-messaging-and-runtime), KI-18, KI-19, KI-20)
> **Date:** 2026-09-30
> **Deciders:** Project owner (accepted the messaging principles of the fix plan: one stream, shared durable pull
> consumers, at-most-once ack-on-accept for long work plus a Go-side watchdog, retries via the JetStream delivery
> count and `{subject}.dlq`)
> **Refines:** [ADR-001](001-nats-jetstream-message-queue.md)

### Context

ADR-001 chose NATS JetStream for "at-least-once delivery, consumer groups" but did not define how consumers are
created, shared and acknowledged. The code on `staging` (2026-09-29) showed what that gap costs
([Known Issues](../../todo.md#known-issues) KI-18, KI-19, KI-20):

- Go durables were created with a 5 minute `InactiveThreshold`, so a Go Core that was down for longer lost them
  (the DLQ monitor as well).
- Every durable was created with deliver policy `all`. Its first creation, and every recreation (after an expiry,
  an external deletion, or the worker's "incompatible consumer" path that deleted and recreated it), replayed the
  subject's history of the 30-day retention window: old runs were executed again.
- Messages were acked only after the handler finished, with an ack wait of 30 s (worker) and 90 s (Go). Agent runs,
  conversation runs (up to one hour), indexing and graph builds were redelivered to a second worker while the first
  was still running them.
- The per-run cancel listeners (ephemeral JetStream consumers) and the conversation heartbeat were never stopped:
  every run leaked two consumers and a task, against the stream's limit of 200 consumers.
- Worker retries were counted from a `Retry-Count` header that nothing sets: failed messages were NAK'd immediately
  and forever, and invalid payloads were NAK'd in a loop. The dedup cache marked a request as processed before it
  ran, so the redelivery of a failed request was acked as a duplicate and the request was lost.
- The Go validator accepted any JSON on `runs.*` and did not know `context.*` and `repomap.*` at all.

### Decision

**1. One stream (unchanged).** The Go Core creates or updates the single stream `CODEFORGE` (limits retention) on
start. The subject catalog is still `internal/port/messagequeue/queue.go` <-> `workers/codeforge/nats_subjects.py`.

**2. One shared durable pull consumer per subject and side.** The Go Core subscribes as `codeforge-go-{subject}`,
the worker as `codeforge-py-{subject}` (dots become dashes, wildcards `all`). All instances of a side bind the same
durable, so each message is processed by one instance. Settings: explicit ack, `AckWait` 90 s, `MaxDeliver` 4
(first delivery plus 3 retries), **no inactivity threshold**. Each side ensures its durables on every start
(idempotent):

- not found: created with deliver policy **`new`** - creating a durable never replays history;
- found: the mutable settings are updated and the delivery position is kept (deliver policy, start sequence and
  start time are carried over, because JetStream rejects changing them: error 10012). This also removes the
  inactivity threshold from durables created by earlier releases;
- worker only: a push consumer left under a durable name by an earlier release is replaced by a pull consumer that
  starts after the old consumer's ack floor (or at new messages if it never acked one).

The Go health monitor checks its durables every 30 s and recreates a deleted one the same way (deliver policy
`new`), replacing the consume context of the subscription. The worker ensures a durable again when a fetch shows it
is gone (no responders, which nats-py raises as `ServiceUnavailableError`; a pending pull on a deleted durable is
reported as a timeout, so the next fetch sees it). A message loop that cannot recover within `consumer_max_errors`
consecutive errors stops the whole worker (health sentinel removed, exit status 1), so the container is restarted
instead of a "healthy" process that no longer consumes the subject. An answered fetch without messages and a
successful re-attach reset the count, so separate error episodes do not add up. Before the worker exits, accepted
at-most-once work gets a grace period of 30 s; what is still running then is cancelled and reported as failed
(section 3). The exit is therefore not delayed by a long run, and the Go Core does not wait for its timeout.

**3. Two acknowledgement modes.**

| Mode | Subjects | Ack | Failure | Crashed instance |
|---|---|---|---|---|
| at-most-once (ack on accept) | worker: `runs.start`, `conversation.run.start`, `tasks.agent.*`, `benchmark.run.request` (runs in the background) | confirmed (double) ack before the handler runs | reported as a failed completion or result (`runs.complete`, `conversation.run.complete`, `tasks.result`, `benchmark.run.result`), never retried | ended by a Go-side watchdog where one exists: run timer, stale-work release, benchmark watchdog; none for conversation runs (section 6) |
| at-least-once | all other subjects, Go and worker | after the handler succeeded | retried, then dead-lettered (section 4) | redelivered after `AckWait` |

Agent runs, conversation runs and backend tasks (Aider, OpenHands, ...) change the workspace and are not idempotent:
a redelivery would execute a partially applied run a second time on another worker. The accept ack is confirmed by
the server (`ack_sync`): a plain ack is fire-and-forget, and a lost one would hand the running work to a second
worker after `AckWait`. An unanswered double ack is repeated (3 attempts of 5 s): it is idempotent, the server
confirms an ack it already applied. If none is confirmed, the work is not started, its dedup key is released and
the message is NAK'd: a message whose ack never arrived goes to the next worker at once, and the server ignores the
NAK of a message whose ack did arrive (that work is then ended by a Go-side watchdog, section 6). An exception
after the accept is reported as a failed completion, unless the work already published its outcome: a conversation
run is failed only while its completion is unpublished, a run only while `RuntimeClient.completed` is false.

The completion or result of accepted work is the only way the Go Core learns its outcome, so it is published with
up to 3 attempts (backoff 0.5 s, then 1 s; `workers/codeforge/nats_publish.py`), all under one `Nats-Msg-Id`: if an
attempt reached the stream but its PubAck was lost, the stream's dedup window (2 min) drops the retry. This matters
for `conversation.run.complete`, which the Go Core deduplicates by `Nats-Msg-Id` only. A result that could still
not be published is logged. The worker registers every accepted piece of work with a callback that publishes its
failed completion (`workers/codeforge/consumer/_in_flight.py`); when the worker has to stop (section 2) it waits up
to 30 s for the registry to empty, then cancels the message loops and the background benchmark runs and publishes
failed completions ("worker stopped before the work finished: ...") for the work that did not finish. A completion
published at the moment of the cancellation can still be followed by the failed one; the window is a few
milliseconds.

Everything else is a request whose repetition is harmless (index, search, repo map, quality gate, memory, review
trigger, ...). While an at-least-once handler runs, its consumer sends an in-progress ack (`+WPI`) every
`AckWait / 3` (Go: a timer in `handleMessage` that is only armed after `AckWait / 3`, so fast handlers send nothing;
worker: in the message loop around every handler, stopping as soon as the message is settled), so a handler that is
slower than `AckWait` is not redelivered to another instance, while a crashed instance stops the heartbeat and its
message is redelivered. The heartbeat is capped: a hung handler stops reporting progress and its message is
redelivered instead of holding a `MaxAckPending` slot forever. Go: the longest legitimate handler is the tool-call
request, which waits up to the HITL approval timeout (`runtime.approval_timeout_seconds`, no upper bound), so the
cap is `max(10 min, approval timeout + 5 min)` (`Queue.SetMaxHandlerDuration`, called at startup); a fixed cap below
the approval timeout would redeliver the request and handle it twice. Worker: 30 min, above the longest indexing or
LLM request. CPU-bound worker work (repo map, retrieval chunking, BM25 indexing and embedding decoding of the whole
corpus, graph extraction) runs in a thread so it does not starve the event loop that sends the heartbeats.

A request/response handler that answers with an **error result** (context rerank, retrieval, sub-agent and graph
search) has settled the request: the Go waiter got its answer, so the message is acked rather than retried.

**4. Retries and dead letters (both sides).**

- The attempt number is JetStream's delivery count (`NumDelivered` / `metadata.num_delivered`, 1-based). The
  `Retry-Count` header is gone. If the Go side cannot read the delivery count, a failure is dead-lettered at once
  (the remaining retries are unknown).
- A failure before attempt 4 is NAK'd with a 2 s delay.
- A failure on attempt 4 is published to `{subject}.dlq` and then acked.
- An **invalid payload** (Go: validator; worker: schema validation error, a payload that is not a JSON object, or a
  missing required field) is published to `{subject}.dlq` at once and **terminated** (`Term`): it is never NAK'd
  and never redelivered.
- The dead-letter copy keeps the headers except the JetStream publish-control headers (`Nats-*`): with the original
  `Nats-Msg-Id` the stream would discard the copy as a duplicate of the original. The original ID is kept in
  `X-Original-Msg-Id`.
- If the dead-letter publish fails, or the stream reports it as a duplicate (`PubAck.Duplicate`, nothing stored),
  the message is NAK'd with the delay instead of being settled, so it is never acknowledged without a dead-letter
  copy. After the last attempt JetStream then keeps it unacknowledged (visible as pending in the consumer info and
  in the max-deliveries advisory) instead of redelivering it.
- The Go DLQ monitor (`codeforge-go-dlq-monitor`, durable, filters `*.*.dlq` to `*.*.*.*.*.dlq`) logs every dead
  letter and forwards it to the notifier.

**5. Dedup.** The worker's in-process dedup cache marks a request key when the request is accepted. A request whose
handler failed (or whose accept ack was not confirmed) is removed from the cache again, so its redelivery is
processed instead of being acked as a duplicate. Go handlers stay idempotent, as at-least-once delivery requires.

**6. Crashed workers and the Go-side run timeout.** At-most-once runs rely on the Go Core to end a run whose worker
died. What exists (checked on `staging`, 2026-09-30):

- `runs.start`: `RuntimeService.StartRun` arms a per-run timer when the policy has `TimeoutSeconds > 0` (all four
  built-in presets: 300 to 3600 s) that cancels the run. Not covered: policies with `TimeoutSeconds: 0`, runs in
  status `quality_gate` (the timer only cancels `running` runs), and any run after a Go Core restart (the timer
  lives in memory). `HeartbeatTimeout` and `AbsoluteMaxExecutionTimeout` are only evaluated when a tool call
  arrives, so they never fire for a dead worker.
- `conversation.run.start`: there is **no** Go-side watchdog; the worker's wall-clock timeout dies with the worker.
- `tasks.agent.*`: no run timer. `ActiveWorkService.ReleaseStaleWork` (every `StaleCheckInterval`, 60 s) resets
  tasks in status `running` or `queued` whose row was not updated for `StaleWorkThreshold` (30 min) to `pending`
  and clears their agent. The task is not failed and not re-dispatched, and the agent's status is not reset. A
  healthy task that runs longer than 30 min without a row update is released while it is still running.
- `benchmark.run.request`: the benchmark watchdog (every 5 min, state in the database) fails runs in status
  `running` that are older than 30 min (simple), 60 min (tool use) or 4 h (agent).

A worker that stops because a message loop gave up fails its own unfinished work (section 3), so these watchdogs
matter for workers that crash, are killed, or lose the connection to NATS. The gaps are follow-up work, not part of
this decision.

**7. Per-run subscriptions.** A run's cancel listeners (`runs.cancel` plus `tasks.cancel` or
`conversation.run.cancel`) and its heartbeat belong to the run: `RuntimeClient.close()` releases them and every run
handler calls it when the run ends, successfully or not. The cancel listeners and the per-call subscription for a
tool-call response are ephemeral consumers with deliver policy `new` and ack policy `none`: they are never acked,
and with explicit acks JetStream would redeliver each message after the ack wait and stop delivering once
`MaxAckPending` messages are outstanding (a long run would stop receiving its cancel). The cancel subjects are shared
by all active runs, so a malformed cancel message is skipped, never fatal to the listener, and empty run or task IDs
never match.

**8. Payload validation.** The Go validator checks every `runs.*`, `context.*` and `repomap.*` subject that has a
port struct against it, like the other subject families (`runs.start`, `runs.toolcall.request/response/result`,
`runs.complete`, `runs.output`, `runs.heartbeat`, `runs.qualitygate.request/result`, `context.shared.updated`,
`context.rerank.request/result`, `repomap.generate.request/result`); `runs.cancel` and `runs.trajectory.event` have
no port struct and stay JSON-only. A `null` payload is rejected like `{}`.

**9. Tests use the real server.** Go integration tests run against the NATS server in `NATS_URL` (restart without
replay, recreation without replay, legacy durable upgrade, two instances with handlers slower than `AckWait`,
retry exhaustion, DLQ). Worker tests use real `nats.aio.msg.Msg` objects with a recording client
(`workers/tests/jetstream_fakes.py`).

The constants live in `internal/adapter/nats/nats.go` and `workers/codeforge/nats_subjects.py`; the worker's
consumer provisioning, delivery count and heartbeat in `workers/codeforge/consumer/_delivery.py`.

### Consequences

#### Positive

- Restarts, expiries and recreated consumers never replay history; messages published while a process is down are
  delivered when it comes back.
- Several Go Core or worker instances share the work of a subject; a long handler is processed once.
- Poison messages end in `{subject}.dlq` instead of looping; transient failures are retried a bounded number of
  times; a failed request is no longer lost as a "duplicate".
- Runs no longer leak consumers and heartbeat tasks.

#### Negative

- Runs and backend tasks are at-most-once: if a worker dies mid-run, the run is not retried. For `runs.start` the
  Go run timeout fails it and a backend task is released back to `pending` after 30 min (within the gaps listed in
  section 6); a conversation run stays "running" until the user stops it, because no Go-side watchdog exists yet.
  The same holds when the server applied an accept ack whose confirmation was lost three times: the work never
  starts.
- A worker that gives up fails its unfinished accepted work after a 30 s grace period instead of letting it finish.
- A request/response handler that answered with an error result is not retried, even if the cause was transient;
  the Go caller decides whether to ask again.
- A message published while its durable does not exist (before the worker's first start on a new deployment, or
  after an external deletion until the durable is recreated) is not delivered to that consumer.
- A message whose dead-letter publish failed on the last attempt stays unacknowledged in the stream without a DLQ
  copy; operators see it only in the consumer info or the JetStream advisory.
- Go and worker must be deployed together (both change retry and ack behaviour).

#### Neutral

- The Go Core does not provision the worker's durables (the design reference proposed it): each side owns the
  durables of the subjects it consumes, so no consumer list is duplicated across languages.
- Notifications (cancel signals, permission decisions) still use ephemeral JetStream consumers (deliver policy
  `new`, ack policy `none`), per run and per tool call, instead of process-wide core NATS subscriptions.
- Durable names are contracts: a start-policy change needs a new durable name.

### Alternatives Considered

| Alternative | Pros | Cons | Why Not |
|---|---|---|---|
| In-progress heartbeats for runs (at-least-once) | A crashed worker's run is retried | The retry re-executes a partially applied run on a changed workspace | Runs are not idempotent; the Go Core owns run state |
| Ack on accept for every subject | Simplest; nothing is ever redelivered | No retries at all; a crash loses every request | Most requests are cheap to repeat and should survive a crash |
| Longer `AckWait` instead of heartbeats | No extra acks | Crash recovery waits as long as the longest handler; still wrong for handlers longer than the limit | Heartbeats keep recovery fast and handle any duration |
| Deliver policy `all` with recreation guarded by dedup | Nothing published before creation is missed | Replays up to 30 days; the dedup cache is empty after a restart | Replay executes old runs again |
| Go Core provisions all durables, including the worker's | A `runs.start` before the first worker start is kept | Worker subjects and consumer settings duplicated in Go | Cross-language duplication for a one-time gap |
| `Retry-Count` header set by the consumer on republish | Works without JetStream metadata | Republishing creates a new message; the header was never set | JetStream already counts deliveries |
| Embedded `nats-server` in Go tests | Hermetic tests | New test dependency; no Python or cross-language use | Minimal-dependency principle; a real server is available |

### References

- [ADR-001: NATS JetStream as Message Queue](001-nats-jetstream-message-queue.md)
- [Known Issues KI-18, KI-19, KI-20](../../todo.md#known-issues)
- [Fix plan, milestone S2](../../known-issues-fix-plan.md)
- JetStream consumers: <https://docs.nats.io/nats-concepts/jetstream/consumers>
