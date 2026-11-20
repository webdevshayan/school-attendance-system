package fanout

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebhookPusherSendsWireShapedEvent(t *testing.T) {
	var gotPath, gotAuth, gotBody, gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	p := NewWebhook(srv.URL+"/relay/attendance", "relay-key")
	err := p.Push(context.Background(), Event{
		DeviceID: "door-1", StudentID: "s1", Status: "in", MarkedAt: "2026-11-20T08:15:00Z",
	})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if gotPath != "/relay/attendance" {
		t.Errorf("path = %q", gotPath)
	}
	if gotAuth != "Bearer relay-key" {
		t.Errorf("auth = %q, want bearer relay key", gotAuth)
	}
	if gotCT != "application/json" {
		t.Errorf("content type = %q", gotCT)
	}
	// The payload must be identical in shape to /api/sync - the parent's
	// PushHandler parses exactly these keys.
	var event map[string]any
	if err := json.Unmarshal([]byte(gotBody), &event); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	for _, key := range []string{"device_id", "student_id", "status", "marked_at"} {
		if _, ok := event[key]; !ok {
			t.Errorf("payload missing %q (parent PushHandler contract): %s", key, gotBody)
		}
	}
}

func TestWebhookPusherOmitsAuthHeaderWhenUnkeyed(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := NewWebhook(srv.URL, "")
	if err := p.Push(context.Background(), Event{StudentID: "s1"}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if gotAuth != "" {
		t.Errorf("auth header set without API key: %q", gotAuth)
	}
}

func TestWebhookPusherRejectsRelayFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	p := NewWebhook(srv.URL, "k")
	err := p.Push(context.Background(), Event{StudentID: "s1"})
	if err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("err = %v, want relay HTTP 503 failure", err)
	}
}
