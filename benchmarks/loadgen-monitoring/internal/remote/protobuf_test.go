package remote

import (
	"encoding/binary"
	"math"
	"testing"
)

// minimal decoder mirroring protobuf.go's encoder, used only to verify the
// hand-rolled wire format round-trips correctly.

func decodeVarint(b []byte) (uint64, int) {
	var v uint64
	var shift uint
	for i, c := range b {
		v |= uint64(c&0x7f) << shift
		if c < 0x80 {
			return v, i + 1
		}
		shift += 7
	}
	return 0, len(b)
}

func decodeWriteRequest(t *testing.T, buf []byte) []TimeSeries {
	t.Helper()
	var out []TimeSeries
	for len(buf) > 0 {
		tag, n := decodeVarint(buf)
		buf = buf[n:]
		field, wireType := tag>>3, tag&0x7
		if field != 1 || wireType != wireBytes {
			t.Fatalf("unexpected top-level field %d wire %d", field, wireType)
		}
		l, n := decodeVarint(buf)
		buf = buf[n:]
		out = append(out, decodeTimeSeries(t, buf[:l]))
		buf = buf[l:]
	}
	return out
}

func decodeTimeSeries(t *testing.T, buf []byte) TimeSeries {
	t.Helper()
	var ts TimeSeries
	for len(buf) > 0 {
		tag, n := decodeVarint(buf)
		buf = buf[n:]
		field, wireType := tag>>3, tag&0x7
		if wireType != wireBytes {
			t.Fatalf("unexpected wire type %d for field %d", wireType, field)
		}
		l, n := decodeVarint(buf)
		buf = buf[n:]
		msg := buf[:l]
		buf = buf[l:]
		switch field {
		case 1:
			ts.Labels = append(ts.Labels, decodeLabel(t, msg))
		case 2:
			ts.Samples = append(ts.Samples, decodeSample(t, msg))
		default:
			t.Fatalf("unexpected field %d in TimeSeries", field)
		}
	}
	return ts
}

func decodeLabel(t *testing.T, buf []byte) Label {
	t.Helper()
	var l Label
	for len(buf) > 0 {
		tag, n := decodeVarint(buf)
		buf = buf[n:]
		field, wireType := tag>>3, tag&0x7
		if wireType != wireBytes {
			t.Fatalf("unexpected wire type %d for Label field %d", wireType, field)
		}
		ln, n := decodeVarint(buf)
		buf = buf[n:]
		s := string(buf[:ln])
		buf = buf[ln:]
		switch field {
		case 1:
			l.Name = s
		case 2:
			l.Value = s
		}
	}
	return l
}

func decodeSample(t *testing.T, buf []byte) Sample {
	t.Helper()
	var s Sample
	for len(buf) > 0 {
		tag, n := decodeVarint(buf)
		buf = buf[n:]
		field, wireType := tag>>3, tag&0x7
		switch field {
		case 1:
			if wireType != wire64 {
				t.Fatalf("expected wire64 for Sample.value")
			}
			s.Value = math.Float64frombits(binary.LittleEndian.Uint64(buf[:8]))
			buf = buf[8:]
		case 2:
			if wireType != wireVarint {
				t.Fatalf("expected varint for Sample.timestamp")
			}
			v, n := decodeVarint(buf)
			s.TimestampMs = int64(v)
			buf = buf[n:]
		}
	}
	return s
}

func TestEncodeWriteRequestRoundTrip(t *testing.T) {
	in := []TimeSeries{
		{
			Labels: []Label{
				{Name: "__name__", Value: "bench_metric"},
				{Name: "region", Value: "us-east-1"},
			},
			Samples: []Sample{
				{Value: 42.5, TimestampMs: 1700000000000},
				{Value: -3.25, TimestampMs: 1700000015000},
			},
		},
		{
			Labels: []Label{
				{Name: "__name__", Value: "bench_metric"},
				{Name: "region", Value: "eu-west-1"},
			},
			Samples: []Sample{
				{Value: 0, TimestampMs: 1700000030000},
			},
		},
	}

	raw := EncodeWriteRequest(in)
	out := decodeWriteRequest(t, raw)

	if len(out) != len(in) {
		t.Fatalf("got %d series, want %d", len(out), len(in))
	}
	for i := range in {
		if len(out[i].Labels) != len(in[i].Labels) {
			t.Fatalf("series %d: got %d labels, want %d", i, len(out[i].Labels), len(in[i].Labels))
		}
		for j := range in[i].Labels {
			if out[i].Labels[j] != in[i].Labels[j] {
				t.Fatalf("series %d label %d: got %+v, want %+v", i, j, out[i].Labels[j], in[i].Labels[j])
			}
		}
		if len(out[i].Samples) != len(in[i].Samples) {
			t.Fatalf("series %d: got %d samples, want %d", i, len(out[i].Samples), len(in[i].Samples))
		}
		for j := range in[i].Samples {
			if out[i].Samples[j] != in[i].Samples[j] {
				t.Fatalf("series %d sample %d: got %+v, want %+v", i, j, out[i].Samples[j], in[i].Samples[j])
			}
		}
	}
}
