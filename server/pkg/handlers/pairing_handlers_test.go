package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/webdevshayan/school-attendance-system/server/pkg/store"
)

// pairRequest posts an unsigned pairing redemption (the code is the credential).
func pairReq(t *testing.T, d Deps, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/api/pair", strings.NewReader(body))
	w := httptest.NewRecorder()
	Pair(d, w, r)
	return w
}

func eventsReq(t *testing.T, d Deps, token, since string) *httptest.ResponseRecorder {
	t.Helper()
	url := "/api/events"
	if since != "" {
		url += "?since=" + since
	}
	r := httptest.NewRequest(http.MethodGet, url, nil)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	Events(d, w, r)
	return w
}

func mintCodeReq(t *testing.T, d Deps, deviceID, body, secret string) *httptest.ResponseRecorder {
	t.Helper()
	w, r := signed(t, "/api/codes", deviceID, body, secret)
	MintCode(d, w, r)
	return w
}

func TestMintCodeIssuesTypedCodeToRegisteredDevice(t *testing.T) {
	st := newFakeStore()
	d := fullDeps(st)
	if err := st.UpsertDevice(context.Background(), store.Device{DeviceID: "door-1"}); err != nil {
		t.Fatal(err)
	}

	w := mintCodeReq(t, d, "door-1", `{"student_id":"s1"}`, testSecret)
	if w.Code != http.StatusOK {
		t.Fatalf("mint = %d (%s)", w.Code, w.Body.String())
	}
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	code, _ := m["code"].(string)
	if len(code) != 8 {
		t.Fatalf("code = %q, want 8 chars", code)
	}
	for _, ch := range code {
		if !strings.ContainsRune(codeAlphabet, ch) {
			t.Errorf("code contains lookalike/invalid char %q", ch)
		}
	}
	if m["student_id"] != "s1" {
		t.Errorf("student_id = %v", m["student_id"])
	}
	// Expires about 7 days out.
	expiresAt, err := time.Parse(time.RFC3339, m["expires_at"].(string))
	if err != nil {
		t.Fatalf("expires_at not RFC3339: %v", err)
	}
	if delta := time.Until(expiresAt); delta < 6*24*time.Hour || delta > 8*24*time.Hour {
		t.Errorf("TTL = %v, want ~7 days", delta)
	}
	if len(st.codes) != 1 {
		t.Errorf("code not persisted: %v", st.codes)
	}
}

func TestMintCodeRequiresSignedRegisteredDevice(t *testing.T) {
	st := newFakeStore()
	d := fullDeps(st)

	// Unregistered device, valid signature.
	w := mintCodeReq(t, d, "door-x", `{"student_id":"s1"}`, testSecret)
	if w.Code != http.StatusForbidden {
		t.Fatalf("unregistered = %d, want 403", w.Code)
	}

	// Registered but unsigned.
	if err := st.UpsertDevice(context.Background(), store.Device{DeviceID: "door-1"}); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/codes", strings.NewReader(`{"student_id":"s1"}`))
	w2 := httptest.NewRecorder()
	MintCode(d, w2, r)
	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned = %d, want 401", w2.Code)
	}

	// Empty student_id.
	w3 := mintCodeReq(t, d, "door-1", `{"student_id":""}`, testSecret)
	if w3.Code != http.StatusBadRequest {
		t.Fatalf("empty student = %d, want 400", w3.Code)
	}

	// Missing store.
	w4 := httptest.NewRecorder()
	MintCode(Deps{Secret: testSecret}, w4, httptest.NewRequest(http.MethodPost, "/api/codes", strings.NewReader(`{"student_id":"s1"}`)))
	if w4.Code != http.StatusServiceUnavailable {
		t.Fatalf("no store = %d, want 503", w4.Code)
	}
}

func TestPairRedeemsCodeAndStoresOnlyTokenHash(t *testing.T) {
	st := newFakeStore()
	d := fullDeps(st)
	future := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)
	if err := st.MintCode(context.Background(), store.PairingCode{
		Code: "ABCD2345", StudentID: "s1", ExpiresAt: future,
	}); err != nil {
		t.Fatal(err)
	}

	w := pairReq(t, d, `{"code":"ABCD2345","label":"moms phone","platform":"android"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("pair = %d (%s)", w.Code, w.Body.String())
	}
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	token, _ := m["token"].(string)
	if len(token) != 64 {
		t.Fatalf("token = %q, want 64 hex chars", token)
	}
	if m["student_id"] != "s1" {
		t.Errorf("student_id = %v", m["student_id"])
	}
	// The token must be stored hashed, not verbatim.
	found := false
	for hash := range st.parents {
		if hash == token {
			t.Errorf("raw token stored server-side!")
		}
		found = true
	}
	if !found {
		t.Errorf("no parent device stored")
	}

	// The token from the pair response must authenticate the events feed.
	w2 := eventsReq(t, d, token, "")
	if w2.Code != http.StatusOK {
		t.Fatalf("events with paired token = %d (%s)", w2.Code, w2.Body.String())
	}
}

func TestPairRejectsUnknownExpiredAndEmptyCodes(t *testing.T) {
	st := newFakeStore()
	d := fullDeps(st)

	if w := pairReq(t, d, `{"code":"ZZZZ9999"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("unknown code = %d, want 400", w.Code)
	}

	past := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	if err := st.MintCode(context.Background(), store.PairingCode{
		Code: "OLD1CODE", StudentID: "s1", ExpiresAt: past,
	}); err != nil {
		t.Fatal(err)
	}
	if w := pairReq(t, d, `{"code":"OLD1CODE"}`); w.Code != http.StatusGone {
		t.Fatalf("expired code = %d, want 410", w.Code)
	}

	if w := pairReq(t, d, `{"code":""}`); w.Code != http.StatusBadRequest {
		t.Fatalf("empty code = %d, want 400", w.Code)
	}
}

func TestEventsFeedReturnsRowsAndHonorsCursor(t *testing.T) {
	st := newFakeStore()
	d := fullDeps(st)
	if err := st.UpsertParentDevice(context.Background(), store.ParentDevice{
		TokenHash: tokenHash("tok-1"), StudentID: "s1",
	}); err != nil {
		t.Fatal(err)
	}
	// Two rows for s1 and one for another student (must not leak).
	_, _ = st.InsertRow(context.Background(), store.Row{DeviceID: "door-1", StudentID: "s1", Status: "in", MarkedAt: "2026-11-20T08:00:00Z"})
	_, _ = st.InsertRow(context.Background(), store.Row{DeviceID: "door-1", StudentID: "s2", Status: "in", MarkedAt: "2026-11-20T08:05:00Z"})
	_, _ = st.InsertRow(context.Background(), store.Row{DeviceID: "door-1", StudentID: "s1", Status: "out", MarkedAt: "2026-11-20T15:00:00Z"})

	// First poll: everything for s1, cursor = id of the last delivered row.
	w := eventsReq(t, d, "tok-1", "")
	if w.Code != http.StatusOK {
		t.Fatalf("events = %d (%s)", w.Code, w.Body.String())
	}
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	events := m["events"].([]any)
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2 (other student must not leak)", len(events))
	}
	first := events[0].(map[string]any)
	if first["status"] != "in" || first["device_id"] != "door-1" {
		t.Errorf("event0 = %v", first)
	}
	cursor, _ := m["cursor"].(float64)
	if cursor != 3 {
		t.Errorf("cursor = %v, want 3", cursor)
	}

	// Second poll from the cursor: nothing new, cursor unchanged.
	w2 := eventsReq(t, d, "tok-1", "3")
	var m2 map[string]any
	if err := json.Unmarshal(w2.Body.Bytes(), &m2); err != nil {
		t.Fatal(err)
	}
	if got := len(m2["events"].([]any)); got != 0 {
		t.Errorf("events after cursor = %d, want 0", got)
	}
	if m2["cursor"].(float64) != 3 {
		t.Errorf("cursor should stay 3, got %v", m2["cursor"])
	}
}

func TestEventsFeedRejectsBadTokensAndMethods(t *testing.T) {
	st := newFakeStore()
	d := fullDeps(st)
	if err := st.UpsertParentDevice(context.Background(), store.ParentDevice{
		TokenHash: tokenHash("tok-1"), StudentID: "s1",
	}); err != nil {
		t.Fatal(err)
	}

	if w := eventsReq(t, d, "not-a-token", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("bad token = %d, want 401", w.Code)
	}
	r := httptest.NewRequest(http.MethodGet, "/api/events", nil)
	w := httptest.NewRecorder()
	Events(d, w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("missing token = %d, want 401", w.Code)
	}
	r2 := httptest.NewRequest(http.MethodPost, "/api/events", nil)
	w2 := httptest.NewRecorder()
	Events(d, w2, r2)
	if w2.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST events = %d, want 405", w2.Code)
	}
	// With token auth passing, a malformed cursor must reach validation.
	w3 := eventsReq(t, d, "tok-1", "banana")
	if w3.Code != http.StatusBadRequest {
		t.Fatalf("bad since = %d, want 400", w3.Code)
	}
}

func TestPairAndEventsRequireStore(t *testing.T) {
	d := Deps{Secret: testSecret} // no store
	if w := pairReq(t, d, `{"code":"ABCD2345"}`); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("pair without store = %d, want 503", w.Code)
	}
	if w := eventsReq(t, d, "tok", ""); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("events without store = %d, want 503", w.Code)
	}
}

// revokeCodeReq posts a signed code-invalidation, exactly the way the admin
// app will send it.
func revokeCodeReq(t *testing.T, d Deps, deviceID, body, secret string) *httptest.ResponseRecorder {
	t.Helper()
	w, r := signed(t, "/api/codes/revoke", deviceID, body, secret)
	RevokeCode(d, w, r)
	return w
}

func mintOneCode(t *testing.T, d Deps) string {
	t.Helper()
	w := mintCodeReq(t, d, "door-1", `{"student_id":"s1"}`, testSecret)
	if w.Code != http.StatusOK {
		t.Fatalf("mint = %d (%s)", w.Code, w.Body.String())
	}
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	code, _ := m["code"].(string)
	if code == "" {
		t.Fatal("mint returned no code")
	}
	return code
}

func TestRevokeCodeKillsTheInvitation(t *testing.T) {
	st := newFakeStore()
	d := fullDeps(st)
	if err := st.UpsertDevice(context.Background(), store.Device{DeviceID: "door-1"}); err != nil {
		t.Fatal(err)
	}
	code := mintOneCode(t, d)

	// Precondition: the code pairs right now.
	if w := pairReq(t, d, fmt.Sprintf(`{"code":%q,"label":"moms phone","platform":"android"}`, code)); w.Code != http.StatusOK {
		t.Fatalf("pair before revoke = %d (%s)", w.Code, w.Body.String())
	}
	// Mint again (the first redemption consumed nothing - codes are
	// multi-use) and revoke it.
	code = mintOneCode(t, d)
	if w := revokeCodeReq(t, d, "door-1", fmt.Sprintf(`{"code":%q}`, code), testSecret); w.Code != http.StatusOK {
		t.Fatalf("revoke = %d (%s)", w.Code, w.Body.String())
	}
	// Only the revoked code is gone; the first mint's invitation is
	// untouched (revoke kills one invitation, not the student's others).
	if len(st.codes) != 1 {
		t.Errorf("codes after revoke = %d, want 1 (the earlier mint survives)", len(st.codes))
	}
	if w := pairReq(t, d, fmt.Sprintf(`{"code":%q}`, code)); w.Code != http.StatusBadRequest {
		t.Fatalf("pair after revoke = %d, want 400 unknown code", w.Code)
	}
}

func TestRevokeCodeIsIdempotent(t *testing.T) {
	st := newFakeStore()
	d := fullDeps(st)
	if err := st.UpsertDevice(context.Background(), store.Device{DeviceID: "door-1"}); err != nil {
		t.Fatal(err)
	}
	code := mintOneCode(t, d)
	for i := 0; i < 2; i++ {
		if w := revokeCodeReq(t, d, "door-1", fmt.Sprintf(`{"code":%q}`, code), testSecret); w.Code != http.StatusOK {
			t.Fatalf("revoke #%d = %d, want 200 (idempotent)", i+1, w.Code)
		}
	}
	// Revoking something that never existed is equally fine.
	if w := revokeCodeReq(t, d, "door-1", `{"code":"ZZZZ9999"}`, testSecret); w.Code != http.StatusOK {
		t.Fatalf("revoke unknown = %d, want 200", w.Code)
	}
}

func TestRevokeCodeRequiresSignedRegisteredDevice(t *testing.T) {
	st := newFakeStore()
	d := fullDeps(st)
	if err := st.UpsertDevice(context.Background(), store.Device{DeviceID: "door-1"}); err != nil {
		t.Fatal(err)
	}

	// No signature at all.
	if w := revokeCodeReq(t, d, "door-1", `{"code":"ABCD2345"}`, ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned revoke = %d, want 401", w.Code)
	}
	// Signed with the wrong secret.
	if w := revokeCodeReq(t, d, "door-1", `{"code":"ABCD2345"}`, "wrong-secret"); w.Code != http.StatusUnauthorized {
		t.Fatalf("bad-signature revoke = %d, want 401", w.Code)
	}
	// Valid signature, unregistered device.
	if w := revokeCodeReq(t, d, "stranger", `{"code":"ABCD2345"}`, testSecret); w.Code != http.StatusForbidden {
		t.Fatalf("stranger revoke = %d, want 403", w.Code)
	}
	// Valid signature, wrong method.
	w, r := signed(t, "/api/codes/revoke", "door-1", `{"code":"ABCD2345"}`, testSecret)
	r.Method = http.MethodGet
	RevokeCode(d, w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET revoke = %d, want 405", w.Code)
	}
}

func TestRevokeCodeRejectsEmptyAndInvalidBodies(t *testing.T) {
	st := newFakeStore()
	d := fullDeps(st)
	if err := st.UpsertDevice(context.Background(), store.Device{DeviceID: "door-1"}); err != nil {
		t.Fatal(err)
	}
	if w := revokeCodeReq(t, d, "door-1", `{"code":""}`, testSecret); w.Code != http.StatusBadRequest {
		t.Fatalf("empty code = %d, want 400", w.Code)
	}
	if w := revokeCodeReq(t, d, "door-1", `{"student_id":"s1"}`, testSecret); w.Code != http.StatusBadRequest {
		t.Fatalf("missing code field = %d, want 400", w.Code)
	}
}
