package main

import (
	"github.com/Strob0t/CodeForge/internal/adapter/github"
	"github.com/Strob0t/CodeForge/internal/adapter/githubpm"
	"github.com/Strob0t/CodeForge/internal/adapter/gitlab"
	"github.com/Strob0t/CodeForge/internal/netutil"
)

// setPMOutboundPolicy lets the providers whose API URL tenants choose - the
// GitLab and GitHub Issues PM providers and the github-api git provider (PR
// delivery, GitHub Enterprise Server, KI-166) - connect to public addresses
// and to the private ones of allowed (pm.allowed_private_hosts; KI-85
// review). An invalid list changes nothing.
func setPMOutboundPolicy(allowed []string) error {
	policy, err := netutil.NewOutboundPolicy(allowed)
	if err != nil {
		return err
	}
	gitlab.SetOutboundPolicy(policy)
	githubpm.SetOutboundPolicy(policy)
	github.SetOutboundPolicy(policy)
	return nil
}
