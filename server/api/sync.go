// Thin adapter for Vercel's Go runtime (module mode): all logic lives in
// internal/handlers so it is testable without a deployment.
package handler

import (
	"net/http"

	"github.com/webdevshayan/school-attendance-system/server/internal/handlers"
)

// Sync accepts one attendance row from a signed, registered admin device.
// Served at POST /api/sync by Vercel's Go runtime.
//
// Contract notes: the runtime compiles the whole `package handler` per route
// and renames this file's exported handler (found by signature, any name)
// during that route's build - so each api/*.go file exports a DISTINCTLY
// named function with the standard (http.ResponseWriter, *http.Request)
// signature, and calls into internal/handlers must stay qualified
// (handlers.Sync), never bare, so the rename cannot touch them.
func Sync(w http.ResponseWriter, r *http.Request) {
	handlers.Sync(handlers.DepsFromEnv(), w, r)
}
