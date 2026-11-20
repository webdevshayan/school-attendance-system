// Command diag runs a MintCode -> FindCode round-trip against the live
// Turso using pkg/store (the exact code path the pair endpoint uses) and
// prints the raw read-back record. Temporary incident diagnostic.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/webdevshayan/school-attendance-system/server/pkg/store"
	"github.com/webdevshayan/school-attendance-system/server/pkg/turso"
)

func main() {
	client := turso.New(os.Getenv("TURSO_DATABASE_URL"), os.Getenv("TURSO_AUTH_TOKEN"))
	st := store.NewTurso(client)
	ctx := context.Background()

	if err := st.Init(ctx); err != nil {
		fmt.Println("Init ERROR:", err)
		os.Exit(1)
	}
	fmt.Println("Init OK")

	now := time.Now().UTC()
	code := store.PairingCode{
		Code:      "DIAGTEST",
		StudentID: "smoke-e2e",
		CreatedAt: now.Format(time.RFC3339),
		ExpiresAt: now.Add(7 * 24 * time.Hour).Format(time.RFC3339),
	}
	if err := st.MintCode(ctx, code); err != nil {
		fmt.Println("MintCode ERROR:", err)
		os.Exit(1)
	}
	fmt.Println("MintCode OK, wrote expires_at:", code.ExpiresAt)

	found, ok, err := st.FindCode(ctx, "DIAGTEST")
	if err != nil {
		fmt.Println("FindCode ERROR:", err)
		os.Exit(1)
	}
	fmt.Printf("FindCode ok=%v record=%+v\n", ok, found)

	events, err := st.EventsSince(ctx, "smoke-e2e", 0, 5)
	if err != nil {
		fmt.Println("EventsSince ERROR:", err)
		os.Exit(1)
	}
	fmt.Printf("EventsSince rows=%d first=%+v\n", len(events), func() any {
		if len(events) > 0 {
			return events[0]
		}
		return "none"
	}())
}
