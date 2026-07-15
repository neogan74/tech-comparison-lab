package pulsar

import (
	"strings"
	"testing"
)

func TestNormalizeTopic(t *testing.T) {
	t.Parallel()

	if got := normalizeTopic("bench"); got != "persistent://public/default/bench" {
		t.Fatalf("normalizeTopic(short) = %q", got)
	}
	full := "persistent://tenant/ns/topic"
	if got := normalizeTopic(full); got != full {
		t.Fatalf("normalizeTopic(full) = %q", got)
	}
}

func TestMakeMsg(t *testing.T) {
	t.Parallel()

	msg := string(makeMsg(42))
	if !strings.Contains(msg, `"id":42`) || !strings.Contains(msg, `"p":"`) {
		t.Fatalf("unexpected message: %s", msg)
	}
}
