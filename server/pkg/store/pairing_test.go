package store

import (
	"context"
	"strings"
	"testing"
	"time"
)

// The pairing layer reuses the fakeTurso from store_test.go, now with
// textRowsResult for SELECTs that must return real cell values.

func TestMintCodeUpsertsAndFindCodeReadsBack(t *testing.T) {
	s, f := newTestStore(t,
		affectedResult(1),
		textRowsResult([]string{"s1", "2026-11-20T14:00:00Z", "2026-11-27T14:00:00Z"}),
		textRowsResult(), // unknown code → empty result
	)
	ctx := context.Background()
	code := PairingCode{
		Code: "ABCD2345", StudentID: "s1",
		CreatedAt: "2026-11-20T14:00:00Z", ExpiresAt: "2026-11-27T14:00:00Z",
	}
	if err := s.MintCode(ctx, code); err != nil {
		t.Fatalf("MintCode: %v", err)
	}
	if !strings.Contains(f.gotSQLs[0], "ON CONFLICT (code) DO UPDATE") {
		t.Errorf("mint SQL is not an upsert: %.80s", f.gotSQLs[0])
	}
	if f.gotArgs[0][0] != "ABCD2345" || f.gotArgs[0][1] != "s1" {
		t.Errorf("mint args = %+v", f.gotArgs[0])
	}

	found, ok, err := s.FindCode(ctx, "ABCD2345")
	if err != nil || !ok {
		t.Fatalf("FindCode = %v, %v; want found", ok, err)
	}
	if found.StudentID != "s1" || found.ExpiresAt != "2026-11-27T14:00:00Z" {
		t.Errorf("found = %+v", found)
	}

	unknown, ok, err := s.FindCode(ctx, "ZZZZ9999")
	if err != nil || ok || unknown.StudentID != "" {
		t.Fatalf("unknown code = %+v, %v, %v; want not found", unknown, ok, err)
	}
}

func TestParentDeviceRoundTrip(t *testing.T) {
	s, f := newTestStore(t,
		affectedResult(1),
		textRowsResult([]string{"s2"}),
		textRowsResult(),
	)
	ctx := context.Background()
	if err := s.UpsertParentDevice(ctx, ParentDevice{
		TokenHash: "deadbeef", StudentID: "s2",
		Label: "moms phone", Platform: "android", PairedAt: "2026-11-20T15:00:00Z",
	}); err != nil {
		t.Fatalf("UpsertParentDevice: %v", err)
	}
	if !strings.Contains(f.gotSQLs[0], "ON CONFLICT (token_hash) DO UPDATE") {
		t.Errorf("device SQL is not an upsert: %.80s", f.gotSQLs[0])
	}

	student, err := s.StudentForToken(ctx, "deadbeef")
	if err != nil || student != "s2" {
		t.Fatalf("StudentForToken = %q, %v; want s2", student, err)
	}
	student, err = s.StudentForToken(ctx, "unknown-hash")
	if err != nil || student != "" {
		t.Fatalf("unknown token = %q, %v; want empty", student, err)
	}
}

func TestEventsSinceReturnsRowsAndRespectsCursor(t *testing.T) {
	s, f := newTestStore(t, textRowsResult(
		[]string{"11", "door-1", "s1", "in", "2026-11-20T08:15:00Z", "2026-11-20T08:15:02Z"},
		[]string{"12", "door-1", "s1", "out", "2026-11-20T15:30:00Z", "2026-11-20T15:30:03Z"},
	))
	events, err := s.EventsSince(context.Background(), "s1", 10, 100)
	if err != nil {
		t.Fatalf("EventsSince: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	if events[0].ID != 11 || events[0].Status != "in" || events[0].DeviceID != "door-1" {
		t.Errorf("event0 = %+v", events[0])
	}
	if events[1].ID != 12 || events[1].Status != "out" {
		t.Errorf("event1 = %+v", events[1])
	}
	// The cursor and the limit must reach the SQL.
	if f.gotArgs[0][0] != "s1" || f.gotArgs[0][1] != "10" || f.gotArgs[0][2] != "100" {
		t.Errorf("events args = %+v, want s1/10/100", f.gotArgs[0])
	}
}

func TestEventsSinceEmptyResult(t *testing.T) {
	s, _ := newTestStore(t, textRowsResult())
	events, err := s.EventsSince(context.Background(), "s1", 999, 100)
	if err != nil || len(events) != 0 {
		t.Fatalf("events = %v, %v; want empty", events, err)
	}
}

// The fixed clock helper mirrors store_test.go's for pairing expiry tests.
func fixedNow() time.Time { return time.Date(2026, 11, 20, 14, 0, 0, 0, time.UTC) }
