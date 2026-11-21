package ratelimit

import (
	"testing"
	"time"
)

// steppedClock advances by step on every call, deterministic for tests.
type steppedClock struct {
	current time.Time
	step    time.Duration
}

func (c *steppedClock) Now() time.Time {
	t := c.current
	c.current = c.current.Add(c.step)
	return t
}

func TestAllowWithinMax(t *testing.T) {
	l := NewPerMinute(3)
	for i := 0; i < 3; i++ {
		if d := l.Allow("ip1"); !d.Allowed {
			t.Fatalf("event %d blocked, want allowed (decision %+v)", i+1, d)
		}
	}
	d := l.Allow("ip1")
	if d.Allowed {
		t.Fatal("4th event allowed, want blocked")
	}
	if d.RetryAfter <= 0 || d.RetryAfter > time.Minute {
		t.Fatalf("RetryAfter = %v, want in (0, 1m]", d.RetryAfter)
	}
}

func TestKeysAreIndependent(t *testing.T) {
	l := NewPerMinute(1)
	if d := l.Allow("ip1"); !d.Allowed {
		t.Fatal("first event for ip1 blocked")
	}
	if d := l.Allow("ip2"); !d.Allowed {
		t.Fatal("first event for ip2 blocked by ip1's counter")
	}
	if d := l.Allow("ip1"); d.Allowed {
		t.Fatal("second event for ip1 allowed")
	}
}

func TestWindowReset(t *testing.T) {
	clock := &steppedClock{current: time.Unix(0, 0), step: 30 * time.Second}
	l := NewPerMinute(1).WithClock(clock.Now)
	if d := l.Allow("ip1"); !d.Allowed {
		t.Fatal("first event blocked")
	}
	if d := l.Allow("ip1"); d.Allowed {
		t.Fatal("second event inside window allowed")
	}
	// Two more steps cross the 60 s window: the counter resets.
	clock.Now()
	if d := l.Allow("ip1"); !d.Allowed {
		t.Fatal("event after window reset blocked")
	}
}

func TestMapGCUnderFlood(t *testing.T) {
	l := NewPerMinute(1)
	for i := 0; i < maxKeys+1; i++ {
		l.Allow(keyFor(i))
	}
	if len(l.buckets) > maxKeys {
		t.Fatalf("bucket map grew to %d entries, GC should cap it", len(l.buckets))
	}
}

func keyFor(i int) string {
	return "ip-" + time.Unix(int64(i), 0).String()
}
