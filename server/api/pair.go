// Thin adapter for Vercel's Go runtime (module mode): all logic lives in
// pkg/handlers so it is testable without a deployment.
package handler

import (
	"net/http"

	"github.com/webdevshayan/school-attendance-system/server/pkg/handlers"
)

// Pair redeems a pairing code and registers the parent install.
// Served at POST /api/pair by Vercel's Go runtime.
func Pair(w http.ResponseWriter, r *http.Request) {
	handlers.Pair(handlers.DepsFromEnv(), w, r)
}
