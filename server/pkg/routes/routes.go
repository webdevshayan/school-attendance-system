// Package routes mounts the whole API on one mux. It is the single source of
// routing shared by the ways this service runs:
//
//   - main.go             the Vercel Go framework entrypoint (preset "Go") and
//     the local development server
//   - api/*.go            per-file functions (framework preset "Other", module mode)
//
// Configuration is read from the environment exactly like the api/ adapters
// do it (DepsFromEnv per mux), so every shape behaves identically.
package routes

import (
	"net/http"

	"github.com/webdevshayan/school-attendance-system/server/pkg/handlers"
)

// New builds a mux with every endpoint.
func New() *http.ServeMux {
	d := handlers.DepsFromEnv()
	mux := http.NewServeMux()
	mount := func(path string, h func(handlers.Deps, http.ResponseWriter, *http.Request)) {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			h(d, w, r)
		})
	}
	mount("/api/health", handlers.Health)
	mount("/api/register", handlers.Register)
	mount("/api/sync", handlers.Sync)
	mount("/api/codes", handlers.MintCode)
	mount("/api/pair", handlers.Pair)
	mount("/api/events", handlers.Events)
	return mux
}
