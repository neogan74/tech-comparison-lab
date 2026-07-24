package remote

import (
	"encoding/binary"
	"math"
)

// Minimal hand-rolled protobuf wire encoder for the Prometheus remote_write
// v1 WriteRequest message, avoiding a dependency on the full
// github.com/prometheus/prometheus module for three tiny messages:
//
//	message Sample     { double value = 1; int64 timestamp = 2; }
//	message Label      { string name = 1; string value = 2; }
//	message TimeSeries  { repeated Label labels = 1; repeated Sample samples = 2; }
//	message WriteRequest { repeated TimeSeries timeseries = 1; }

const (
	wireVarint = 0
	wire64     = 1
	wireBytes  = 2
)

func appendTag(buf []byte, field int, wireType byte) []byte {
	return appendVarint(buf, uint64(field)<<3|uint64(wireType))
}

func appendVarint(buf []byte, v uint64) []byte {
	for v >= 0x80 {
		buf = append(buf, byte(v)|0x80)
		v >>= 7
	}
	return append(buf, byte(v))
}

func appendString(buf []byte, field int, s string) []byte {
	buf = appendTag(buf, field, wireBytes)
	buf = appendVarint(buf, uint64(len(s)))
	return append(buf, s...)
}

func appendBytes(buf []byte, field int, b []byte) []byte {
	buf = appendTag(buf, field, wireBytes)
	buf = appendVarint(buf, uint64(len(b)))
	return append(buf, b...)
}

func appendDouble(buf []byte, field int, v float64) []byte {
	buf = appendTag(buf, field, wire64)
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], math.Float64bits(v))
	return append(buf, b[:]...)
}

func appendInt64(buf []byte, field int, v int64) []byte {
	buf = appendTag(buf, field, wireVarint)
	return appendVarint(buf, uint64(v))
}

func encodeLabel(l Label) []byte {
	var buf []byte
	buf = appendString(buf, 1, l.Name)
	buf = appendString(buf, 2, l.Value)
	return buf
}

func encodeSample(s Sample) []byte {
	var buf []byte
	buf = appendDouble(buf, 1, s.Value)
	buf = appendInt64(buf, 2, s.TimestampMs)
	return buf
}

func encodeTimeSeries(ts TimeSeries) []byte {
	var buf []byte
	for _, l := range ts.Labels {
		buf = appendBytes(buf, 1, encodeLabel(l))
	}
	for _, s := range ts.Samples {
		buf = appendBytes(buf, 2, encodeSample(s))
	}
	return buf
}

// EncodeWriteRequest marshals series into an uncompressed protobuf
// WriteRequest payload, ready for snappy block compression.
func EncodeWriteRequest(series []TimeSeries) []byte {
	var buf []byte
	for _, ts := range series {
		buf = appendBytes(buf, 1, encodeTimeSeries(ts))
	}
	return buf
}
