package handler

import (
	"encoding/json"
	"net/http"
)

// Handler reports liveness for monitoring and the admin app's sync probe.
// Served at GET /api/health by Vercel's zero-config Go runtime.
func Handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"service": "school-attendance-server",
	})
}
