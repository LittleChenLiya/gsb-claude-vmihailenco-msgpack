package msgpack_test

import (
	"bytes"
	"encoding/hex"
	"testing"
	"time"

	"github.com/vmihailenco/msgpack/v5"
)

// Reference vectors for the MessagePack Timestamp extension (type -1), see
// https://github.com/msgpack/msgpack/blob/master/spec.md#timestamp-extension-type
//
// Layouts covered below:
//
//	d6 ff            timestamp32,  4 data bytes: uint32 seconds.
//	                 Used only when nsec == 0 and 0 <= sec < 2^32.
//	d7 ff            timestamp64,  8 data bytes: high 30 bits nsec,
//	                 low 34 bits uint32 seconds. Used when nsec != 0 (or sec
//	                 needs more than 32 bits) and 0 <= sec < 2^34.
//	c7 0c ff         timestamp96, 12 data bytes: uint32 nsec then int64
//	                 seconds. The only layout that keeps negative seconds
//	                 signed, and the layout for sec >= 2^34.
func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("bad hex %q: %v", s, err)
	}
	return b
}

// marshalTime encodes a time value and returns the exact wire bytes.
func marshalTime(t *testing.T, tm time.Time) []byte {
	t.Helper()
	var buf bytes.Buffer
	enc := msgpack.NewEncoder(&buf)
	if err := enc.EncodeTime(tm); err != nil {
		t.Fatalf("EncodeTime(%v) error: %v", tm, err)
	}
	return buf.Bytes()
}

// unmarshalTime decodes an exact ext frame without touching any other bytes.
func unmarshalTime(t *testing.T, frame []byte) (time.Time, error) {
	t.Helper()
	dec := msgpack.NewDecoder(bytes.NewReader(frame))
	return dec.DecodeTime()
}

func TestTimestampEncodeLayouts(t *testing.T) {
	tests := []struct {
		name   string
		in     time.Time
		wanted string // full ext frame, checked byte-by-byte
	}{
		{
			name:   "epoch zero uses timestamp32",
			in:     time.Unix(0, 0),
			wanted: "d6ff00000000",
		},
		{
			name:   "small positive second uses timestamp32",
			in:     time.Unix(1, 0),
			wanted: "d6ff00000001",
		},
		{
			name:   "32-bit last second still uses timestamp32",
			in:     time.Unix(1<<32-1, 0),
			wanted: "d6ffffffffff",
		},
		{
			name:   "32-bit overflow promotes to timestamp64",
			in:     time.Unix(1<<32, 0),
			wanted: "d7ff0000000100000000",
		},
		{
			name:   "nonzero nsec promotes to timestamp64",
			in:     time.Unix(1, 1),
			wanted: "d7ff0000000400000001",
		},
		{
			name:   "max nsec with sec=1 fits timestamp64",
			in:     time.Unix(1, 999999999),
			wanted: "d7ffee6b27fc00000001",
		},
		{
			name:   "64-bit boundary sec=2^34-1 with max nsec",
			in:     time.Unix(1<<34-1, 999999999),
			wanted: "d7ffee6b27ffffffffff",
		},
		{
			name:   "64-bit overflow sec=2^34 promotes to timestamp96",
			in:     time.Unix(1<<34, 0),
			wanted: "c70cff000000000000000400000000",
		},
		{
			name:   "large second uses timestamp96",
			in:     time.Unix(1<<40, 42),
			wanted: "c70cff0000002a0000010000000000",
		},
		{
			name:   "negative second keeps signed and uses timestamp96",
			in:     time.Unix(-1, 0),
			wanted: "c70cff00000000ffffffffffffffff",
		},
		{
			name:   "negative second with nsec uses timestamp96",
			in:     time.Unix(-1, 5),
			wanted: "c70cff00000005ffffffffffffffff",
		},
		{
			name:   "Go zero time uses timestamp96",
			in:     time.Time{},
			wanted: "c70cff00000000fffffff1886e0900",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := marshalTime(t, tt.in.UTC())
			want := mustHex(t, tt.wanted)
			if !bytes.Equal(got, want) {
				t.Fatalf("wire bytes mismatch:\n got  %x\n want %x", got, want)
			}
		})
	}
}

func TestTimestampDecodeRoundTrip(t *testing.T) {
	// Every encoded frame must decode back to the same UTC instant and must
	// report UTC regardless of the machine's local time zone.
	instances := []time.Time{
		time.Unix(0, 0),
		time.Unix(1, 0),
		time.Unix(1<<32-1, 0),
		time.Unix(1<<32, 0),
		time.Unix(1, 999999999),
		time.Unix(1<<34-1, 999999999),
		time.Unix(1<<34, 0),
		time.Unix(1<<40, 42),
		time.Unix(-1, 0),
		time.Unix(-1, 5),
	}

	for _, in := range instances {
		frame := marshalTime(t, in.UTC())
		out, err := unmarshalTime(t, frame)
		if err != nil {
			t.Fatalf("decode %x error: %v", frame, err)
		}
		if !out.Equal(in) {
			t.Fatalf("instant mismatch for %x: got %v, want %v", frame, out, in)
		}
		if zone, _ := out.Zone(); zone != "UTC" {
			t.Fatalf("decoded time must be UTC, got zone %q for %x", zone, frame)
		}
	}
}

func TestTimestampDecodeVectors(t *testing.T) {
	tests := []struct {
		name  string
		frame string
		in    time.Time
	}{
		{"timestamp32", "d6ff00000000", time.Unix(0, 0)},
		{"timestamp32 max sec", "d6ffffffffff", time.Unix(1<<32-1, 0)},
		{"timestamp64 sec nsec", "d7ff0000000400000001", time.Unix(1, 1)},
		{"timestamp64 max nsec", "d7ffee6b27fc00000001", time.Unix(1, 999999999)},
		{"timestamp64 boundary", "d7ffee6b27ffffffffff", time.Unix(1<<34-1, 999999999)},
		{"timestamp96 positive", "c70cff0000002a0000010000000000", time.Unix(1<<40, 42)},
		{"timestamp96 negative", "c70cff00000000ffffffffffffffff", time.Unix(-1, 0)},
		// time.Unix normalizes (-1, 5) to (-2, 999999995); compare instants.
		{"timestamp96 negative with carry", "c70cff00000005ffffffffffffffff", time.Unix(-1, 5)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := unmarshalTime(t, mustHex(t, tt.frame))
			if err != nil {
				t.Fatalf("decode error: %v", err)
			}
			if !out.Equal(tt.in) {
				t.Fatalf("instant mismatch: got (sec=%d nsec=%d), want %v",
					out.Unix(), out.Nanosecond(), tt.in)
			}
		})
	}
}

func TestTimestampRejectIllegalNanoseconds(t *testing.T) {
	// nsec == 999999999 must be accepted in both 8- and 12-byte layouts.
	if _, err := unmarshalTime(t, mustHex(t, "d7ffee6b27fc00000001")); err != nil {
		t.Fatalf("nsec=999999999 in timestamp64 must be accepted, got %v", err)
	}
	if _, err := unmarshalTime(t, mustHex(t, "c70cff3b9ac9ff0000000000000000")); err != nil {
		t.Fatalf("nsec=999999999 in timestamp96 must be accepted, got %v", err)
	}

	// nsec == 1000000000 must be rejected in every layout.
	bad := []string{
		// timestamp96: uint32 nsec 1000000000 (0x3b9aca00), sec 0.
		"c70cff3b9aca000000000000000000",
		// timestamp64: 30-bit nsec field holding 1000000000 (it fits in 30
		// bits but is still an illegal nanosecond count), sec field 0.
		"d7ffee6b280000000000",
	}
	for _, frame := range bad {
		if _, err := unmarshalTime(t, mustHex(t, frame)); err == nil {
			t.Fatalf("frame %s with illegal nanoseconds must be rejected", frame)
		}
	}
}

func TestTimestampRejectWrongLength(t *testing.T) {
	bad := []string{
		// ext8 length 1, type -1, one payload byte.
		"c701ff00",
		// ext8 length 3, type -1, three payload bytes.
		"c703ff000000",
		// ext8 length 16, type -1, sixteen payload bytes.
		"c710ff00000000000000000000000000000000",
	}
	for _, frame := range bad {
		if _, err := unmarshalTime(t, mustHex(t, frame)); err == nil {
			t.Fatalf("frame %s with unsupported timestamp length must be rejected", frame)
		}
	}
}

// TestTimestampOverMarshal exercises the default time.Time registration path
// (Marshal/Unmarshal) to prove the built-in time ext now uses the same strict
// codec while the generic RegisterExt API itself is untouched.
func TestTimestampOverMarshal(t *testing.T) {
	in := time.Unix(-123456789, 987654321)
	b, err := msgpack.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	// Negative seconds must land in the 12-byte timestamp96 layout.
	want := mustHex(t, "c70cff3ade68b1fffffffff8a432eb")
	if !bytes.Equal(b, want) {
		t.Fatalf("Marshal bytes:\n got  %x\n want %x", b, want)
	}

	var out time.Time
	if err := msgpack.Unmarshal(b, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !out.Equal(in) {
		t.Fatalf("instant mismatch: got %v, want %v", out, in)
	}
	if zone, _ := out.Zone(); zone != "UTC" {
		t.Fatalf("decoded time must be UTC, got %q", zone)
	}
}
