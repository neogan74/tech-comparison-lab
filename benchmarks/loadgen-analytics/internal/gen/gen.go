package gen

import (
	"crypto/rand"
	"fmt"
	mrand "math/rand"
	"time"
)

// Event is the shared analytics event schema.
type Event struct {
	ID      string
	UserID  uint32
	Event   string
	TS      time.Time
	Country string
	Value   float32
	Session string
}

var (
	eventTypes = []string{"view", "click", "search", "add_to_cart", "purchase", "share"}
	countries  = []string{"US", "GB", "DE", "FR", "JP", "CA", "AU", "IN", "BR", "MX"}

	// Base time window: full year 2024 (~365 days in nanoseconds)
	baseTime  = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	yearNanos = int64(365 * 24 * time.Hour)
)

// New generates a single random Event.
func New() Event {
	ev := eventTypes[mrand.Intn(len(eventTypes))]
	val := float32(0)
	if ev == "purchase" {
		val = float32(mrand.Intn(1000) + 1)
	}
	return Event{
		ID:      newUUID(),
		UserID:  uint32(mrand.Intn(100_000)), // 100k unique users
		Event:   ev,
		TS:      baseTime.Add(time.Duration(mrand.Int63n(yearNanos))),
		Country: countries[mrand.Intn(len(countries))],
		Value:   val,
		Session: newUUID(),
	}
}

// Batch generates n Events.
func Batch(n int) []Event {
	events := make([]Event, n)
	for i := range events {
		events[i] = New()
	}
	return events
}

func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
