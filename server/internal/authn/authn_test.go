package authn

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const (
	testSecret   = "test-secret"
	testDeviceID = "door-1"
	testBody     = `{"student_id":"s1","status":"in"}`
)

// signedRequest builds a request carrying headers produced by Sign, exactly
// the way the admin app sends them.
func signedRequest(t *testing.T, deviceID, ts, secret string, body []byte) *http.Request {
	t.Helper()
	r := httptest.NewRequest("POST", "/api/sync", bytes.NewReader(body))
	if deviceID != "" {
		r.Header.Set(DeviceHeader, deviceID)
	}
	if ts != "" {
		r.Header.Set(TimestampHeader, ts)
		if sig := Sign(deviceID, ts, secret, body); sig != "" {
			r.Header.Set(SignatureHeader, sig)
		}
	}
	return r
}

func TestVerifyAcceptsFreshValidSignature(t *testing.T) {
	now := time.Date(2026, 11, 20, 13, 0, 0, 0, time.UTC)
	ts := now.Format(time.RFC3339)
	r := signedRequest(t, testDeviceID, ts, testSecret, []byte(testBody))

	if err := VerifyAt(r, []byte(testBody), testSecret, now, DefaultMaxSkew); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
}

func TestVerifyRejectsMissingHeaders(t *testing.T) {
	now := time.Date(2026, 11, 20, 13, 0, 0, 0, time.UTC)
	ts := now.Format(time.RFC3339)

	for _, tc := range []struct{ dropDevice, dropTS, dropSig bool }{
		{true, false, false},
		{false, true, false},
		{false, false, true},
	} {
		r := signedRequest(t, testDeviceID, ts, testSecret, []byte(testBody))
		if tc.dropDevice {
			r.Header.Del(DeviceHeader)
		}
		if tc.dropTS {
			r.Header.Del(TimestampHeader)
		}
		if tc.dropSig {
			r.Header.Del(SignatureHeader)
		}
		if err := VerifyAt(r, []byte(testBody), testSecret, now, DefaultMaxSkew); err != ErrMissingHeaders {
			t.Errorf("dropped headers (dev=%t ts=%t sig=%t): got %v, want ErrMissingHeaders", tc.dropDevice, tc.dropTS, tc.dropSig, err)
		}
	}
}

func TestVerifyRejectsMalformedTimestamp(t *testing.T) {
	now := time.Date(2026, 11, 20, 13, 0, 0, 0, time.UTC)
	r := signedRequest(t, testDeviceID, "yesterday-ish", testSecret, []byte(testBody))
	if err := VerifyAt(r, []byte(testBody), testSecret, now, DefaultMaxSkew); err != ErrBadTimestamp {
		t.Errorf("got %v, want ErrBadTimestamp", err)
	}
}

func TestVerifyRejectsStaleAndFutureTimestamps(t *testing.T) {
	now := time.Date(2026, 11, 20, 13, 0, 0, 0, time.UTC)
	for _, offset := range []time.Duration{-6 * time.Minute, 6 * time.Minute} {
		ts := now.Add(offset).Format(time.RFC3339)
		r := signedRequest(t, testDeviceID, ts, testSecret, []byte(testBody))
		if err := VerifyAt(r, []byte(testBody), testSecret, now, DefaultMaxSkew); err != ErrStaleTimestamp {
			t.Errorf("offset %v: got %v, want ErrStaleTimestamp", offset, err)
		}
	}
	// Inside the window must still pass.
	ts := now.Add(-4 * time.Minute).Format(time.RFC3339)
	r := signedRequest(t, testDeviceID, ts, testSecret, []byte(testBody))
	if err := VerifyAt(r, []byte(testBody), testSecret, now, DefaultMaxSkew); err != nil {
		t.Errorf("in-window timestamp rejected: %v", err)
	}
}

func TestVerifyRejectsTamperedBodyAndWrongSecret(t *testing.T) {
	now := time.Date(2026, 11, 20, 13, 0, 0, 0, time.UTC)
	ts := now.Format(time.RFC3339)

	r := signedRequest(t, testDeviceID, ts, testSecret, []byte(testBody))
	if err := VerifyAt(r, []byte(`{"student_id":"s1","status":"out"}`), testSecret, now, DefaultMaxSkew); err != ErrBadSignature {
		t.Errorf("tampered body: got %v, want ErrBadSignature", err)
	}

	r = signedRequest(t, testDeviceID, ts, "other-secret", []byte(testBody))
	if err := VerifyAt(r, []byte(testBody), testSecret, now, DefaultMaxSkew); err != ErrBadSignature {
		t.Errorf("wrong secret: got %v, want ErrBadSignature", err)
	}

	r = signedRequest(t, testDeviceID, ts, testSecret, []byte(testBody))
	r.Header.Set(SignatureHeader, "not-hex-at-all")
	if err := VerifyAt(r, []byte(testBody), testSecret, now, DefaultMaxSkew); err != ErrBadSignature {
		t.Errorf("non-hex signature: got %v, want ErrBadSignature", err)
	}
}

// A signature computed for one device must not validate for another.
func TestVerifyBindsSignatureToDevice(t *testing.T) {
	now := time.Date(2026, 11, 20, 13, 0, 0, 0, time.UTC)
	ts := now.Format(time.RFC3339)
	r := signedRequest(t, testDeviceID, ts, testSecret, []byte(testBody))
	r.Header.Set(DeviceHeader, "door-2") // signed as door-1, sent as door-2
	if err := VerifyAt(r, []byte(testBody), testSecret, now, DefaultMaxSkew); err != ErrBadSignature {
		t.Errorf("got %v, want ErrBadSignature", err)
	}
}
