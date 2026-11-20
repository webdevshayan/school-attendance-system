// Command smoke runs a full end-to-end pass against the LIVE deployment,
// exercising every endpoint in the order a real deployment would be used:
//
//	health -> register (signed) -> sync (signed) -> de-dup check ->
//	codes (signed) -> pair (code as credential) -> events (bearer, cursor)
//
// It is run by the Smoke Test workflow after every production deploy; it can
// also be triggered manually. Credentials come from the environment:
//
//	BASE_URL           the deployment to test (default: production)
//	SYNC_SIGNING_SECRET  the same secret Vercel has, so signatures verify
//
// The secret and the bearer token are never printed - only step outcomes.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/webdevshayan/school-attendance-system/server/pkg/authn"
)

const (
	smokeDevice  = "door-smoke"
	smokeStudent = "smoke-e2e"
)

var (
	baseURL = envDefault("BASE_URL", "https://rgs-attendance-system.vercel.app")
	secret  = os.Getenv("SYNC_SIGNING_SECRET")
	client  = &http.Client{Timeout: 30 * time.Second}
	failed  bool
)

func envDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// step runs one named check; a failure marks the run failed but the chain
// continues where meaningful, so one report shows everything that is wrong.
func step(name string, err error) {
	if err != nil {
		failed = true
		fmt.Printf("FAIL  %s: %v\n", name, err)
		return
	}
	fmt.Printf("ok    %s\n", name)
}

func get(path string, headers map[string]string) (int, []byte, error) {
	req, err := http.NewRequest(http.MethodGet, baseURL+path, nil)
	if err != nil {
		return 0, nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	return res.StatusCode, body, err
}

// signedPost sends a POST with the three HMAC headers, exactly like the
// admin app does for /api/sync and /api/register.
func signedPost(path string, deviceID string, payload any) (int, []byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, err
	}
	ts := time.Now().UTC().Format(time.RFC3339)
	req, err := http.NewRequest(http.MethodPost, baseURL+path, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(authn.DeviceHeader, deviceID)
	req.Header.Set(authn.TimestampHeader, ts)
	req.Header.Set(authn.SignatureHeader, authn.Sign(deviceID, ts, secret, body))
	res, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	return res.StatusCode, data, err
}

func post(path string, payload any) (int, []byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, err
	}
	res, err := client.Post(baseURL+path, "application/json", bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	return res.StatusCode, data, err
}

func object(body []byte) map[string]any {
	var m map[string]any
	_ = json.Unmarshal(body, &m) // callers inspect fields defensively
	return m
}

func main() {
	if secret == "" {
		fmt.Println("SYNC_SIGNING_SECRET is not set; cannot sign requests like a real door device")
		os.Exit(1)
	}

	// 1. Health: the deployment is alive and persistence is configured.
	status, body, err := get("/api/health", nil)
	health := object(body)
	step("health", err)
	if err == nil {
		if status != http.StatusOK || health["status"] != "ok" {
			step("health", fmt.Errorf("HTTP %d body %s", status, body))
		} else if health["turso"] != "configured" {
			step("health", fmt.Errorf("turso=%v - set TURSO_DATABASE_URL/TURSO_AUTH_TOKEN (bridged from Actions secrets on deploy)", health["turso"]))
		}
	}

	// 2. Register the smoke device (idempotent upsert on the allowlist).
	status, body, err = signedPost("/api/register", smokeDevice, map[string]string{
		"device_id": smokeDevice,
		"label":     "CI smoke test",
	})
	reg := object(body)
	step("register", err)
	if err == nil && (status != http.StatusOK || reg["registered"] != true) {
		step("register", fmt.Errorf("HTTP %d body %s", status, body))
	}

	// 3. Sync one attendance row (signed, unique timestamp per run).
	markedAt := time.Now().UTC().Format(time.RFC3339)
	row := map[string]string{
		"device_id":  smokeDevice,
		"student_id": smokeStudent,
		"status":     "in",
		"marked_at":  markedAt,
	}
	status, body, err = signedPost("/api/sync", smokeDevice, row)
	first := object(body)
	step("sync", err)
	if err == nil && (status != http.StatusAccepted || first["accepted"] != true || first["duplicate"] != false) {
		step("sync", fmt.Errorf("HTTP %d body %s", status, body))
	}

	// 4. The exact same row must come back as a duplicate (server-side
	// de-dup: a lost-ack retry never double-records attendance).
	status, body, err = signedPost("/api/sync", smokeDevice, row)
	second := object(body)
	step("sync de-dup", err)
	if err == nil && (status != http.StatusAccepted || second["duplicate"] != true) {
		step("sync de-dup", fmt.Errorf("HTTP %d body %s", status, body))
	}

	// 5. Mint a pairing code for the smoke student (signed door device).
	status, body, err = signedPost("/api/codes", smokeDevice, map[string]string{
		"student_id": smokeStudent,
	})
	code := object(body)
	step("codes", err)
	pairingCode, _ := code["code"].(string)
	if err == nil && (status != http.StatusOK || len(pairingCode) != 8) {
		step("codes", fmt.Errorf("HTTP %d body %s", status, body))
	}

	// 6. Pair a parent install with that code (the code is the credential).
	status, body, err = post("/api/pair", map[string]string{
		"code":     pairingCode,
		"label":    "CI smoke parent",
		"platform": "ci",
	})
	pair := object(body)
	step("pair", err)
	token, _ := pair["token"].(string)
	if err == nil && (status != http.StatusOK || pair["student_id"] != smokeStudent || len(token) != 64) {
		step("pair", fmt.Errorf("HTTP %d body %s", status, body))
	}

	// 7. The events feed must deliver the synced row to the paired token.
	status, body, err = get("/api/events?since=0", map[string]string{
		"Authorization": "Bearer " + token,
	})
	events := object(body)
	step("events", err)
	if err == nil {
		if status != http.StatusOK {
			step("events", fmt.Errorf("HTTP %d body %s", status, body))
		} else if !strings.Contains(string(body), markedAt) {
			step("events", fmt.Errorf("synced row (marked_at %s) missing from the feed", markedAt))
		} else if cursor, ok := events["cursor"].(float64); !ok || cursor < 1 {
			step("events", fmt.Errorf("cursor = %v, want >= 1", events["cursor"]))
		}
	}

	if failed {
		fmt.Println("E2E FAILED")
		os.Exit(1)
	}
	fmt.Println("E2E OK: every endpoint behaved per contract against", baseURL)
}
