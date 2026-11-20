// Thin adapter for Vercel's Go runtime (module mode): all logic lives in
// internal/handlers so it is testable without a deployment.
package handler

import (
	"net/http"

	"github.com/webdevshayan/school-attendance-system/server/pkg/handlers"
)

// Health reports liveness plus which integrations are configured.
// Served at GET /api/health by Vercel's Go runtime.
func Health(w http.ResponseWriter, r *http.Request) {
	handlers.Health(handlers.DepsFromEnv(), w, r)
}
