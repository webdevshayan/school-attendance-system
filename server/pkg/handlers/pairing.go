// Pairing endpoints: mint a code (door admin, HMAC-signed), redeem a code
// (parent install — the code itself is the credential), and the events feed
// the paired install polls (Bearer token; the same payload shape PushHandler
// on the parent app already parses).
package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/webdevshayan/school-attendance-system/server/pkg/authn"
	"github.com/webdevshayan/school-attendance-system/server/pkg/store"
)

const (
	// codeAlphabet avoids 0/O/1/I lookalikes so codes can be read aloud.
	codeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	codeLength   = 8
	// codeTTL is how long a minted code can redeem devices. Multi-use by
	// design: both guardians pair from the school's one code.
	codeTTL = 7 * 24 * time.Hour
	// eventFeedLimit bounds one poll response.
	eventFeedLimit = 100
)

// randomPairingCode mints one short code from crypto/rand.
func randomPairingCode() (string, error) {
	buf := make([]byte, codeLength)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, codeLength)
	for i, b := range buf {
		out[i] = codeAlphabet[int(b)%len(codeAlphabet)]
	}
	return string(out), nil
}

// randomToken mints a 64-hex-char bearer token.
func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// tokenHash is what the server stores instead of the raw token.
func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// requireStore fails the request with 503 when persistence is unconfigured
// (used by the parent-facing endpoints, which need no admin HMAC secret).
func requireStore(d Deps, w http.ResponseWriter) bool {
	if d.Store != nil {
		return true
	}
	writeJSON(w, http.StatusServiceUnavailable, map[string]any{
		"error":   "server persistence not configured; attendance keeps queueing on the device",
		"missing": []string{"TURSO_DATABASE_URL", "TURSO_AUTH_TOKEN"},
	})
	return false
}

// mintCodeRequest is one pairing-code request from a signed door device.
type mintCodeRequest struct {
	StudentID string `json:"student_id"`
}

// MintCode creates a pairing code for a student. Door-device authenticated
// (same HMAC scheme as /api/sync): only provisioned devices hand out codes.
func MintCode(d Deps, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed, use POST"})
		return
	}
	if !requireConfig(d, w) {
		return
	}
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
	// Only allowlisted door devices hand out codes.
	deviceID := r.Header.Get(authn.DeviceHeader)
	exists, err := d.Store.DeviceExists(r.Context(), deviceID)
	if err != nil {
		storageDown(w, err)
		return
	}
	if !exists {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "unknown device; POST /api/register first"})
		return
	}
	var req mintCodeRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json body"})
		return
	}
	if req.StudentID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "student_id is required"})
		return
	}
	code, err := randomPairingCode()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not generate code"})
		return
	}
	now := time.Now().UTC()
	pc := store.PairingCode{
		Code:      code,
		StudentID: req.StudentID,
		CreatedAt: now.Format(time.RFC3339),
		ExpiresAt: now.Add(codeTTL).Format(time.RFC3339),
	}
	if err := d.Store.MintCode(r.Context(), pc); err != nil {
		storageDown(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"code":       code,
		"student_id": req.StudentID,
		"expires_at": pc.ExpiresAt,
		"note":       "give this code to the guardians; it stays valid for both parents until it expires",
	})
}

// pairRequest is one pairing redemption from a parent install.
type pairRequest struct {
	Code     string `json:"code"`
	Label    string `json:"label"`    // human-readable device label
	Platform string `json:"platform"` // android/ios
}

// Pair redeems a pairing code and registers the parent install. The bearer
// token is returned exactly once; only its SHA-256 is stored server-side.
func Pair(d Deps, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed, use POST"})
		return
	}
	if !requireStore(d, w) {
		return
	}
	if err := d.Store.Init(r.Context()); err != nil {
		storageDown(w, err)
		return
	}
	body := readBody(w, r)
	if body == nil {
		return
	}
	var req pairRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json body"})
		return
	}
	if req.Code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "code is required"})
		return
	}
	pc, ok, err := d.Store.FindCode(r.Context(), req.Code)
	if err != nil {
		storageDown(w, err)
		return
	}
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unknown code"})
		return
	}
	expiresAt, err := time.Parse(time.RFC3339, pc.ExpiresAt)
	if err != nil || time.Now().UTC().After(expiresAt) {
		writeJSON(w, http.StatusGone, map[string]any{"error": "code expired; ask the school for a new one"})
		return
	}
	token, err := randomToken()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not generate token"})
		return
	}
	pd := store.ParentDevice{
		TokenHash: tokenHash(token),
		StudentID: pc.StudentID,
		Label:     req.Label,
		Platform:  req.Platform,
		PairedAt:  time.Now().UTC().Format(time.RFC3339),
	}
	if err := d.Store.UpsertParentDevice(r.Context(), pd); err != nil {
		storageDown(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"student_id": pc.StudentID,
		"token":      token,
		"note":       "store the token on the device; it cannot be recovered later",
	})
}

// Events is the paired install's polling feed: bearer-authenticated, cursor
// based, oldest first. Payloads reuse the /api/sync wire shape so the parent
// app's PushHandler consumes them unchanged.
func Events(d Deps, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed, use GET"})
		return
	}
	if !requireStore(d, w) {
		return
	}
	token, ok := bearerToken(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "missing bearer token"})
		return
	}
	studentID, err := d.Store.StudentForToken(r.Context(), tokenHash(token))
	if err != nil {
		storageDown(w, err)
		return
	}
	if studentID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid or revoked token"})
		return
	}
	since := int64(0)
	if raw := r.URL.Query().Get("since"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "since must be a non-negative integer"})
			return
		}
		since = parsed
	}
	events, err := d.Store.EventsSince(r.Context(), studentID, since, eventFeedLimit)
	if err != nil {
		storageDown(w, err)
		return
	}
	payload := make([]map[string]any, 0, len(events))
	cursor := since
	for _, e := range events {
		payload = append(payload, map[string]any{
			"id":          e.ID,
			"device_id":   e.DeviceID,
			"student_id":  e.StudentID,
			"status":      e.Status,
			"marked_at":   e.MarkedAt,
			"received_at": e.ReceivedAt,
		})
		cursor = e.ID
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": payload, "cursor": cursor})
}

// bearerToken extracts "Authorization: Bearer <token>".
func bearerToken(r *http.Request) (string, bool) {
	const prefix = "Bearer "
	raw := r.Header.Get("Authorization")
	if len(raw) <= len(prefix) || raw[:len(prefix)] != prefix {
		return "", false
	}
	return raw[len(prefix):], true
}
