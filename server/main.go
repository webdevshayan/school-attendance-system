// Package handler (project root) hosts the Vercel Go framework entrypoint.
//
// The Go framework preset ("Go" in the dashboard, slug "go") builds this
// directory's index.go with @vercel/go: ONE serverless function whose
// exported Handler sits behind a catch-all route, so every /api/* path lands
// here and is dispatched internally. The dispatch itself lives in main.go so
// the layout matches what the preset's detector expects (go.mod + main.go)
// while index.go stays a pure adapter.
//
// The builder refuses "package main" entrypoints when a go.mod exists (the
// generated bootstrap owns main()), which is why this root package is
// "handler" and the runnable development server lives in cmd/server.
package handler

import (
	"net/http"

	"github.com/webdevshayan/school-attendance-system/server/pkg/routes"
)

// dispatch sends one request through the shared mux - the same routing the
// per-file api/ functions and the local cmd/server use.
func dispatch(w http.ResponseWriter, r *http.Request) {
	routes.New().ServeHTTP(w, r)
}
