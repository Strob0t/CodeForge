package http

import "time"

// LongRunningRoutes are the API route patterns that do not fit the default
// request timeout (KI-213), with their own bound for middleware.RouteTimeout:
// synchronous git operations get gitOperation (git.operation_timeout), the
// A2A task stream none (0); it ends with its task or its client.
func LongRunningRoutes(gitOperation time.Duration) map[string]time.Duration {
	return map[string]time.Duration{
		"/api/v1/projects/{id}/clone":      gitOperation,
		"/api/v1/projects/{id}/setup":      gitOperation,
		"/api/v1/projects/{id}/git/pull":   gitOperation,
		"/api/v1/projects/batch/pull":      gitOperation,
		"/api/v1/a2a/tasks/{id}/subscribe": 0,
	}
}
