package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Strob0t/CodeForge/internal/netutil"
	"github.com/Strob0t/CodeForge/internal/port/pmprovider"
)

// KI-85 review: pm.allowed_private_hosts reaches the GitLab PM provider the
// syncs build through the registry; without an entry, private and loopback
// addresses stay refused.
func TestSetPMOutboundPolicy(t *testing.T) {
	t.Cleanup(func() { _ = setPMOutboundPolicy(nil) })
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`[{"iid":1,"title":"t","state":"opened"}]`))
	}))
	defer srv.Close()
	listItems := func() error {
		p, err := pmprovider.New("gitlab", map[string]string{"base_url": srv.URL})
		if err != nil {
			t.Fatal(err)
		}
		_, err = p.ListItems(context.Background(), "group/app")
		return err
	}

	if err := setPMOutboundPolicy([]string{"127.0.0.1"}); err != nil {
		t.Fatalf("setPMOutboundPolicy: %v", err)
	}
	if err := listItems(); err != nil || hits.Load() != 1 {
		t.Fatalf("allowlisted loopback: %v, %d requests; want the sync to reach it", err, hits.Load())
	}

	if err := setPMOutboundPolicy(nil); err != nil {
		t.Fatalf("setPMOutboundPolicy(nil): %v", err)
	}
	if err := listItems(); !errors.Is(err, netutil.ErrAddressRefused) || hits.Load() != 1 {
		t.Fatalf("no allowlist: %v, %d requests; want ErrAddressRefused before connecting", err, hits.Load())
	}

	if err := setPMOutboundPolicy([]string{"https://gitlab.corp.internal"}); err == nil {
		t.Fatal("an invalid entry was accepted")
	}
}
