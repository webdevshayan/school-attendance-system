// Package handlers orchestrates the public endpoints: health, device
// registration and the signed attendance sync. The api/*.go files under
// Vercel's Go runtime are thin adapters that call into this package, so all
// logic lives in one testable place.
//
// Configuration comes from Vercel environment variables:
//
//	SYNC_SIGNING_SECRET   HMAC secret shared with admin devices (required)
//	TURSO_DATABASE_URL    libSQL URL (required for persistence)
//	TURSO_AUTH_TOKEN      libSQL token (required for persistence)
//	NITROPING_WEBHOOK_URL push relay endpoint (optional, best-effort)
//	NITROPING_API_KEY     push relay bearer token (optional)
package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/webdevshayan/school-attendance-system/server/internal/authn"
	"github.com/webdevshayan/school-attendance-system/server/internal/fanout"
	"github.com/webdevshayan/school-attendance-system/server/internal/store"
	"github.com/webdevshayan/school-attendance-system/server/internal/turso"
)

// maxBodyBytes caps the request body so a misbehaving device can't blow up
// the handler. Real attendance rows are a few hundred bytes; 1 MB is generous.
const maxBodyBytes = 1 << 20

// The Turso store is a per-serverless-instance singleton: the schema Init is
// memoized on it, so building a fresh store per request would re-run Init
// (or, worse, miss it entirely). Env vars are constant per instance.
var (
	storeOnce   sync.Once
	sharedStore store.Store
)

func depStore() store.Store {
	storeOnce.Do(func() {
		url, token := os.Getenv("TURSO_DATABASE_URL"), os.Getenv("TURSO_AUTH_TOKEN")
		if url != "" && token != "" {
			sharedStore = store.NewTurso(turso.New(url, token))
		}
	})
	return sharedStore
}

// Deps is the full dependency set for the endpoint handlers. A nil Store or
// Pusher means that integration is not configured; Sync/Register answer 503
// (Store) or skip silently (Pusher) accordingly.
type Deps struct {
	Secret string
	Store  store.Store
	Pusher fanout.Pusher
}

// DepsFromEnv assembles the dependency set from the process environment.
func DepsFromEnv() Deps {
	var p fanout.Pusher
	if hook := os.Getenv("NITROPING_WEBHOOK_URL"); hook != "" {
		p = fanout.NewWebhook(hook, os.Getenv("NITROPING_API_KEY"))
	}
	return Deps{Secret: os.Getenv("SYNC_SIGNING_SECRET"), Store: depStore(), Pusher: p}
}

func writeJSON(w http.ResponseWriter, code int, payload map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

// Health reports liveness plus which optional integrations are configured.
// Additive over the Phase 1 stub: "service" and "status":"ok" are unchanged,
// so existing probes keep passing.
func Health(d Deps, w http.ResponseWriter, r *http.Request) {
	tursoState, fanoutState := "missing", "disabled"
	if d.Store != nil {
		tursoState = "configured"
	}
	if d.Pusher != nil {
		fanoutState = "configured"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"service": "school-attendance-server",
		"status":  "ok",
		"signing": d.Secret != "",
		"turso":   tursoState,
		"fanout":  fanoutState,
	})
}

// syncRequest is one attendance row posted by a registered admin device.
type syncRequest struct {
	DeviceID  string `json:"device_id"`
	StudentID string `json:"student_id"`
	Status    string `json:"status"`    // "in" or "out"
	MarkedAt  string `json:"marked_at"` // RFC3339, device clock converted to UTC
}

// Sync accepts one attendance row from a signed, registered device:
// verify signature -> validate body -> check the device allowlist ->
// persist (server-side de-duplicated) -> best-effort fan-out to the parents.
func Sync(d Deps, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed, use POST"})
		return
	}
	if !requireConfig(d, w) {
		return
	}
	// Schema bootstrap (idempotent; memoized on the store singleton).
	if err := d.Store.Init(r.Context()); err != nil {
		storageDown(w, err)
		return
	}
	body := readBody(w, r)
	if body == nil {
		return
	}
	if err := authn.Verify(r, body, d.Secret, authn.DefaultMaxSkew); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
		return
	}
	var req syncRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json body"})
		return
	}
	if req.StudentID == "" || req.Status == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "student_id and status are required"})
		return
	}
	if req.Status != "in" && req.Status != "out" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": `status must be "in" or "out"`})
		return
	}
	// Required since Phase 4: the server-side de-dup key and the parent
	// fan-out payload both depend on a real timestamp.
	if _, err := time.Parse(time.RFC3339, req.MarkedAt); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "marked_at is required and must be RFC3339"})
		return
	}
	deviceID := r.Header.Get(authn.DeviceHeader)
	if req.DeviceID != "" && req.DeviceID != deviceID {
		// The signature covers the body, so a mismatch means the device
		// identity in the body was rewritten after signing - reject it.
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "device_id in body must match the X-Device-Id header"})
		return
	}

	exists, err := d.Store.DeviceExists(r.Context(), deviceID)
	if err != nil {
		storageDown(w, err)
		return
	}
	if !exists {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "unknown device; POST /api/register first"})
		return
	}

	inserted, err := d.Store.InsertRow(r.Context(), store.Row{
		DeviceID: deviceID, StudentID: req.StudentID, Status: req.Status, MarkedAt: req.MarkedAt,
	})
	if err != nil {
		storageDown(w, err)
		return
	}

	pushed := false
	if d.Pusher != nil {
		// Best-effort: the row is already durable; a relay outage must not
		// fail a mark captured at the door. The parent de-duplicates by
		// (student, status, instant), so a re-push of a duplicate row is safe.
		err := d.Pusher.Push(r.Context(), fanout.Event{
			DeviceID: deviceID, StudentID: req.StudentID, Status: req.Status, MarkedAt: req.MarkedAt,
		})
		if err != nil {
			log.Printf("sync: fan-out failed for student %s: %v", req.StudentID, err)
		} else {
			pushed = true
		}
	}

	note := "row persisted; parent fan-out best-effort"
	if !inserted {
		note = "duplicate of an already-stored row; no second record"
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"accepted":  true,
		"stored":    true,
		"duplicate": !inserted,
		"pushed":    pushed,
		"student":   req.StudentID,
		"note":      note,
	})
}

// registerRequest is a device asking to join the sync allowlist.
type registerRequest struct {
	DeviceID string `json:"device_id"`
	Label    string `json:"label"`
}

// Register adds a signed device to the sync allowlist. The body device_id
// must match the signed X-Device-Id header, so a valid signature for one
// device cannot register another.
func Register(d Deps, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed, use POST"})
		return
	}
	if !requireConfig(d, w) {
		return
	}
	// Schema bootstrap (idempotent; memoized on the store singleton).
	if err := d.Store.Init(r.Context()); err != nil {
		storageDown(w, err)
		return
	}
	body := readBody(w, r)
	if body == nil {
		return
	}
	if err := authn.Verify(r, body, d.Secret, authn.DefaultMaxSkew); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
		return
	}
	var req registerRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json body"})
		return
	}
	deviceID := r.Header.Get(authn.DeviceHeader)
	if req.DeviceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "device_id is required"})
		return
	}
	if req.DeviceID != deviceID {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "device_id in body must match the X-Device-Id header"})
		return
	}
	if err := d.Store.UpsertDevice(r.Context(), store.Device{DeviceID: deviceID, Label: req.Label}); err != nil {
		storageDown(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"registered": true, "device_id": deviceID})
}

// requireConfig fails the request with 503 + the exact missing variables when
// the signing secret or the persistence layer is not configured. The admin
// app treats any non-2xx as retryable, so rows keep queueing offline.
func requireConfig(d Deps, w http.ResponseWriter) bool {
	missing := []string{}
	if d.Secret == "" {
		missing = append(missing, "SYNC_SIGNING_SECRET")
	}
	if d.Store == nil {
		missing = append(missing, "TURSO_DATABASE_URL", "TURSO_AUTH_TOKEN")
	}
	if len(missing) == 0 {
		return true
	}
	writeJSON(w, http.StatusServiceUnavailable, map[string]any{
		"error":   "server persistence/signing not configured; attendance keeps queueing on the device",
		"missing": missing,
	})
	return false
}

// storageDown reports a Turso failure as 502: the request itself was fine,
// the upstream storage is not, and the admin app will retry later.
func storageDown(w http.ResponseWriter, err error) {
	log.Printf("storage failure: %v", err)
	writeJSON(w, http.StatusBadGateway, map[string]any{"error": "storage unavailable; the admin app will retry"})
}

// readBody reads the request body under the size cap; nil on failure with
// the right error already written.
func readBody(w http.ResponseWriter, r *http.Request) []byte {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error": "request body too large"})
		} else {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "could not read request body"})
		}
		return nil
	}
	return body
}
