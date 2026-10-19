package main

import (
	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	"github.com/Strob0t/CodeForge/internal/middleware"
)

// apiRouteOptions are the options the API routes are mounted with: the
// stricter rate limiter of the auth endpoints and the audit store, without
// which no audit middleware writes anything and GET /audit-logs does not
// exist (KI-172).
func apiRouteOptions(authRL *middleware.RateLimiter, auditStore cfhttp.AuditDB) []cfhttp.RouteOption {
	return []cfhttp.RouteOption{cfhttp.WithAuthRateLimiter(authRL), cfhttp.WithAuditStore(auditStore)}
}
