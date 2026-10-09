package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Strob0t/CodeForge/internal/netutil"
	"github.com/Strob0t/CodeForge/internal/port/gitprovider"
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

// KI-166: pm.allowed_private_hosts also reaches the GitHub providers the
// Core builds through the registries: the github-api git provider (PR
// delivery to a GitHub Enterprise Server) and the github-issues provider of
// an integration with its own base_url.
func TestSetPMOutboundPolicy_GitHub(t *testing.T) {
	t.Cleanup(func() { _ = setPMOutboundPolicy(nil) })
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	calls := map[string]func() error{
		"github-api": func() error {
			p, err := gitprovider.New("github-api", map[string]string{"token": "t", "base_url": srv.URL})
			if err != nil {
				t.Fatal(err)
			}
			_, err = p.ListRepos(context.Background())
			return err
		},
		"github-issues": func() error {
			p, err := pmprovider.New("github-issues", map[string]string{"token": "t", "base_url": srv.URL})
			if err != nil {
				t.Fatal(err)
			}
			_, err = p.ListItems(context.Background(), "acme/app")
			return err
		},
	}
	for name, call := range calls {
		if err := setPMOutboundPolicy(nil); err != nil {
			t.Fatal(err)
		}
		before := hits.Load()
		if err := call(); !errors.Is(err, netutil.ErrAddressRefused) || hits.Load() != before {
			t.Fatalf("%s without an allowlist: %v; want ErrAddressRefused before connecting", name, err)
		}
		if err := setPMOutboundPolicy([]string{"127.0.0.1"}); err != nil {
			t.Fatal(err)
		}
		if err := call(); err != nil || hits.Load() != before+1 {
			t.Fatalf("%s with 127.0.0.1 allowed: %v", name, err)
		}
	}
}
