// index.go is the Vercel Go framework entrypoint: with framework preset
// "Go", Vercel builds this file with @vercel/go and exposes its exported
// Handler behind a catch-all route, so every /api/* path lands here.
package handler

import "net/http"

// Handler dispatches one request to the matching API endpoint.
func Handler(w http.ResponseWriter, r *http.Request) {
	dispatch(w, r)
}
