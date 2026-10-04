package main

import (
	"github.com/Strob0t/CodeForge/internal/adapter/gitlab"
	"github.com/Strob0t/CodeForge/internal/netutil"
)

// setPMOutboundPolicy lets the GitLab PM provider, whose base URL tenants
// choose, connect to public addresses and to the private ones of allowed
// (pm.allowed_private_hosts; KI-85 review). An invalid list changes nothing.
func setPMOutboundPolicy(allowed []string) error {
	policy, err := netutil.NewOutboundPolicy(allowed)
	if err != nil {
		return err
	}
	gitlab.SetOutboundPolicy(policy)
	return nil
}
