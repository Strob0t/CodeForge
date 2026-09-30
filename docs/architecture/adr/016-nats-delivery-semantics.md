# ADR-016: NATS Delivery, Acknowledgement and Retry Semantics

> **Status:** accepted (implemented in milestone S2 of the [fix plan](../../known-issues-fix-plan.md), KI-18, KI-19, KI-20)
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
`new`), replacing the consume context of the subscription.

**3. Two acknowledgement modes.**

| Mode | Subjects | Ack | Failure | Crashed instance |
|---|---|---|---|---|
| at-most-once (ack on accept) | worker: `runs.start`, `conversation.run.start`, `benchmark.run.request` (already ran in the background) | before the handler runs | reported as a failed completion (`runs.complete`, `conversation.run.complete`, `benchmark.run.result`), never retried | run is failed by the Go Core (section 6) |
| at-least-once | all other subjects, Go and worker | after the handler succeeded | retried, then dead-lettered (section 4) | redelivered after `AckWait` |

Agent and conversation runs change the workspace and are not idempotent: a redelivery would execute a partially
applied run a second time on another worker. Everything else is a request whose repetition is harmless (index,
search, repo map, quality gate, memory, review trigger, ...). While an at-least-once handler runs, its consumer sends
an in-progress ack (`+WPI`) every `AckWait / 3` (Go: in `handleMessage`; worker: in the message loop around every
handler, stopping as soon as the message is settled), so a handler that is slower than `AckWait` is not redelivered
to another instance, while a crashed instance stops the heartbeat and its message is redelivered.

**4. Retries and dead letters (both sides).**

- The attempt number is JetStream's delivery count (`NumDelivered` / `metadata.num_delivered`, 1-based). The
  `Retry-Count` header is gone.
- A failure before attempt 4 is NAK'd with a 2 s delay.
- A failure on attempt 4 is published to `{subject}.dlq` (headers kept) and then acked.
- An **invalid payload** (Go: validator; worker: schema validation error, a payload that is not a JSON object, or a
  missing required field) is published to `{subject}.dlq` at once and **terminated** (`Term`): it is never NAK'd
  and never redelivered.
- If the dead-letter publish fails, the message is NAK'd with the delay instead of being settled, so it is never
  acknowledged without a dead-letter copy. After the last attempt JetStream then keeps it unacknowledged (visible
  as pending in the consumer info and in the max-deliveries advisory) instead of redelivering it.
- The Go DLQ monitor (`codeforge-go-dlq-monitor`, durable, filters `*.*.dlq` to `*.*.*.*.*.dlq`) logs every dead
  letter and forwards it to the notifier.

**5. Dedup.** The worker's in-process dedup cache marks a request key when the request is accepted. A request whose
handler failed is removed from the cache again, so its redelivery is processed instead of being acked as a
duplicate. Go handlers stay idempotent, as at-least-once delivery requires.

**6. Crashed workers and the Go-side run timeout.** At-most-once runs rely on the Go Core to end a run whose worker
died. What exists (checked on `staging`, 2026-09-30):

- `runs.start`: `RuntimeService.StartRun` arms a per-run timer when the policy has `TimeoutSeconds > 0` (all four
  built-in presets: 300 to 3600 s) that cancels the run. Not covered: policies with `TimeoutSeconds: 0`, runs in
  status `quality_gate` (the timer only cancels `running` runs), and any run after a Go Core restart (the timer
  lives in memory). `HeartbeatTimeout` and `AbsoluteMaxExecutionTimeout` are only evaluated when a tool call
  arrives, so they never fire for a dead worker.
- `conversation.run.start`: there is **no** Go-side watchdog; the worker's wall-clock timeout dies with the worker.

These gaps are follow-up work, not part of this decision.

**7. Per-run subscriptions.** A run's cancel listeners (`runs.cancel` plus `tasks.cancel` or
`conversation.run.cancel`, ephemeral consumers with deliver policy `new`) and its heartbeat belong to the run:
`RuntimeClient.close()` releases them and every run handler calls it when the run ends, successfully or not.

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

- Runs are at-most-once: if a worker dies mid-run, the run is not retried. For `runs.start` the Go run timeout
  fails it (within the gaps listed in section 6); a conversation run stays "running" until the user stops it,
  because no Go-side watchdog exists yet.
- A message published while its durable does not exist (before the worker's first start on a new deployment, or
  after an external deletion until the durable is recreated) is not delivered to that consumer.
- A message whose dead-letter publish failed on the last attempt stays unacknowledged in the stream without a DLQ
  copy; operators see it only in the consumer info or the JetStream advisory.
- Go and worker must be deployed together (both change retry and ack behaviour).

#### Neutral

- The Go Core does not provision the worker's durables (the design reference proposed it): each side owns the
  durables of the subjects it consumes, so no consumer list is duplicated across languages.
- Notifications (cancel signals, permission decisions) still use ephemeral JetStream consumers with deliver policy
  `new`, per run and per tool call, instead of process-wide core NATS subscriptions.
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
