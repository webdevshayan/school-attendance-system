package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// postPair drives one POST /api/pair through the mux from ip.
func postPair(t *testing.T, mux *http.ServeMux, ip, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/api/pair", strings.NewReader(body))
	r.Header.Set("X-Forwarded-For", ip)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

func TestPairRateLimitBlocksGrinding(t *testing.T) {
	t.Setenv("PAIR_RATE_LIMIT", "2")
	mux := New()
	// With no store configured Pair answers 503; the limiter must still
	// count every attempt, so the third one is blocked with 429.
	if w := postPair(t, mux, "203.0.113.7", `{"code":"ABCDEFGH"}`); w.Code == http.StatusTooManyRequests {
		t.Fatalf("first attempt blocked (status %d), want 503", w.Code)
	}
	if w := postPair(t, mux, "203.0.113.7", `{"code":"ABCDEFGH"}`); w.Code == http.StatusTooManyRequests {
		t.Fatalf("second attempt blocked (status %d), want 503", w.Code)
	}
	w := postPair(t, mux, "203.0.113.7", `{"code":"ABCDEFGH"}`)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("third attempt status %d, want 429", w.Code)
	}
	if ra := w.Header().Get("Retry-After"); ra == "" || ra == "0" {
		t.Fatalf("Retry-After = %q, want a positive integer", ra)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body["error"] == "" {
		t.Fatalf("429 body = %q, want a JSON error message", w.Body.String())
	}
}

func TestPairRateLimitIsPerIP(t *testing.T) {
	t.Setenv("PAIR_RATE_LIMIT", "1")
	mux := New()
	if w := postPair(t, mux, "203.0.113.7", `{}`); w.Code == http.StatusTooManyRequests {
		t.Fatal("first attempt for .7 blocked")
	}
	if w := postPair(t, mux, "203.0.113.8", `{}`); w.Code == http.StatusTooManyRequests {
		t.Fatal("first attempt for .8 blocked by .7's counter")
	}
	if w := postPair(t, mux, "203.0.113.7", `{}`); w.Code != http.StatusTooManyRequests {
		t.Fatalf("second attempt for .7 status %d, want 429", w.Code)
	}
}

func TestPairRateLimitDisabledByZero(t *testing.T) {
	t.Setenv("PAIR_RATE_LIMIT", "0")
	mux := New()
	for i := 0; i < 25; i++ {
		if w := postPair(t, mux, "203.0.113.7", `{}`); w.Code == http.StatusTooManyRequests {
			t.Fatalf("attempt %d blocked with PAIR_RATE_LIMIT=0", i+1)
		}
	}
}

func TestPairLimiterFromEnv(t *testing.T) {
	if pairLimiterFromEnv("") == nil {
		t.Fatal("unset env must build the default limiter")
	}
	if pairLimiterFromEnv("junk") == nil {
		t.Fatal("unparseable env must fall back to the default limiter")
	}
	if pairLimiterFromEnv("0") != nil {
		t.Fatal(`"0" must disable the limiter`)
	}
	if pairLimiterFromEnv("5") == nil {
		t.Fatal("positive value must build a limiter")
	}
}

func TestClientIP(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/pair", nil)
	r.RemoteAddr = "198.51.100.9:44444"
	if got := clientIP(r); got != "198.51.100.9" {
		t.Fatalf("clientIP(RemoteAddr) = %q", got)
	}
	r.Header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.1")
	if got := clientIP(r); got != "203.0.113.7" {
		t.Fatalf("clientIP(XFF multi) = %q, want the first hop", got)
	}
	r.Header.Set("X-Forwarded-For", "203.0.113.11")
	if got := clientIP(r); got != "203.0.113.11" {
		t.Fatalf("clientIP(XFF single) = %q", got)
	}
}
