package msgpack

import (
	"encoding/binary"
	"fmt"
	"time"
)

// MessagePack Timestamp extension type (-1), see
// https://github.com/msgpack/msgpack/blob/master/spec.md#timestamp-extension-type.
//
// The timestamp is stored as an absolute UTC instant made of signed seconds
// since the Unix epoch plus an unsigned nanosecond remainder. There are three
// fixed wire layouts:
//
//	timestamp32 (4 data bytes):
//	  uint32 seconds.  Only representable when nsec == 0 and 0 <= sec < 2^32.
//	timestamp64 (8 data bytes):
//	  top 30 bits uint32 nanoseconds, low 34 bits uint32 seconds.
//	  Only representable when 0 <= nsec < 2^30 (== 1e9) and 0 <= sec < 2^34.
//	timestamp96 (12 data bytes):
//	  uint32 nanoseconds followed by int64 seconds; covers negative seconds.
//
// Encoding always picks the shortest legal layout; decoding validates the data
// and builds the result with time.Unix(...).UTC(), so the recovered instant
// never depends on the machine's local time zone.
const (
	timestampLen32 = 4
	timestampLen64 = 8
	timestampLen96 = 12
)

const (
	// nanosecondsPerSecond is the number of nanoseconds in one second.
	nanosecondsPerSecond = 1_000_000_000
	// timestamp32MaxSec is the exclusive upper bound for 32-bit seconds.
	timestamp32MaxSec = 1 << 32
	// timestamp64MaxSec is the exclusive upper bound for the 34-bit
	// unsigned second field of the 64-bit layout.
	timestamp64MaxSec = 1 << 34
	// timestamp64NsecShift is the shift between the 30-bit nsec field and the
	// 34-bit second field in the 64-bit layout (both packing and unpacking).
	timestamp64NsecShift = 34
	// timestamp64SecMask keeps the low 34 seconds bits in timestamp64.
	timestamp64SecMask = timestamp64MaxSec - 1
)

var (
	errTimestampNanoseconds = fmt.Errorf(
		"msgpack: timestamp nanoseconds out of range [0, %d)", nanosecondsPerSecond)
	errTimestampLen = fmt.Errorf(
		"msgpack: invalid timestamp ext length")
)

// marshalTimestamp is the strict timestamp32/64/96 encoder. Inputs are
// validated before any bytes are emitted: illegal nanoseconds are rejected and
// seconds that overflow the current layout promote the encoding to the next
// layout instead of being silently truncated. The shortest legal layout is
// chosen, and negative seconds stay signed, so they use the 96-bit layout
// rather than being converted to an unsigned value.
func marshalTimestamp(sec int64, nsec int64) ([]byte, error) {
	if uint64(nsec) >= nanosecondsPerSecond {
		return nil, errTimestampNanoseconds
	}

	switch {
	// timestamp32: nsec must be zero and seconds must fit uint32.
	case nsec == 0 && uint64(sec) < timestamp32MaxSec:
		b := make([]byte, timestampLen32)
		binary.BigEndian.PutUint32(b, uint32(sec))
		return b, nil
	// timestamp64: non-negative seconds must fit the unsigned 34-bit field.
	case uint64(sec) < timestamp64MaxSec:
		b := make([]byte, timestampLen64)
		binary.BigEndian.PutUint64(b, uint64(nsec)<<timestamp64NsecShift|uint64(sec))
		return b, nil
	}

	// timestamp96: uint32 nanoseconds followed by signed int64 seconds; the
	// only legal form for negative or >34-bit seconds.
	b := make([]byte, timestampLen96)
	binary.BigEndian.PutUint32(b, uint32(nsec))
	binary.BigEndian.PutUint64(b[timestampLen32:], uint64(sec))
	return b, nil
}

// unmarshalTimestamp is the strict timestamp32/64/96 decoder. Wrong extension
// lengths and illegal nanosecond fields are rejected, and the returned time is
// anchored to UTC so decoding is independent of the local time zone.
func unmarshalTimestamp(data []byte) (time.Time, error) {
	switch len(data) {
	case timestampLen32:
		sec := binary.BigEndian.Uint32(data)
		return time.Unix(int64(sec), 0).UTC(), nil
	case timestampLen64:
		v := binary.BigEndian.Uint64(data)
		nsec := int64(v >> timestamp64NsecShift)
		if uint64(nsec) >= nanosecondsPerSecond {
			return time.Time{}, errTimestampNanoseconds
		}
		sec := int64(v & timestamp64SecMask)
		return time.Unix(sec, nsec).UTC(), nil
	case timestampLen96:
		nsec := int64(binary.BigEndian.Uint32(data))
		if uint64(nsec) >= nanosecondsPerSecond {
			return time.Time{}, errTimestampNanoseconds
		}
		sec := int64(binary.BigEndian.Uint64(data[timestampLen32:]))
		return time.Unix(sec, nsec).UTC(), nil
	default:
		return time.Time{}, fmt.Errorf("%w: got %d bytes", errTimestampLen, len(data))
	}
}
