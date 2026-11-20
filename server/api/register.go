// Thin adapter for Vercel's Go runtime (module mode): all logic lives in
// internal/handlers so it is testable without a deployment.
package handler

import (
	"net/http"

	"github.com/webdevshayan/school-attendance-system/server/internal/handlers"
)

// Register adds a signed device to the server's sync allowlist.
// Served at POST /api/register by Vercel's Go runtime.
func Register(w http.ResponseWriter, r *http.Request) {
	handlers.Register(handlers.DepsFromEnv(), w, r)
}
