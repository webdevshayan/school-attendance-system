// Command server runs the API locally with the exact routing the Vercel
// deployment uses (framework preset "Go" builds index.go; preset "Other"
// builds the per-file api/ functions; both dispatch through pkg/routes):
//
//	TURSO_DATABASE_URL=... TURSO_AUTH_TOKEN=... SYNC_SIGNING_SECRET=... \
//	  go run ./cmd/server    # from server/
//	curl localhost:3000/api/health
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
