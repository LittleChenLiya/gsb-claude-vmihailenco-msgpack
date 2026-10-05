package msgpack

import (
	"testing"
	"time"
)

// TestEncodeTimestampStrict checks the strict encoder directly: illegal
// nanoseconds are rejected, layout selection uses range checks instead of
// truncation, and negative seconds are kept as signed two's complement.
func TestEncodeTimestampStrict(t *testing.T) {
	buf := make([]byte, timestamp96Len)

	// Illegal nanoseconds must be rejected before any byte is written.
	if _, err := encodeTimestamp(buf, 0, 1_000_000_000); err == nil {
		t.Fatal("expected error for nanoseconds=1000000000")
	}
	if _, err := encodeTimestamp(buf, 0, 1_000_000_001); err == nil {
		t.Fatal("expected error for nanoseconds=1000000001")
	}

	// Legal boundary nanoseconds pass.
	b, err := encodeTimestamp(buf, 0, maxTimestampNanoseconds)
	if err != nil {
		t.Fatalf("unexpected error for nanoseconds=999999999: %v", err)
	}
	if len(b) != timestamp64Len {
		t.Fatalf("got body len=%d, want %d (timestamp 64)", len(b), timestamp64Len)
	}

	// Seconds overflowing 32 bits must not be truncated to timestamp 32.
	b, err = encodeTimestamp(buf, 1<<32, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != timestamp64Len {
		t.Fatalf("got body len=%d, want %d (timestamp 64)", len(b), timestamp64Len)
	}

	// Seconds overflowing 34 bits must not be truncated to timestamp 64.
	b, err = encodeTimestamp(buf, 1<<34, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != timestamp96Len {
		t.Fatalf("got body len=%d, want %d (timestamp 96)", len(b), timestamp96Len)
	}

	// Negative seconds must not be wrapped to an unsigned value: the 96-byte
	// layout keeps the signed two's complement bit pattern.
	b, err = encodeTimestamp(buf, -1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != timestamp96Len {
		t.Fatalf("got body len=%d, want %d (timestamp 96)", len(b), timestamp96Len)
	}
	for i := 4; i < timestamp96Len; i++ {
		if b[i] != 0xff {
			t.Fatalf("negative seconds lost: got %x, want ff ff ff ff ff ff ff ff", b[4:])
		}
	}

	// The zero instant uses the shortest legal layout (timestamp 32).
	b, err = encodeTimestamp(buf, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != timestamp32Len {
		t.Fatalf("got body len=%d, want %d (timestamp 32)", len(b), timestamp32Len)
	}
}

// TestDecodeTimestampStrict checks the strict decoder directly: wrong
// extension lengths and illegal nanoseconds are rejected, and the result is
// normalized to UTC.
func TestDecodeTimestampStrict(t *testing.T) {
	// Wrong extension lengths are rejected.
	for _, n := range []int{0, 1, 2, 3, 5, 13, 16} {
		b := make([]byte, n)
		if _, err := decodeTimestamp(b); err == nil {
			t.Fatalf("expected error for extension length=%d", n)
		}
	}

	// Illegal nanoseconds are rejected instead of being normalized.
	b64 := make([]byte, 8)
	b64[0] = 0xee
	b64[1] = 0x6b
	b64[2] = 0x28
	if _, err := decodeTimestamp(b64); err == nil {
		t.Fatal("expected error for timestamp 64 nanoseconds=1000000000")
	}
	b96 := make([]byte, 12)
	b96[0] = 0x3b
	b96[1] = 0x9a
	b96[2] = 0xca
	b96[3] = 0x00
	if _, err := decodeTimestamp(b96); err == nil {
		t.Fatal("expected error for timestamp 96 nanoseconds=1000000000")
	}

	// Negative seconds in the 96-byte layout are restored as signed values.
	neg := []byte{0, 0, 0, 0, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	tm, err := decodeTimestamp(neg)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Unix(-1, 0).UTC()
	if !tm.Equal(want) {
		t.Fatalf("got %v, want %v", tm, want)
	}
	if tm.Location() != time.UTC {
		t.Fatalf("got location %v, want UTC", tm.Location())
	}
}
