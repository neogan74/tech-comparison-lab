package zabbix

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestEncodeFraming(t *testing.T) {
	payload := []byte(`{"request":"sender data"}`)
	frame := encode(payload)

	if got := string(frame[:4]); got != "ZBXD" {
		t.Fatalf("magic = %q, want ZBXD", got)
	}
	if frame[4] != flagZabbix {
		t.Fatalf("flags = %#x, want %#x", frame[4], flagZabbix)
	}
	n := binary.LittleEndian.Uint32(frame[5:9])
	if int(n) != len(payload) {
		t.Fatalf("datalen = %d, want %d", n, len(payload))
	}
	if binary.LittleEndian.Uint32(frame[9:13]) != 0 {
		t.Fatalf("reserved field must be zero")
	}
	if !bytes.Equal(frame[headerLen:], payload) {
		t.Fatalf("payload mismatch")
	}
}

func TestDecodeRoundTrip(t *testing.T) {
	payload := []byte(`{"response":"success","info":"processed: 3; failed: 0"}`)
	frame := encode(payload)

	body, err := decode(bytes.NewReader(frame))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !bytes.Equal(body, payload) {
		t.Fatalf("round-trip mismatch: got %q", body)
	}
}

func TestDecodeBadMagic(t *testing.T) {
	bad := make([]byte, headerLen)
	copy(bad, "XXXX")
	if _, err := decode(bytes.NewReader(bad)); err == nil {
		t.Fatal("expected error on bad magic")
	}
}

func TestParseProcessed(t *testing.T) {
	cases := map[string]int{
		"processed: 5; failed: 0; total: 5; seconds spent: 0.000042": 5,
		"processed: 0; failed: 3; total: 3":                          0,
		"failed: 1":                                                  -1,
		"":                                                           -1,
	}
	for info, want := range cases {
		if got := parseProcessed(info); got != want {
			t.Errorf("parseProcessed(%q) = %d, want %d", info, got, want)
		}
	}
}
