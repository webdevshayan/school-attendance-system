// Thin adapter for Vercel's Go runtime (module mode): all logic lives in
// pkg/handlers so it is testable without a deployment.
package handler

import (
	"net/http"

	"github.com/webdevshayan/school-attendance-system/server/pkg/handlers"
)

// CodesRevoke invalidates a minted pairing code. Door-device
// authenticated (HMAC, like /api/codes). Served at POST /api/codes/revoke.
func CodesRevoke(w http.ResponseWriter, r *http.Request) {
	handlers.RevokeCode(handlers.DepsFromEnv(), w, r)
}
