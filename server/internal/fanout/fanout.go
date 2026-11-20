// Package fanout sends attendance events to the parent side through the
// Nitroping relay.
//
// The parent app's PushHandler is transport-agnostic: it accepts the same
// wire shape as /api/sync (device_id, student_id, status, marked_at), so the
// relay payload mirrors that shape verbatim and the parent needs no change
// when the channel goes live. Fan-out is best-effort: attendance is already
// durable in Turso when the push fires, and a relay outage must never fail a
// mark that was captured at the door.
package fanout

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Event is one attendance mark ready for delivery.
type Event struct {
	DeviceID  string `json:"device_id"`
	StudentID string `json:"student_id"`
	Status    string `json:"status"`    // "in" or "out"
	MarkedAt  string `json:"marked_at"` // RFC3339
}

// Pusher delivers one event to the parent side. Nil pusher (handlers treat
// it so) means fan-out is not configured; nothing is sent.
type Pusher interface {
	Push(ctx context.Context, e Event) error
}

// WebhookPusher posts events to the Nitroping relay webhook. Configure with
// NITROPING_WEBHOOK_URL and optionally NITROPING_API_KEY (sent as a bearer
// token); routing to the child's guardians happens inside the relay.
type WebhookPusher struct {
	URL    string
	APIKey string
	HTTP   *http.Client
}

// NewWebhook builds a relay pusher with a short timeout: a push must never
// outlive a sync request's useful work.
func NewWebhook(url, apiKey string) *WebhookPusher {
	return &WebhookPusher{URL: url, APIKey: apiKey, HTTP: &http.Client{Timeout: 5 * time.Second}}
}

// Push posts one event; any non-2xx relay answer is an error.
func (p *WebhookPusher) Push(ctx context.Context, e Event) error {
	payload, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("fanout: encode event: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.URL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("fanout: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if p.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.APIKey)
	}
	resp, err := p.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("fanout: relay request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("fanout: relay HTTP %d", resp.StatusCode)
	}
	return nil
}
