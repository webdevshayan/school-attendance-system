// Package authn verifies signed requests from admin devices.
//
// A device proves possession of the shared signing secret with three headers:
//
//	X-Device-Id:      the device identity (e.g. door-1)
//	X-Sync-Timestamp: RFC3339 UTC instant the request was signed
//	X-Sync-Signature: hex HMAC-SHA256(deviceId \n timestamp \n body)
//
// The timestamp bounds replays: anything that drifts more than MaxSkew from
// server time is rejected even with a valid signature.
package authn

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Canonical header names used by both /api/sync and /api/register.
const (
	DeviceHeader    = "X-Device-Id"
	TimestampHeader = "X-Sync-Timestamp"
	SignatureHeader = "X-Sync-Signature"
)

// DefaultMaxSkew bounds client/server clock drift for the replay window.
const DefaultMaxSkew = 5 * time.Minute

// Sentinel errors the caller maps to HTTP statuses.
var (
	ErrMissingHeaders = errors.New("missing signing headers")
	ErrBadTimestamp   = errors.New("x-sync-timestamp is not a valid RFC3339 instant")
	ErrStaleTimestamp = errors.New("x-sync-timestamp outside the replay window")
	ErrBadSignature   = errors.New("signature mismatch")
)

// Sign computes the signature the admin app sends. Exported so tests (and a
// future provisioning CLI) can build valid requests.
func Sign(deviceID, timestamp, secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%s\n%s\n", deviceID, timestamp)
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// Verify checks the request's signing headers against secret for body.
// It returns nil only for a fresh, validly-signed request.
func Verify(r *http.Request, body []byte, secret string, maxSkew time.Duration) error {
	return VerifyAt(r, body, secret, time.Now().UTC(), maxSkew)
}

// VerifyAt is Verify with an injected clock (test seam).
func VerifyAt(r *http.Request, body []byte, secret string, now time.Time, maxSkew time.Duration) error {
	deviceID := r.Header.Get(DeviceHeader)
	ts := r.Header.Get(TimestampHeader)
	sig := r.Header.Get(SignatureHeader)
	if deviceID == "" || ts == "" || sig == "" {
		return ErrMissingHeaders
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ErrBadTimestamp
	}
	if t.Sub(now) > maxSkew || now.Sub(t) > maxSkew {
		return ErrStaleTimestamp
	}
	want, err2 := hex.DecodeString(Sign(deviceID, ts, secret, body))
	if err2 != nil {
		return ErrBadSignature
	}
	got, err3 := hex.DecodeString(sig)
	if err3 != nil {
		return ErrBadSignature
	}
	// Constant-time comparison: signature checks must not leak timing.
	if !hmac.Equal(want, got) {
		return ErrBadSignature
	}
	return nil
}
