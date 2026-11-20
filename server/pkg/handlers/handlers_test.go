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

	"github.com/webdevshayan/school-attendance-system/server/pkg/authn"
	"github.com/webdevshayan/school-attendance-system/server/pkg/fanout"
	"github.com/webdevshayan/school-attendance-system/server/pkg/store"
)

// fakeStore implements store.Store in memory; InsertRow de-duplicates on the
// same key the real store enforces.
type fakeStore struct {
	devices   map[string]string
	rows      []store.Row
	seen      map[string]bool
	codes     map[string]store.PairingCode
	parents   map[string]string // tokenHash -> studentID
	existsErr error
	insertErr error
	initErr   error
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		devices: map[string]string{},
		seen:    map[string]bool{},
		codes:   map[string]store.PairingCode{},
		parents: map[string]string{},
	}
}

func (f *fakeStore) Init(_ context.Context) error { return f.initErr }

func (f *fakeStore) InsertRow(_ context.Context, r store.Row) (bool, error) {
	if f.insertErr != nil {
		return false, f.insertErr
	}
	key := fmt.Sprintf("%s|%s|%s|%s", r.DeviceID, r.StudentID, r.Status, r.MarkedAt)
	if f.seen[key] {
		return false, nil
	}
	f.seen[key] = true
	f.rows = append(f.rows, r)
	return true, nil
}

func (f *fakeStore) UpsertDevice(_ context.Context, d store.Device) error {
	f.devices[d.DeviceID] = d.Label
	return nil
}

func (f *fakeStore) DeviceExists(_ context.Context, deviceID string) (bool, error) {
	if f.existsErr != nil {
		return false, f.existsErr
	}
	_, ok := f.devices[deviceID]
	return ok, nil
}

func (f *fakeStore) MintCode(_ context.Context, c store.PairingCode) error {
	f.codes[c.Code] = c
	return nil
}

func (f *fakeStore) FindCode(_ context.Context, code string) (store.PairingCode, bool, error) {
	c, ok := f.codes[code]
	return c, ok, nil
}

func (f *fakeStore) UpsertParentDevice(_ context.Context, d store.ParentDevice) error {
	f.parents[d.TokenHash] = d.StudentID
	return nil
}

func (f *fakeStore) StudentForToken(_ context.Context, tokenHash string) (string, error) {
	return f.parents[tokenHash], nil
}

func (f *fakeStore) EventsSince(_ context.Context, studentID string, sinceID int64, limit int) ([]store.EventRecord, error) {
	events := make([]store.EventRecord, 0, limit)
	for i, r := range f.rows {
		id := int64(i + 1)
		if r.StudentID == studentID && id > sinceID && len(events) < limit {
			events = append(events, store.EventRecord{
				ID: id, DeviceID: r.DeviceID, StudentID: r.StudentID,
				Status: r.Status, MarkedAt: r.MarkedAt,
			})
		}
	}
	return events, nil
}

type fakePusher struct {
	events []fanout.Event
	err    error
}

func (f *fakePusher) Push(_ context.Context, e fanout.Event) error {
	if f.err != nil {
		return f.err
	}
	f.events = append(f.events, e)
	return nil
}

const testSecret = "phase4-test-secret"

// fullDeps returns a configured Deps wired to fakes.
func fullDeps(st *fakeStore, p fanout.Pusher) Deps {
	return Deps{Secret: testSecret, Store: st, Pusher: p}
}

// signed builds a POST with the three signing headers, exactly the way the
// admin app sends them.
func signed(t *testing.T, path, deviceID, body, secret string) (*httptest.ResponseRecorder, *http.Request) {
	t.Helper()
	ts := time.Now().UTC().Format(time.RFC3339)
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	if deviceID != "" {
		r.Header.Set(authn.DeviceHeader, deviceID)
	}
	r.Header.Set(authn.TimestampHeader, ts)
	r.Header.Set(authn.SignatureHeader, authn.Sign(deviceID, ts, secret, []byte(body)))
	w := httptest.NewRecorder()
	return w, r
}

func bodyMap(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("response body not JSON: %v (%s)", err, w.Body.String())
	}
	return m
}

func TestRegisterThenSyncPersistsAndPushes(t *testing.T) {
	st := newFakeStore()
	push := &fakePusher{}
	d := fullDeps(st, push)

	w, r := signed(t, "/api/register", "door-1", `{"device_id":"door-1","label":"front door"}`, testSecret)
	Register(d, w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("register = %d (%s)", w.Code, w.Body.String())
	}

	w, r = signed(t, "/api/sync", "door-1",
		`{"device_id":"door-1","student_id":"s1","status":"in","marked_at":"2026-11-20T08:15:00Z"}`, testSecret)
	Sync(d, w, r)
	if w.Code != http.StatusAccepted {
		t.Fatalf("sync = %d (%s)", w.Code, w.Body.String())
	}
	m := bodyMap(t, w)
	if m["stored"] != true || m["duplicate"] != false || m["pushed"] != true {
		t.Errorf("sync payload = %v", m)
	}
	if len(st.rows) != 1 || st.rows[0].StudentID != "s1" || st.rows[0].DeviceID != "door-1" {
		t.Errorf("stored rows = %+v", st.rows)
	}
	if len(push.events) != 1 || push.events[0].StudentID != "s1" || push.events[0].Status != "in" {
		t.Errorf("pushed events = %+v", push.events)
	}
}

func TestSyncRejectsUnregisteredDevice(t *testing.T) {
	d := fullDeps(newFakeStore(), &fakePusher{})
	w, r := signed(t, "/api/sync", "door-x", `{"student_id":"s1","status":"in","marked_at":"2026-11-20T08:00:00Z"}`, testSecret)
	Sync(d, w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("sync = %d, want 403", w.Code)
	}
	if !strings.Contains(w.Body.String(), "register") {
		t.Errorf("body should point the device at /api/register: %s", w.Body.String())
	}
}

func TestSyncRejectsBadAndMissingSignatures(t *testing.T) {
	d := fullDeps(newFakeStore(), &fakePusher{})
	st := newFakeStore()
	_ = st.UpsertDevice(context.Background(), store.Device{DeviceID: "door-1"})

	// Signed body, tampered before send.
	w, r := signed(t, "/api/sync", "door-1", `{"student_id":"s1","status":"in"}`, testSecret)
	r2 := httptest.NewRequest(http.MethodPost, "/api/sync", strings.NewReader(`{"student_id":"s1","status":"out"}`))
	r2.Header = r.Header
	Sync(d, w, r2)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("tampered body = %d, want 401", w.Code)
	}

	// No signing headers at all.
	r3 := httptest.NewRequest(http.MethodPost, "/api/sync", strings.NewReader(`{"student_id":"s1","status":"in"}`))
	w3 := httptest.NewRecorder()
	Sync(d, w3, r3)
	if w3.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned = %d, want 401", w3.Code)
	}
}

func TestSyncRequiresConfigAndNamesMissingVars(t *testing.T) {
	// Nothing configured.
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/sync", strings.NewReader("{}"))
	Sync(Deps{}, w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("bare deps = %d, want 503", w.Code)
	}
	m := bodyMap(t, w)
	missing, _ := m["missing"].([]any)
	if len(missing) != 3 {
		t.Errorf("missing = %v, want all three vars", missing)
	}

	// Secret present, persistence missing.
	w2 := httptest.NewRecorder()
	Sync(Deps{Secret: testSecret}, w2, r)
	if w2.Code != http.StatusServiceUnavailable {
		t.Fatalf("no store = %d, want 503", w2.Code)
	}
	m2 := bodyMap(t, w2)
	missing2, _ := m2["missing"].([]any)
	if len(missing2) != 2 {
		t.Errorf("missing = %v, want only the Turso pair", missing2)
	}
}

func TestSyncValidatesBody(t *testing.T) {
	d := fullDeps(newFakeStore(), &fakePusher{})
	cases := []struct {
		name, body string
		want       int
	}{
		{"missing student", `{"status":"in"}`, http.StatusBadRequest},
		{"missing status", `{"student_id":"s1"}`, http.StatusBadRequest},
		{"bad status", `{"student_id":"s1","status":"late"}`, http.StatusBadRequest},
		{"missing marked_at", `{"student_id":"s1","status":"in"}`, http.StatusBadRequest},
		{"bad marked_at", `{"student_id":"s1","status":"in","marked_at":"tuesday"}`, http.StatusBadRequest},
		{"device mismatch", `{"device_id":"door-9","student_id":"s1","status":"in"}`, http.StatusBadRequest},
		{"not json", `nope`, http.StatusBadRequest},
	}
	st := newFakeStore()
	_ = st.UpsertDevice(context.Background(), store.Device{DeviceID: "door-1"})
	d.Store = st
	for _, tc := range cases {
		w, r := signed(t, "/api/sync", "door-1", tc.body, testSecret)
		Sync(d, w, r)
		if w.Code != tc.want {
			t.Errorf("%s: code = %d, want %d (%s)", tc.name, w.Code, tc.want, w.Body.String())
		}
	}
}

func TestSyncDeDuplicatesButKeepsPushing(t *testing.T) {
	st := newFakeStore()
	push := &fakePusher{}
	d := fullDeps(st, push)
	_ = st.UpsertDevice(context.Background(), store.Device{DeviceID: "door-1"})
	row := `{"device_id":"door-1","student_id":"s1","status":"in","marked_at":"2026-11-20T08:15:00Z"}`

	w, r := signed(t, "/api/sync", "door-1", row, testSecret)
	Sync(d, w, r)
	w2, r2 := signed(t, "/api/sync", "door-1", row, testSecret) // lost-ack retry
	Sync(d, w2, r2)

	m := bodyMap(t, w2)
	if m["duplicate"] != true || m["stored"] != true {
		t.Errorf("retry payload = %v, want duplicate=true stored=true", m)
	}
	if len(st.rows) != 1 {
		t.Errorf("rows stored = %d, want 1", len(st.rows))
	}
	// Both attempts push: a retry may be the only chance to notify parents,
	// and the parent app de-duplicates by (student, status, instant).
	if len(push.events) != 2 {
		t.Errorf("pushed events = %d, want 2", len(push.events))
	}
}

func TestSyncPushFailureStillAcceptsRow(t *testing.T) {
	st := newFakeStore()
	d := fullDeps(st, &fakePusher{err: fmt.Errorf("relay down")})
	_ = st.UpsertDevice(context.Background(), store.Device{DeviceID: "door-1"})

	w, r := signed(t, "/api/sync", "door-1", `{"student_id":"s1","status":"out","marked_at":"2026-11-20T15:00:00Z"}`, testSecret)
	Sync(d, w, r)
	if w.Code != http.StatusAccepted {
		t.Fatalf("sync = %d, want 202 despite relay failure", w.Code)
	}
	if bodyMap(t, w)["pushed"] != false {
		t.Errorf("payload should report pushed=false")
	}
	if len(st.rows) != 1 {
		t.Errorf("row must still be stored")
	}
}

func TestSyncStorageFailureIs502(t *testing.T) {
	st := newFakeStore()
	st.insertErr = fmt.Errorf("db offline")
	d := fullDeps(st, &fakePusher{})
	_ = st.UpsertDevice(context.Background(), store.Device{DeviceID: "door-1"})

	w, r := signed(t, "/api/sync", "door-1", `{"student_id":"s1","status":"in","marked_at":"2026-11-20T08:00:00Z"}`, testSecret)
	Sync(d, w, r)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("sync = %d, want 502", w.Code)
	}
}

// F1 from the review: schema bootstrap failures must surface as 502 (retry
// later), never as a silent skip.
func TestSyncSchemaInitFailureIs502(t *testing.T) {
	st := newFakeStore()
	st.initErr = fmt.Errorf("no such table")
	d := fullDeps(st, &fakePusher{})
	_ = st.UpsertDevice(context.Background(), store.Device{DeviceID: "door-1"})

	w, r := signed(t, "/api/sync", "door-1", `{"student_id":"s1","status":"in","marked_at":"2026-11-20T08:00:00Z"}`, testSecret)
	Sync(d, w, r)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("sync = %d, want 502 on init failure", w.Code)
	}
}

func TestSyncRejectsWrongMethod(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/sync", nil)
	Sync(fullDeps(newFakeStore(), &fakePusher{}), w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET sync = %d, want 405", w.Code)
	}
}

func TestRegisterValidatesDeviceIdentity(t *testing.T) {
	d := fullDeps(newFakeStore(), &fakePusher{})

	w, r := signed(t, "/api/register", "door-1", `{"device_id":"door-2"}`, testSecret)
	Register(d, w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("identity mismatch = %d, want 400", w.Code)
	}

	w, r = signed(t, "/api/register", "door-1", `{"label":"no id"}`, testSecret)
	Register(d, w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing device_id = %d, want 400", w.Code)
	}

	w, r = signed(t, "/api/register", "door-1", `{"device_id":"door-1","label":"ok"}`, testSecret)
	Register(d, w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("valid register = %d (%s)", w.Code, w.Body.String())
	}
}

func TestHealthReportsConfiguration(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	Health(Deps{}, w, r)
	m := bodyMap(t, w)
	if m["status"] != "ok" || m["service"] != "school-attendance-server" {
		t.Errorf("health base shape changed: %v", m)
	}
	if m["turso"] != "missing" || m["signing"] != false || m["fanout"] != "disabled" {
		t.Errorf("bare health = %v", m)
	}

	w2 := httptest.NewRecorder()
	Health(fullDeps(newFakeStore(), &fakePusher{}), w2, r)
	m2 := bodyMap(t, w2)
	if m2["turso"] != "configured" || m2["signing"] != true || m2["fanout"] != "configured" {
		t.Errorf("configured health = %v", m2)
	}
}
