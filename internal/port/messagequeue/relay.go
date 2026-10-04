package messagequeue

import "context"

// Relay hands a result or a decision to the Go Core replica that waits for
// it (KI-86). Worker results reach whichever replica takes them from a
// shared durable consumer, and HTTP decisions whichever replica the load
// balancer picks; during a blue-green switch two replicas run, and the
// waiter may be the other one. A waiter serves its key while it waits, and
// a replica that holds a result without a local waiter requests the key.
//
// A queue offers the relay as an optional capability (the NATS queue does,
// through core NATS request-reply between the Go Core's own connections);
// without one, results reach only the waiters of the replica that takes them.
type Relay interface {
	// Serve answers the requests sent to key on this replica until stop is
	// called. handle returns the answer, nil when it has none. It runs on
	// the queue's delivery goroutine and must not block.
	Serve(key string, handle func(data []byte) []byte) (stop func(), err error)

	// Request sends data to the replica that serves key and returns its
	// answer: nil when no replica serves key or the one that does had no
	// answer; an error when the relay itself failed.
	Request(ctx context.Context, key string, data []byte) ([]byte, error)
}

// RelayOf returns the relay of q, nil when q offers none.
func RelayOf(q Queue) Relay {
	r, _ := q.(Relay)
	return r
}
