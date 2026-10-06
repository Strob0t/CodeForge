package http

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
)

// API key scopes (KI-175). A key created with scopes may only call the
// routes of its scope groups; a key without scopes keeps its user's full
// rights (keys from before the enforcement). The user's role still applies.
//
// Every route under /api/v1 belongs to one group, by the first matching
// rule of apiKeyScopeRules on the path below /api/v1 (a rule covers its
// path and everything below it; "*" matches one segment):
//
//	projects  projects:read / projects:write   projects, files, git, roadmap,
//	                                           reviews, retrieval scopes, memories,
//	                                           microagents, skills, costs, dashboard
//	runs      runs:read / runs:write           runs, tasks, plans, conversations,
//	                                           sessions, policies, modes, pipelines,
//	                                           routing, benchmarks, goal discovery
//	agents    agents:read / agents:write       agents, their inbox and state, A2A
//	admin     admin:all                        everything else: auth, users, tenants,
//	                                           settings, LLM models and keys, MCP,
//	                                           webhooks, knowledge bases, channels,
//	                                           quarantine, audit logs, and any route
//	                                           not named here
//	none      (no scope)                       the API version and GET /auth/me
//
// GET and HEAD need the group's read scope, every other method its write
// scope, except a POST to a read-only query that takes a body (search,
// recall, evaluate, preview: the read scope); admin:all satisfies every
// scope. A route is classified the way chi dispatches it: by the pattern
// of the route the raw path matches (requireAPIKeyScope), so an escaped "/"
// in an ID stays in its segment.
type scopeGroup string

const (
	scopeGroupProjects scopeGroup = "projects"
	scopeGroupRuns     scopeGroup = "runs"
	scopeGroupAgents   scopeGroup = "agents"
	scopeGroupAdmin    scopeGroup = "admin"
	scopeGroupNone     scopeGroup = "none"
)

type scopeRule struct {
	pattern string
	group   scopeGroup
	// readOnly: a query that takes a body; a POST needs the read scope only.
	readOnly bool
}

// apiKeyScopeRules: the first matching rule wins, so a project's
// sub-resources that run or schedule agents come before "/projects".
var apiKeyScopeRules = []scopeRule{
	{pattern: "/source", group: scopeGroupNone},
	{pattern: "/auth/me", group: scopeGroupNone},

	{pattern: "/search", group: scopeGroupProjects, readOnly: true},
	{pattern: "/parse-repo-url", group: scopeGroupProjects, readOnly: true},
	{pattern: "/projects/*/search/agent", group: scopeGroupProjects},
	{pattern: "/projects/*/search", group: scopeGroupProjects, readOnly: true},
	{pattern: "/projects/*/graph/search", group: scopeGroupProjects, readOnly: true},
	{pattern: "/projects/*/memories/recall", group: scopeGroupProjects, readOnly: true},
	{pattern: "/scopes/*/search", group: scopeGroupProjects, readOnly: true},
	{pattern: "/scopes/*/graph/search", group: scopeGroupProjects, readOnly: true},
	{pattern: "/policies/*/evaluate", group: scopeGroupRuns, readOnly: true},
	{pattern: "/prompt-sections/preview", group: scopeGroupRuns, readOnly: true},

	{pattern: "/projects/*/conversations", group: scopeGroupRuns},
	{pattern: "/projects/*/tasks", group: scopeGroupRuns},
	{pattern: "/projects/*/active-work", group: scopeGroupRuns},
	{pattern: "/projects/*/plans", group: scopeGroupRuns},
	{pattern: "/projects/*/decompose", group: scopeGroupRuns},
	{pattern: "/projects/*/plan-feature", group: scopeGroupRuns},
	{pattern: "/projects/*/auto-agent", group: scopeGroupRuns},
	{pattern: "/projects/*/goals/ai-discover", group: scopeGroupRuns},
	{pattern: "/projects/*/sessions", group: scopeGroupRuns},
	{pattern: "/projects/*/audit", group: scopeGroupRuns},
	{pattern: "/projects/*/review-refactor", group: scopeGroupRuns},
	{pattern: "/projects/*/agents", group: scopeGroupAgents},
	{pattern: "/projects/*/webhooks", group: scopeGroupAdmin},
	{pattern: "/projects/*/mcp-servers", group: scopeGroupAdmin},
	{pattern: "/projects", group: scopeGroupProjects},
	{pattern: "/search", group: scopeGroupProjects},
	{pattern: "/detect-stack", group: scopeGroupProjects},
	{pattern: "/repos", group: scopeGroupProjects},
	{pattern: "/providers", group: scopeGroupProjects},
	{pattern: "/backends", group: scopeGroupProjects},
	{pattern: "/milestones", group: scopeGroupProjects},
	{pattern: "/features", group: scopeGroupProjects},
	{pattern: "/branch-rules", group: scopeGroupProjects},
	{pattern: "/review-policies", group: scopeGroupProjects},
	{pattern: "/reviews", group: scopeGroupProjects},
	{pattern: "/scopes", group: scopeGroupProjects},
	{pattern: "/microagents", group: scopeGroupProjects},
	{pattern: "/skills", group: scopeGroupProjects},
	{pattern: "/experience", group: scopeGroupProjects},
	{pattern: "/costs", group: scopeGroupProjects},
	{pattern: "/dashboard", group: scopeGroupProjects},
	{pattern: "/goals", group: scopeGroupProjects},

	{pattern: "/runs", group: scopeGroupRuns},
	{pattern: "/tasks", group: scopeGroupRuns},
	{pattern: "/plans", group: scopeGroupRuns},
	{pattern: "/sessions", group: scopeGroupRuns},
	{pattern: "/conversations", group: scopeGroupRuns},
	{pattern: "/feedback", group: scopeGroupRuns},
	{pattern: "/audit", group: scopeGroupRuns},
	{pattern: "/pipelines", group: scopeGroupRuns},
	{pattern: "/modes", group: scopeGroupRuns},
	{pattern: "/policies", group: scopeGroupRuns},
	{pattern: "/agent-config", group: scopeGroupRuns},
	{pattern: "/commands", group: scopeGroupRuns},
	{pattern: "/prompt-sections", group: scopeGroupRuns},
	{pattern: "/prompt-evolution", group: scopeGroupRuns},
	{pattern: "/teams", group: scopeGroupRuns},
	{pattern: "/routing", group: scopeGroupRuns},
	{pattern: "/benchmarks", group: scopeGroupRuns},
	{pattern: "/dev", group: scopeGroupRuns},

	{pattern: "/agents", group: scopeGroupAgents},
	{pattern: "/a2a", group: scopeGroupAgents},
}

// apiPrefix is where the API routes are mounted.
const apiPrefix = "/api/v1"

// APIKeyScope returns the scope an API key needs for method and path, a
// request path or a route pattern (a {param} segment matches "*"), below
// apiPrefix or not: "" when none is needed.
func APIKeyScope(method, path string) string {
	segments := pathSegments(strings.TrimPrefix(path, apiPrefix))
	group := scopeGroupAdmin
	if len(segments) == 0 {
		group = scopeGroupNone
	}
	readOnly := false
	for _, rule := range apiKeyScopeRules {
		if matchesPrefix(pathSegments(rule.pattern), segments) {
			group, readOnly = rule.group, rule.readOnly
			break
		}
	}
	switch group {
	case scopeGroupNone:
		return ""
	case scopeGroupAdmin:
		return user.ScopeAdminAll
	}
	switch method {
	case http.MethodGet, http.MethodHead:
		return string(group) + ":read"
	case http.MethodPost:
		if readOnly {
			return string(group) + ":read"
		}
	}
	return string(group) + ":write"
}

// pathSegments splits a path into its non-empty segments.
func pathSegments(path string) []string {
	var segments []string
	for _, s := range strings.Split(path, "/") {
		if s != "" {
			segments = append(segments, s)
		}
	}
	return segments
}

// matchesPrefix reports whether pattern matches the first segments of path,
// "*" matching any one segment.
func matchesPrefix(pattern, path []string) bool {
	if len(pattern) > len(path) {
		return false
	}
	for i, p := range pattern {
		if p != "*" && p != path[i] {
			return false
		}
	}
	return true
}

// requireAPIKeyScope checks a scoped API key against the scope of the route
// of api (the router it is used on) that the request dispatches to. The
// route is found the way chi routes: by method, on the raw path below the
// mount (chi.Context.RoutePath, params not unescaped), so an encoded "/" in
// an ID cannot move a request under another rule, and a method without a
// route on the path (405) is a write. A path without any route is
// classified by the table as before.
func requireAPIKeyScope(api chi.Routes) func(http.Handler) http.Handler {
	return middleware.RequireScopeFunc(func(r *http.Request) string {
		path := rawRoutePath(r)
		if pattern := api.Find(chi.NewRouteContext(), r.Method, path); pattern != "" {
			path = pattern
		}
		return APIKeyScope(r.Method, path)
	})
}

// rawRoutePath is the path chi routes the request on: what the parent
// router left below its mount, else the URL's raw path.
func rawRoutePath(r *http.Request) string {
	if rctx := chi.RouteContext(r.Context()); rctx != nil && rctx.RoutePath != "" {
		return rctx.RoutePath
	}
	if r.URL.RawPath != "" {
		return r.URL.RawPath
	}
	return r.URL.Path
}
