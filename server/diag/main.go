// Command diag probes the live Turso with the deployed credentials: a
// MintCode -> FindCode round-trip through pkg/store, plus the RAW response
// body of a SELECT so the exact wire shape is visible. Temporary incident
// diagnostic - the token is never printed.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/webdevshayan/school-attendance-system/server/pkg/store"
	"github.com/webdevshayan/school-attendance-system/server/pkg/turso"
)

// rawSelect posts one SELECT straight to /v2/pipeline and prints the body.
func rawSelect(baseURL, token string) {
	payload := `{"requests":[{"type":"execute","stmt":{"sql":"SELECT code, student_id, created_at, expires_at FROM pairing_codes LIMIT 2"}},{"type":"close"}]}`
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/v2/pipeline", bytes.NewReader([]byte(payload)))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println("raw SELECT transport ERROR:", err)
		return
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	fmt.Printf("raw SELECT HTTP %d body: %s\n", res.StatusCode, string(body))
}

func main() {
	baseURL := os.Getenv("TURSO_DATABASE_URL")
	token := os.Getenv("TURSO_AUTH_TOKEN")
	client := turso.New(baseURL, token)
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

	rawSelect(normalizeForLog(baseURL), token)
	_ = json.Valid // keep encoding/json imported for future probes
}

// normalizeForLog only shapes the URL for printing (never prints the token).
func normalizeForLog(raw string) string {
	c := turso.New(raw, "t")
	return c.BaseURL
}

var _ = context.Background
