// Thin adapter for Vercel's Go runtime (module mode): all logic lives in
// pkg/handlers so it is testable without a deployment.
package handler

import (
	"net/http"

	"github.com/webdevshayan/school-attendance-system/server/pkg/handlers"
)

// Events is the paired parent install's polling feed (bearer token,
// cursor based). Served at GET /api/events by Vercel's Go runtime.
func Events(w http.ResponseWriter, r *http.Request) {
	handlers.Events(handlers.DepsFromEnv(), w, r)
}
