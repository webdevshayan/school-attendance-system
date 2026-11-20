package main

import (
	"encoding/json"
	"net/http"
)

// syncRequest is one attendance row posted by the admin app once
// connectivity returns. Phase 4 persists these to Turso and fans out
// Nitroping pushes to the child's parent app.
type syncRequest struct {
	DeviceID  string `json:"device_id"`
	StudentID string `json:"student_id"`
	Status    string `json:"status"`    // "in" or "out"
	MarkedAt  string `json:"marked_at"` // RFC3339, device-local clock
}

// Handler accepts one attendance row from the admin app.
// Served at POST /api/sync by Vercel's zero-config Go runtime.
//
// Phase 1 validates the shape and acknowledges; Turso persistence and
// push fan-out land in Phase 4 (tracked in the roadmap).
func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed, use POST"}`, http.StatusMethodNotAllowed)
		return
	}

	var req syncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json body"}`, http.StatusBadRequest)
		return
	}
	if req.StudentID == "" || req.Status == "" {
		http.Error(w, `{"error":"student_id and status are required"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"accepted": true,
		"student":  req.StudentID,
		"note":     "phase 1 stub: turso persistence and push fan-out land in phase 4",
	})
}
