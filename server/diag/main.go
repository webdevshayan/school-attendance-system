// Command diag exercises the Turso connection exactly like the deployed
// functions do (pkg/turso over the v2 pipeline API) and prints the
// failure reason. Temporary incident diagnostic (PR #6 follow-up) - the
// token is never printed; the database URL appears host-only.
package main

import (
	"context"
	"fmt"
	"net/url"
	"os"

	"github.com/webdevshayan/school-attendance-system/server/pkg/turso"
)

func main() {
	raw := os.Getenv("TURSO_DATABASE_URL")
	token := os.Getenv("TURSO_AUTH_TOKEN")
	fmt.Println("scheme  :", func() string {
		u, err := url.Parse(raw)
		if err != nil {
			return "unparseable: " + err.Error()
		}
		return u.Scheme
	}())
	fmt.Println("host    :", func() string {
		u, err := url.Parse(raw)
		if err != nil {
			return "?"
		}
		return u.Host
	}())
	fmt.Println("token   :", len(token), "chars")

	c := turso.New(raw, token)
	results, err := c.Execute(context.Background(), turso.Statement{SQL: "SELECT 1 AS one"})
	if err != nil {
		fmt.Println("SELECT 1 -> ERROR:", err)
		os.Exit(1)
	}
	fmt.Printf("SELECT 1 -> OK: %d row(s)\n", results[0].RowCount)
}
