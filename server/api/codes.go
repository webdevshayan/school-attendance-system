// Thin adapter for Vercel's Go runtime (module mode): all logic lives in
// pkg/handlers so it is testable without a deployment.
package handler

import (
	"net/http"

	"github.com/webdevshayan/school-attendance-system/server/pkg/handlers"
)

// Codes mints a parent-pairing code for a student. Door-device
// authenticated (HMAC, like /api/sync). Served at POST /api/codes.
func Codes(w http.ResponseWriter, r *http.Request) {
	handlers.MintCode(handlers.DepsFromEnv(), w, r)
}
