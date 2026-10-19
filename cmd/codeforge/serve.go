package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
)

// serveHTTP binds srv.Addr and serves srv in the background. A bind failure
// (the port is taken) is returned at once; a later serve failure arrives on
// the returned channel. Either way run() stops: a Core without its API must
// not keep consuming NATS work (KI-213). srv.Addr is set to the bound
// address.
func serveHTTP(ctx context.Context, srv *http.Server) (<-chan error, error) {
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", srv.Addr)
	if err != nil {
		return nil, fmt.Errorf("listen %s: %w", srv.Addr, err)
	}
	srv.Addr = ln.Addr().String()
	serveErr := make(chan error, 1)
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()
	return serveErr, nil
}
