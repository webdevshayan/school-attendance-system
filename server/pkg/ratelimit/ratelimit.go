// Package ratelimit keeps brute-force-prone endpoints cheap to abuse.
//
// /api/pair accepts a short, human-readable code as the credential, so a
// leaked code shape could be ground against the redemption endpoint. This
// limiter is an in-memory fixed-window counter per client IP: every
// serverless instance enforces its own counter (no cross-instance state
// exists on this runtime), which makes guessing slow and observable without
// adding a dependency. Abuse from many IPs at once remains theoretically
// possible - the code's entropy and TTL are the real bound; this limiter
// keeps single-source grinding pointless.
package ratelimit

import (
	"sync"
	"time"
)

// maxKeys bounds the bucket map: an instance that saw this many distinct
// keys in one window drops all counters (an acceptable, documented trade -
// it only matters under a key flood, where per-key state is moot anyway).
const maxKeys = 10_000

// Limiter is a fixed-window counter per key.
type Limiter struct {
	max     int
	window  time.Duration
	mu      sync.Mutex
	buckets map[string]*bucket
	now     func() time.Time // test seam
}

type bucket struct {
	count   int
	started time.Time
}

// Decision reports whether one event for key is allowed and, when it is
// not, how long until the window resets (drives the Retry-After header).
type Decision struct {
	Allowed    bool
	RetryAfter time.Duration
}

// NewPerMinute builds a limiter allowing max events per key per 60 s window.
func NewPerMinute(max int) *Limiter {
	return &Limiter{
		max:     max,
		window:  time.Minute,
		buckets: make(map[string]*bucket),
		now:     time.Now,
	}
}

// WithClock overrides the clock (test seam).
func (l *Limiter) WithClock(now func() time.Time) *Limiter {
	l.now = now
	return l
}

// Allow consumes one event for key.
func (l *Limiter) Allow(key string) Decision {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	b, ok := l.buckets[key]
	if !ok || now.Sub(b.started) >= l.window {
		if len(l.buckets) >= maxKeys {
			l.buckets = make(map[string]*bucket)
		}
		l.buckets[key] = &bucket{count: 1, started: now}
		return Decision{Allowed: true}
	}
	b.count++
	if b.count <= l.max {
		return Decision{Allowed: true}
	}
	return Decision{Allowed: false, RetryAfter: l.window - now.Sub(b.started)}
}
