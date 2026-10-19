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
LLM request, except quality gates: a gate request stays in progress as long as its gate sends heartbeats (`heartbeat_seconds` on the request, `runs.heartbeat` with phase `quality_gate`), and the Go stuck-work watchdog fails a gate whose heartbeats stop (KI-28). CPU-bound worker work (repo map, retrieval chunking, BM25 indexing and embedding decoding of the whole
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

> **Update (2026-09-30, KI-28, KI-65):** the gaps above are closed by a store-based stuck-work watchdog
> (`internal/service/stuck_work_watchdog.go`, every `runtime.stale_check_interval`, works across restarts and
> replicas). The worker sends heartbeats with `tenant_id` for runs (`runs.heartbeat`), conversation runs (with
> `turn_id`) and backend tasks (`tasks.heartbeat`); Go records them in the store (migration 098). Work whose heartbeats
> stop for `heartbeat_timeout + 2 x heartbeat_interval` (3 min by default) is ended through its normal path: runs are
> stopped as `timeout`, conversation runs get a failed completion for their active turn, tasks are failed and their
> agent reset. Work that never sent a heartbeat (still queued) is not ended by this check. Quality gates send
> heartbeats with phase `quality_gate` and are failed when silent and nothing is queued (NATS backlog probe).
> Dead-lettered starts are ended by Go subscribers on `runs.start.dlq` and `conversation.run.start.dlq`. The worker
> no longer creates the stream (it waits for the Go Core), dedups at-least-once requests per message (key plus stream
> sequence), keeps a registry of `tasks.cancel` messages so a task cancelled while queued is not started, and on
> SIGTERM gives accepted work 5 s, then fails it through the in-flight registry before draining NATS (prod compose
> `stop_grace_period: 45s`). `ReleaseStaleWork` was removed. Every Go publish carries the tenant as the
> `X-Tenant-ID` header, and the worker echoes it on what it publishes while handling a message (KI-64).

> **Update (2026-10-01, KI-15, S2-F review, S2-G):** further rules for work that outlives one message.
>
> - **Dispatch IDs.** Every dispatch of a backend task gets a `dispatch_id`, carried by `tasks.agent.*` and echoed on `tasks.heartbeat` and `tasks.result` (migration 099). A heartbeat counts only for the dispatch it was sent for, only the result of the task's current dispatch ends the task, and the cost of a dispatch is counted once (`task_result_costs`, migration 104, a late or redelivered result adds nothing again). An ended dispatch clears its `dispatch_id` and its heartbeat. A dispatch that no worker accepted within `runtime.task_accept_timeout` (default 1 h, `0` = never) is failed by the watchdog check "tasks never accepted".
> - **Heartbeat interval.** `heartbeat_seconds` (from `runtime.heartbeat_interval`) is sent on `runs.start`, `conversation.run.start` and `tasks.agent.*`; the configuration is validated at load (interval 0 or at least 1 s, timeout 0 or longer than the interval).
> - **Dead letters.** Go subscribes to `runs.start.dlq`, `conversation.run.start.dlq`, `tasks.agent.*.dlq`, `handoff.request.dlq`, `handoff.approved.dlq` and `benchmark.run.request.dlq` and ends the work they carried as failed in its tenant.
> - **Cancels.** The cancel listeners of tasks, runs and conversation runs replay from the stream sequence of the start message, so a cancel published between the start and the subscription is not lost. A start the worker skips because it was cancelled publishes a `cancelled` completion, so the Go Core ends the run; `tasks.cancel` no longer makes the worker skip run starts. A stop that could not be recorded replays every kind of deferred worker message in order.
> - **Delayed redelivery.** A handler may ask for a redelivery after a delay (`messagequeue.RetryAfter`); it counts as a delivery, the last one is dead-lettered as usual.
> - **Handoffs.** `handoff.request` (worker -> Go Core, with `handoff_id` and string metadata) and `handoff.approved` (Go Core only) are at-least-once but start a workspace-changing run, so the Go Core claims each stage in `handoff_claims` before it starts anything (migrations 102, 106, 107): a claim that was never marked done is taken over after an 11-minute lease (longer than the 10 minutes a running handler is kept in progress), a retry reuses the stage's task, a permanent start error refuses the handoff at once, a transient one is retried and dead-lettered after the last attempt.
> - **Conversation completions.** A turn's worker completion is claimed once (`conversation_turn_completions`, migration 105; the Go Core's own failed completions do not claim). The completion of a turn that is no longer the active one stores its messages and cost when no newer turn started, otherwise only its cost; tool calls of a turn that is not the active one are denied.

> **Update (2026-10-01, KI-71, [ADR-017](017-tool-isolation-and-nats-authentication.md)):** NATS requires authentication in the deployment, with per-user subject permissions. Delivery semantics stay as described here, with these changes.
>
> - **Notifications.** The per-listener ephemeral notification consumers of section 7 (cancels, tool-call responses) are replaced by the worker's `NotificationHub` (`workers/codeforge/notifications.py`): one shared named push consumer per notification subject (`codeforge-py-notify-<subject>`, ack none, deliver policy `new`), delivering to `_INBOX_worker.notify.<name>`, with fan-out to every worker instance. The hub never deletes a consumer.
> - **No cancel lost.** "Replay from the start message's stream sequence" now comes from reading the stream back with batched direct gets (the stream is created with `AllowDirect`), at subscription time, on every gap in the consumer sequences and after every reconnect. A read-back that fails or exceeds 100,000 messages fails the run or task.
> - **Permissions.** New subjects, and new worker durables or notification consumers, also need entries in `configs/nats/nats-server.conf`: the `core` and `worker` users publish only their own subjects, and the worker may use only consumers named there. `workers/tests/test_nats_permissions.py` and `internal/adapter/nats/auth_test.go` fail without them (they need a `nats-server` binary, `NATS_SERVER_BIN`). Both clients keep their inbox prefixes (`_INBOX_core`, `_INBOX_worker`).
> - **Version.** Production pins `nats:2.15-alpine`; batched direct get needs nats-server 2.11 or newer.

> **Update (2026-10-03, checklist for handlers):** where the rules of this ADR live in code, for anyone who adds or changes a subject or a handler.
>
> - **Subjects.** The catalog is `internal/port/messagequeue/queue.go` (`Subject*`) <-> `workers/codeforge/nats_subjects.py` (`workers/codeforge/consumer/_subjects.py` re-exports it); names match exactly. The stream's subject list (`CreateOrUpdateStream` in `internal/adapter/nats/nats.go`) needs a wildcard for every new prefix (e.g. `benchmark.>`), and every subject needs a publisher on one side and a subscriber on the other.
> - **Settling (worker).** The generic handler in `workers/codeforge/consumer/_base.py` settles every message exactly once: ack after success or after a published error result; `_retry_or_dead_letter` on failure (NAK with the 2 s delay, DLQ copy plus ack on the last delivery, counted from `num_delivered`); `_reject_invalid` for an invalid payload (DLQ copy plus `term`, never a NAK).
> - **At-most-once handlers** (the generic handler with `ack_on_accept=True`, or a handler that calls `self._accept(msg)` itself: conversations, tasks, benchmarks): the accept is a confirmed double ack (`ack_sync`, 3 attempts; unconfirmed: NAK, the work is not started); the handler registers its work with `self._in_flight.track(...)` and a callback that publishes the failed completion (this is not automatic); completions go through `_publish_result` / `publish_with_retry` (`workers/codeforge/nats_publish.py`, one `Nats-Msg-Id` for all attempts). Duplicates are skipped by the work's ID (`run-{run_id}`, `task-{task_id}@{dispatch_id}`, `bench-{run_id}`; conversation runs through the set of active run IDs), so no second message executes the same work.
> - **Duplicate guards (Go).** At-least-once results must be idempotent: a completion for a run that already ended (terminal status such as `completed`) only keeps the worker's usage totals (`HandleRunComplete`), a dispatch's cost counts once (`task_result_costs`), a turn's completion is claimed once (`conversation_turn_completions`).
> - **Quality gates.** `runs.qualitygate.request` carries `timeout_seconds` and `heartbeat_seconds`; a check that could not run or did not finish (no command, a command that is invalid, not allowed or cannot start, a timeout) is reported as a null verdict plus `error` (`workers/codeforge/qualitygate.py`), never as a failure: only a check that ran and failed rolls the workspace back.
> - **Workspace deletion.** `workspace.delete.request` is at-least-once (the worker deletes as the tenant's tool UID); Go subscribes to `workspace.delete.request.dlq` as well and counts a dead letter as a failed attempt, which the retry job republishes ([dev-setup](../../dev-setup.md#workspace-deletion-ki-96)).

**7. Per-run subscriptions.** A run's cancel listeners (`runs.cancel` plus `tasks.cancel` or
`conversation.run.cancel`) and its heartbeat belong to the run: `RuntimeClient.close()` releases them and every run
handler calls it when the run ends, successfully or not. The cancel listeners and the per-call subscription for a
tool-call response are ephemeral consumers with deliver policy `new` and ack policy `none` (since KI-71 the worker's `NotificationHub` provides the same subscriptions on shared named consumers, see the update above): they are never acked,
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

**10. Results reach the replica that waits (KI-86, 2026-10-04).** A shared durable consumer hands a result to any
Go Core replica, but the caller waits in the memory of one. A waiter therefore serves its key on core NATS
(`core.relay.<sha256(key)>`, request-reply outside the stream, `messagequeue.Relay`) while it waits; a replica with no
local waiter requests the key, and "no responders" means nobody waits. This covers syncWaiter results, HITL
decisions and approval lookups (keys contain the tenant), and the auto-agent's waits. Only the `core` user may use
`core.relay.>`.
A replica acts only on a relay request whose reply is in the Go Core's inboxes (`_INBOX_core.`, set on every Core
connection): the NATS server does not check a push consumer's deliver subject or the reply subject of a JetStream API
request, so a stream message the worker published can reach a relay subject, with a `$JS.ACK.*` reply or none, and is
dropped. A relayed result is the worker's message as it came (re-encoded without HTML escaping where the message is
not at hand), so it fits the payload limit like the original.

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
- Notifications (cancel signals, permission decisions) use JetStream consumers with deliver policy `new` and ack
  policy `none` instead of process-wide core NATS subscriptions: ephemeral ones per run and per tool call when this
  ADR was written, shared named ones in the worker's `NotificationHub` since KI-71 ([ADR-017](017-tool-isolation-and-nats-authentication.md)).
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
