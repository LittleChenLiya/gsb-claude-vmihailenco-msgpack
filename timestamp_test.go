package msgpack_test

import (
	"bytes"
	"encoding/hex"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/vmihailenco/msgpack/v5"
)

// TestTimestampLayoutVectors pins the shortest legal layout selection for
// seconds at every boundary (0, 32-bit overflow, 34-bit overflow, negative
// seconds) with byte-exact vectors.
func TestTimestampLayoutVectors(t *testing.T) {
	tests := []struct {
		name string
		tm   time.Time
		want string // full msgpack bytes: ext header + timestamp body
	}{
		{
			name: "zero seconds uses timestamp 32",
			tm:   time.Unix(0, 0).UTC(),
			want: "d6ff" + "00000000",
		},
		{
			name: "small positive seconds uses timestamp 32",
			tm:   time.Unix(1, 0).UTC(),
			want: "d6ff" + "00000001",
		},
		{
			name: "largest 32-bit seconds uses timestamp 32",
			tm:   time.Unix(0xffffffff, 0).UTC(),
			want: "d6ff" + "ffffffff",
		},
		{
			name: "seconds overflowing 32 bits use timestamp 64, not truncated",
			tm:   time.Unix(1<<32, 0).UTC(),
			want: "d7ff" + "0000000100000000",
		},
		{
			name: "largest 34-bit seconds use timestamp 64",
			tm:   time.Unix(1<<34-1, 0).UTC(),
			want: "d7ff" + "00000003ffffffff",
		},
		{
			name: "seconds overflowing 34 bits use timestamp 96, not truncated",
			tm:   time.Unix(1<<34, 0).UTC(),
			want: "c70cff" + "00000000" + "0000000400000000",
		},
		{
			name: "negative seconds use timestamp 96, not wrapped and lost",
			tm:   time.Unix(-1, 0).UTC(),
			want: "c70cff" + "00000000" + "ffffffffffffffff",
		},
		{
			name: "negative seconds with nanoseconds use timestamp 96",
			tm:   time.Unix(-1234567890, 123456789).UTC(),
			want: "c70cff" + "075bcd15" + "ffffffffb669fd2e",
		},
		{
			name: "max legal nanoseconds with zero seconds use timestamp 64",
			tm:   time.Unix(0, 999999999).UTC(),
			want: "d7ff" + "ee6b27fc00000000",
		},
		{
			name: "max legal nanoseconds with max 34-bit seconds use timestamp 64",
			tm:   time.Unix(1<<34-1, 999999999).UTC(),
			want: "d7ff" + "ee6b27ffffffffff",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := msgpack.Marshal(tt.tm)
			require.NoError(t, err)
			require.Equal(t, tt.want, hex.EncodeToString(b),
				"timestamp layout mismatch for %v", tt.tm)
		})
	}
}

// TestTimestampDecodeVectors decodes hand-built timestamp extension bytes and
// checks the exact instant, including negative seconds and UTC normalization.
func TestTimestampDecodeVectors(t *testing.T) {
	tests := []struct {
		name string
		wire string
		want time.Time
	}{
		{
			name: "timestamp 32 zero",
			wire: "d6ff00000000",
			want: time.Unix(0, 0).UTC(),
		},
		{
			name: "timestamp 32 max",
			wire: "d6ffffffffff",
			want: time.Unix(0xffffffff, 0).UTC(),
		},
		{
			name: "timestamp 64 over 32 bits",
			wire: "d7ff0000000100000000",
			want: time.Unix(1<<32, 0).UTC(),
		},
		{
			name: "timestamp 64 max legal nanoseconds",
			wire: "d7ffee6b27fc00000000",
			want: time.Unix(0, 999999999).UTC(),
		},
		{
			name: "timestamp 96 negative seconds",
			wire: "c70cff00000000ffffffffffffffff",
			want: time.Unix(-1, 0).UTC(),
		},
		{
			name: "timestamp 96 negative seconds with nanoseconds",
			wire: "c70cff075bcd15ffffffffb669fd2e",
			want: time.Unix(-1234567890, 123456789).UTC(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := hex.DecodeString(tt.wire)
			require.NoError(t, err)

			var got time.Time
			require.NoError(t, msgpack.Unmarshal(data, &got))
			require.True(t, got.Equal(tt.want), "got %v, want %v", got, tt.want)
			require.Equal(t, time.UTC, got.Location(),
				"decoded timestamp must be UTC regardless of local timezone")
		})
	}
}

// TestTimestampDecodeRejectedVectors makes sure malformed timestamp extensions
// are rejected before any value is constructed: illegal nanoseconds, wrong
// extension lengths and unknown extension ids.
func TestTimestampDecodeRejectedVectors(t *testing.T) {
	tests := []struct {
		name string
		wire string
	}{
		{
			name: "timestamp 64 with nanoseconds 1000000000",
			wire: "d7ffee6b280000000000",
		},
		{
			name: "timestamp 96 with nanoseconds 1000000000",
			wire: "c70cff3b9aca0000000000000000",
		},
		{
			name: "fixext 1 body length",
			wire: "d4ff00",
		},
		{
			name: "fixext 2 body length",
			wire: "d5ff0000",
		},
		{
			name: "ext 8 with body length 3",
			wire: "c703ff000000",
		},
		{
			name: "ext 8 with body length 5",
			wire: "c705ff0000000000",
		},
		{
			name: "ext 8 with body length 13",
			wire: "c70dff" + "00000000000000000000000000",
		},
		{
			name: "fixext 16 body length",
			wire: "d8ff" + "00000000000000000000000000000000",
		},
		{
			name: "unknown extension id",
			wire: "d60000000000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := hex.DecodeString(tt.wire)
			require.NoError(t, err)

			var got time.Time
			err = msgpack.Unmarshal(data, &got)
			require.Error(t, err, "expected rejection for %s", tt.wire)
		})
	}
}

// TestTimestampEncodeDecodeTimeAPI exercises the EncodeTime/DecodeTime entry
// points and verifies the decoded instant is in UTC.
func TestTimestampEncodeDecodeTimeAPI(t *testing.T) {
	times := []time.Time{
		time.Unix(0, 0).UTC(),
		time.Unix(1, 0).UTC(),
		time.Unix(1<<32, 0).UTC(),
		time.Unix(1<<34, 0).UTC(),
		time.Unix(-1, 0).UTC(),
		time.Unix(-1234567890, 123456789).UTC(),
	}

	for _, tm := range times {
		var buf bytes.Buffer
		enc := msgpack.NewEncoder(&buf)
		require.NoError(t, enc.EncodeTime(tm))

		dec := msgpack.NewDecoder(&buf)
		got, err := dec.DecodeTime()
		require.NoError(t, err)
		require.True(t, got.Equal(tm), "got %v, want %v", got, tm)
		require.Equal(t, time.UTC, got.Location())
	}
}

// TestTimestampRoundTrip keeps the full range of representable instants,
// including negative seconds with nanoseconds, and checks the UTC instant is
// preserved independently of the local timezone.
func TestTimestampRoundTrip(t *testing.T) {
	times := []time.Time{
		time.Unix(0, 0),
		time.Unix(1, 1),
		time.Unix(0xffffffff, 999999999),
		time.Unix(1<<32, 1),
		time.Unix(1<<34-1, 999999999),
		time.Unix(1<<34, 0),
		time.Unix(-1, 1),
		time.Unix(-1234567890, 123456789),
		time.Unix(-1<<40, 999999999),
	}

	for _, tm := range times {
		b, err := msgpack.Marshal(tm)
		require.NoError(t, err)

		var got time.Time
		require.NoError(t, msgpack.Unmarshal(b, &got))
		require.True(t, got.Equal(tm), "round trip failed for %v", tm)
		require.Equal(t, time.UTC, got.Location())
	}
}
