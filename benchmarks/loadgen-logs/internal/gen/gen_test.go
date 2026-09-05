package gen

import (
	mrand "math/rand"
	"strings"
	"testing"
	"time"
)

func TestServicesDeterministic(t *testing.T) {
	got := Services(3)
	want := []string{"svc-000", "svc-001", "svc-002"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Services(3)[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if len(Services(0)) != 1 {
		t.Errorf("Services(0) must degrade to a single service")
	}
}

func TestEntryTimestampsStayInsideWindow(t *testing.T) {
	rnd := mrand.New(mrand.NewSource(1))
	services := Services(4)
	window := 5 * time.Minute
	nowNs := time.Now().UnixNano()

	for i := 0; i < 500; i++ {
		e := Entry(i, services, nowNs, window.Nanoseconds(), rnd)
		// Never in the future — both backends reject or hide such entries.
		if e.TimestampNs >= nowNs {
			t.Fatalf("entry %d timestamp is not in the past", i)
		}
		// Never older than the window queries look back across.
		if e.TimestampNs < nowNs-window.Nanoseconds()-int64(time.Second) {
			t.Fatalf("entry %d timestamp fell outside the query window", i)
		}
	}
}

func TestEntryRoundRobinsServices(t *testing.T) {
	rnd := mrand.New(mrand.NewSource(1))
	services := Services(4)
	for i := 0; i < 8; i++ {
		e := Entry(i, services, time.Now().UnixNano(), int64(time.Minute), rnd)
		if want := services[i%4]; e.Service != want {
			t.Errorf("entry %d service = %q, want %q", i, e.Service, want)
		}
	}
}

func TestFilterTokenDensity(t *testing.T) {
	rnd := mrand.New(mrand.NewSource(1))
	services := Services(4)
	const n = 1000

	hits := 0
	for i := 0; i < n; i++ {
		e := Entry(i, services, time.Now().UnixNano(), int64(time.Minute), rnd)
		if strings.Contains(e.Message, FilterToken) {
			hits++
		}
	}
	// The filter-match benchmark is only meaningful if the needle density is
	// exactly what both backends are told to expect.
	if want := n / TokenEveryN; hits != want {
		t.Errorf("token appeared in %d of %d lines, want %d", hits, n, want)
	}
}

func TestEntryMessagesAreUnique(t *testing.T) {
	rnd := mrand.New(mrand.NewSource(1))
	services := Services(4)
	seen := make(map[string]bool)
	for i := 0; i < 500; i++ {
		e := Entry(i, services, time.Now().UnixNano(), int64(time.Minute), rnd)
		// Loki drops an entry whose stream, timestamp and line all repeat, so
		// duplicate bodies would silently shrink the ingested dataset.
		if seen[e.Message] {
			t.Fatalf("duplicate message at %d: %q", i, e.Message)
		}
		seen[e.Message] = true
	}
}
