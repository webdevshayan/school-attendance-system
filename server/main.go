// Command server is the Vercel Go framework entrypoint - and, being a plain
// package-main HTTP server, also the local development server.
//
// The Go framework preset ("framework": "go", the dashboard's "Go") detects
// this project root's go.mod plus the main.go entrypoint and builds ONE
// server binary. The entrypoint contract (vercel.com/docs/functions/runtimes/
// go) is strict: package main, listening on the PORT environment variable.
// The 2026-11-20/21 production incidents ("fork/exec ./user-server: exec
// format error", FUNCTION_INVOCATION_FAILED) came from a non-main package
// sitting here: the builder had no runnable server to compile, so the
// runtime executed a placeholder artifact.
//
// Routing is shared: the same pkg/routes mux serves this binary and the
// api/*.go per-file functions that the pipeline falls back to (preset
// "Other") when a Go-framework build cannot be verified.
package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/webdevshayan/school-attendance-system/server/pkg/routes"
)

func main() {
	addr := os.Getenv("PORT")
	switch {
	case addr == "":
		addr = ":3000"
	case !strings.Contains(addr, ":"):
		addr = ":" + addr
	}
	log.Printf("school-attendance-server listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, routes.New()))
}
