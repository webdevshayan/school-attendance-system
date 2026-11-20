package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

// maxBodyBytes caps the request body so a misbehaving device can't blow up
// the handler. Real attendance rows are a few hundred bytes; 1 MB is generous.
const maxBodyBytes = 1 << 20

// syncRequest is one attendance row posted by the admin app once
// connectivity returns. Phase 4 persists these to Turso and fans out
// Nitroping pushes to the child's parent app.
type syncRequest struct {
	DeviceID  string `json:"device_id"`
	StudentID string `json:"student_id"`
	Status    string `json:"status"`    // "in" or "out"
	MarkedAt  string `json:"marked_at"` // RFC3339, device-local clock
}

// writeJSON sends a JSON response with the right content type.
func writeJSON(w http.ResponseWriter, code int, payload map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

// Handler accepts one attendance row from the admin app.
// Served at POST /api/sync by Vercel's zero-config Go runtime.
//
// Phase 1 validates the shape and acknowledges; Turso persistence and
// push fan-out land in Phase 4 (tracked in the roadmap).
func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed, use POST"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	var req syncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error": "request body too large"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json body"})
		return
	}
	if req.StudentID == "" || req.Status == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "student_id and status are required"})
		return
	}
	if req.Status != "in" && req.Status != "out" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "status must be \"in\" or \"out\""})
		return
	}
	if req.MarkedAt != "" {
		if _, err := time.Parse(time.RFC3339, req.MarkedAt); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "marked_at must be RFC3339"})
			return
		}
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"accepted": true,
		"student":  req.StudentID,
		"note":     "phase 1 stub: turso persistence and push fan-out land in phase 4",
	})
}
