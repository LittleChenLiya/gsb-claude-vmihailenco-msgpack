package msgpack

import (
	"encoding/binary"
	"fmt"
	"reflect"
	"time"

	"github.com/vmihailenco/msgpack/v5/msgpcode"
)

// timeExtID is the MessagePack timestamp extension type id (-1), as defined
// by the msgpack timestamp extension spec:
// https://github.com/msgpack/msgpack/blob/master/spec.md#timestamp-extension-type
var timeExtID int8 = -1

// Timestamp extension body layouts. A timestamp extension always carries the
// type id -1; the body length selects the layout:
//
//   - timestamp 32 (fixext 4, 4 body bytes):
//     seconds since 1970-01-01 00:00:00 UTC as an unsigned 32-bit big-endian
//     integer. Nanoseconds are not representable and must be zero.
//     Body: [sec32 BE]
//
//   - timestamp 64 (fixext 8, 8 body bytes):
//     the top 30 bits hold the unsigned nanoseconds (0..999_999_999) and the
//     low 34 bits hold the unsigned seconds. Both fields are big-endian.
//     Body: [nsec30 | sec34 BE]
//
//   - timestamp 96 (ext 8/16/32, 12 body bytes):
//     an unsigned 32-bit big-endian nanoseconds field followed by a signed
//     64-bit big-endian seconds field (two's complement), so seconds before
//     the Unix epoch are preserved.
//     Body: [nsec32 BE][sec64 signed BE]
const (
	timestamp32Len = 4
	timestamp64Len = 8
	timestamp96Len = 12
)

// maxTimestampNanoseconds is the only legal value range for the nanoseconds
// field of any timestamp layout.
const maxTimestampNanoseconds = 999_999_999

func init() {
	RegisterExtEncoder(timeExtID, time.Time{}, timeEncoder)
	RegisterExtDecoder(timeExtID, time.Time{}, timeDecoder)
}

func timeEncoder(e *Encoder, v reflect.Value) ([]byte, error) {
	return e.encodeTime(v.Interface().(time.Time))
}

func timeDecoder(d *Decoder, v reflect.Value, extLen int) error {
	tm, err := d.decodeTime(extLen)
	if err != nil {
		return err
	}

	ptr := v.Addr().Interface().(*time.Time)
	*ptr = tm

	return nil
}

func (e *Encoder) EncodeTime(tm time.Time) error {
	b, err := e.encodeTime(tm)
	if err != nil {
		return err
	}
	if err := e.encodeExtLen(len(b)); err != nil {
		return err
	}
	if err := e.w.WriteByte(byte(timeExtID)); err != nil {
		return err
	}
	return e.write(b)
}

// encodeTime encodes tm as a timestamp extension body, choosing the shortest
// legal layout (see encodeTimestamp).
func (e *Encoder) encodeTime(tm time.Time) ([]byte, error) {
	if e.timeBuf == nil {
		e.timeBuf = make([]byte, timestamp96Len)
	}
	return encodeTimestamp(e.timeBuf, tm.Unix(), uint32(tm.Nanosecond()))
}

// encodeTimestamp writes the seconds/nanoseconds pair into the shortest legal
// msgpack timestamp layout using buf as scratch space (len(buf) must be at
// least 12).
//
// The layout is selected by explicit range checks, not by narrowing the
// values: seconds that do not fit a narrower layout cause that layout to be
// skipped, so no value is ever silently truncated. In particular negative
// seconds are kept as signed values and encoded with their two's complement
// bit pattern in the timestamp 96 layout instead of being wrapped to an
// unsigned value and lost.
//
// Nanoseconds outside [0, 999_999_999] are rejected before any byte is
// written.
func encodeTimestamp(buf []byte, sec int64, nsec uint32) ([]byte, error) {
	if nsec > maxTimestampNanoseconds {
		return nil, fmt.Errorf("msgpack: invalid timestamp nanoseconds=%d", nsec)
	}

	switch {
	case nsec == 0 && sec >= 0 && sec < (1<<32):
		// timestamp 32: unsigned 32-bit seconds, no nanoseconds.
		b := buf[:timestamp32Len]
		binary.BigEndian.PutUint32(b, uint32(sec))
		return b, nil
	case sec >= 0 && sec < (1<<34):
		// timestamp 64: 30-bit nanoseconds on top of 34-bit seconds.
		b := buf[:timestamp64Len]
		binary.BigEndian.PutUint64(b, uint64(nsec)<<34|uint64(sec))
		return b, nil
	default:
		// timestamp 96: unsigned 32-bit nanoseconds followed by signed
		// 64-bit seconds. The uint64 conversion keeps the two's complement
		// bit pattern, which is exactly how the spec encodes signed
		// seconds, so negative seconds round-trip.
		b := buf[:timestamp96Len]
		binary.BigEndian.PutUint32(b, nsec)
		binary.BigEndian.PutUint64(b[4:], uint64(sec))
		return b, nil
	}
}

func (d *Decoder) DecodeTime() (time.Time, error) {
	c, err := d.readCode()
	if err != nil {
		return time.Time{}, err
	}

	// Legacy format.
	if c == msgpcode.FixedArrayLow|2 {
		sec, err := d.DecodeInt64()
		if err != nil {
			return time.Time{}, err
		}

		nsec, err := d.DecodeInt64()
		if err != nil {
			return time.Time{}, err
		}

		return time.Unix(sec, nsec).UTC(), nil
	}

	if msgpcode.IsString(c) {
		s, err := d.string(c)
		if err != nil {
			return time.Time{}, err
		}

		tm, err := time.Parse(time.RFC3339Nano, s)
		if err != nil {
			return time.Time{}, err
		}
		return tm.UTC(), nil
	}

	extID, extLen, err := d.extHeader(c)
	if err != nil {
		return time.Time{}, err
	}

	// NodeJS seems to use extID 13.
	if extID != timeExtID && extID != 13 {
		return time.Time{}, fmt.Errorf("msgpack: invalid time ext id=%d", extID)
	}

	return d.decodeTime(extLen)
}

// decodeTime reads a timestamp extension body of extLen bytes and decodes it
// with the strict codec. The returned time is normalized to UTC so the
// decoded instant does not depend on the host timezone.
func (d *Decoder) decodeTime(extLen int) (time.Time, error) {
	b, err := d.readN(extLen)
	if err != nil {
		return time.Time{}, err
	}
	return decodeTimestamp(b)
}

// decodeTimestamp decodes a timestamp extension body.
//
// Only body lengths of 4, 8 and 12 are legal; any other length is rejected
// before decoding. Nanoseconds outside [0, 999_999_999] are rejected instead
// of being normalized into extra seconds. Seconds in the 96-bit layout are
// read back as signed two's complement values, so negative seconds are
// restored. The result is returned in UTC, preserving the decoded instant
// independently of the local timezone.
func decodeTimestamp(b []byte) (time.Time, error) {
	switch len(b) {
	case timestamp32Len:
		// timestamp 32: unsigned 32-bit seconds, no nanoseconds.
		sec := binary.BigEndian.Uint32(b)
		return time.Unix(int64(sec), 0).UTC(), nil
	case timestamp64Len:
		// timestamp 64: 30-bit nanoseconds followed by 34-bit seconds.
		v := binary.BigEndian.Uint64(b)
		nsec := int64(v >> 34)
		if nsec > maxTimestampNanoseconds {
			return time.Time{}, fmt.Errorf("msgpack: invalid timestamp nanoseconds=%d", nsec)
		}
		sec := int64(v & 0x00000003ffffffff)
		return time.Unix(sec, nsec).UTC(), nil
	case timestamp96Len:
		// timestamp 96: unsigned 32-bit nanoseconds followed by signed
		// 64-bit seconds. Converting the unsigned 64-bit value back to
		// int64 restores the two's complement representation of negative
		// seconds without going through a lossy unsigned conversion.
		nsec := binary.BigEndian.Uint32(b)
		if nsec > maxTimestampNanoseconds {
			return time.Time{}, fmt.Errorf("msgpack: invalid timestamp nanoseconds=%d", nsec)
		}
		sec := int64(binary.BigEndian.Uint64(b[4:]))
		return time.Unix(sec, int64(nsec)).UTC(), nil
	default:
		return time.Time{}, fmt.Errorf("msgpack: invalid timestamp extension length=%d", len(b))
	}
}
