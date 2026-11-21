// Package routes mounts the whole API on one mux. It is the single source of
// routing shared by the ways this service runs:
//
//   - main.go             the Vercel Go framework entrypoint (preset "Go") and
//     the local development server
//   - api/*.go            per-file functions (framework preset "Other", module mode)
//
// Configuration is read from the environment exactly like the api/ adapters
// do it (DepsFromEnv per mux), so every shape behaves identically.
package routes

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/webdevshayan/school-attendance-system/server/pkg/handlers"
	"github.com/webdevshayan/school-attendance-system/server/pkg/ratelimit"
)

// defaultPairRPM bounds /api/pair guesses per client IP per minute. The
// pairing code IS the credential, so redemption must resist grinding.
const defaultPairRPM = 10

// New builds a mux with every endpoint.
func New() *http.ServeMux {
	d := handlers.DepsFromEnv()
	mux := http.NewServeMux()
	mount := func(path string, h func(handlers.Deps, http.ResponseWriter, *http.Request)) {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			h(d, w, r)
		})
	}
	mount("/api/health", handlers.Health)
	mount("/api/register", handlers.Register)
	mount("/api/sync", handlers.Sync)
	mount("/api/codes", handlers.MintCode)
	mount("/api/codes/revoke", handlers.RevokeCode)
	pairLimiter := pairLimiterFromEnv(os.Getenv("PAIR_RATE_LIMIT"))
	mux.HandleFunc("/api/pair", func(w http.ResponseWriter, r *http.Request) {
		if pairLimiter != nil {
			if dec := pairLimiter.Allow(clientIP(r)); !dec.Allowed {
				tooManyAttempts(w, dec.RetryAfter)
				return
			}
		}
		handlers.Pair(d, w, r)
	})
	mount("/api/events", handlers.Events)
	return mux
}

// pairLimiterFromEnv parses PAIR_RATE_LIMIT (attempts per client IP per
// minute for /api/pair). Unset or unparseable -> the default; "0" -> the
// limiter is disabled (explicit escape hatch for trusted deployments).
func pairLimiterFromEnv(value string) *ratelimit.Limiter {
	n := defaultPairRPM
	if value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return ratelimit.NewPerMinute(defaultPairRPM)
		}
		if parsed <= 0 {
			return nil
		}
		n = parsed
	}
	return ratelimit.NewPerMinute(n)
}

// clientIP is the best-effort client identity for rate limiting. Vercel's
// edge puts the real client first in X-Forwarded-For (it overwrites
// client-supplied values); fall back to the socket peer for direct runs.
func clientIP(r *http.Request) string {
	if xf := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xf != "" {
		if i := strings.IndexByte(xf, ','); i >= 0 {
			xf = xf[:i]
		}
		return strings.TrimSpace(xf)
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// tooManyAttempts answers 429 with the header clients are told to honor.
func tooManyAttempts(w http.ResponseWriter, retryAfter time.Duration) {
	w.Header().Set("Content-Type", "application/json")
	retry := int(retryAfter.Seconds())
	if retry < 1 {
		retry = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(retry))
	w.WriteHeader(http.StatusTooManyRequests)
	json.NewEncoder(w).Encode(map[string]string{"error": "too many attempts; slow down"})
}
