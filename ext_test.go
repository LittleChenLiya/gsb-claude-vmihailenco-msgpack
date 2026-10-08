package msgpack_test

import (
	"bytes"
	"encoding/hex"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
	"github.com/vmihailenco/msgpack/v5/msgpcode"
)

func init() {
	msgpack.RegisterExt(9, (*ExtTest)(nil))
}

type ExtTest struct {
	S string
}

var (
	_ msgpack.Marshaler   = (*ExtTest)(nil)
	_ msgpack.Unmarshaler = (*ExtTest)(nil)
)

func (ext ExtTest) MarshalMsgpack() ([]byte, error) {
	return msgpack.Marshal("hello " + ext.S)
}

func (ext *ExtTest) UnmarshalMsgpack(b []byte) error {
	return msgpack.Unmarshal(b, &ext.S)
}

func TestEncodeDecodeExtHeader(t *testing.T) {
	v := &ExtTest{"world"}

	payload, err := v.MarshalMsgpack()
	require.Nil(t, err)

	var buf bytes.Buffer
	enc := msgpack.NewEncoder(&buf)
	err = enc.EncodeExtHeader(9, len(payload))
	require.Nil(t, err)

	_, err = buf.Write(payload)
	require.Nil(t, err)

	var dst interface{}
	err = msgpack.Unmarshal(buf.Bytes(), &dst)
	require.Nil(t, err)

	v = dst.(*ExtTest)
	wanted := "hello world"
	require.Equal(t, v.S, wanted)

	dec := msgpack.NewDecoder(&buf)
	extID, extLen, err := dec.DecodeExtHeader()
	require.Nil(t, err)
	require.Equal(t, int8(9), extID)
	require.Equal(t, len(payload), extLen)

	data := make([]byte, extLen)
	err = dec.ReadFull(data)
	require.Nil(t, err)

	v = &ExtTest{}
	err = v.UnmarshalMsgpack(data)
	require.Nil(t, err)
	require.Equal(t, wanted, v.S)
}

func TestExt(t *testing.T) {
	v := &ExtTest{"world"}
	b, err := msgpack.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}

	var dst interface{}
	err = msgpack.Unmarshal(b, &dst)
	if err != nil {
		t.Fatal(err)
	}

	v, ok := dst.(*ExtTest)
	if !ok {
		t.Fatalf("got %#v, wanted ExtTest", dst)
	}

	wanted := "hello world"
	if v.S != wanted {
		t.Fatalf("got %q, wanted %q", v.S, wanted)
	}

	ext := new(ExtTest)
	err = msgpack.Unmarshal(b, &ext)
	if err != nil {
		t.Fatal(err)
	}
	if ext.S != wanted {
		t.Fatalf("got %q, wanted %q", ext.S, wanted)
	}
}

func TestUnknownExt(t *testing.T) {
	b := []byte{byte(msgpcode.FixExt1), 2, 0}

	var dst interface{}
	err := msgpack.Unmarshal(b, &dst)
	if err == nil {
		t.Fatalf("got nil, wanted error")
	}
	got := err.Error()
	wanted := "msgpack: unknown ext id=2"
	if got != wanted {
		t.Fatalf("got %q, wanted %q", got, wanted)
	}
}

const (
	passthroughUnknownExtID  int8 = 42
	passthroughRegisteredID  int8 = 43
	passthroughRegisteredID2 int8 = 44
)

// passthroughExt is registered (and unregistered) only for the duration of the
// passthrough tests so it never leaks into the global ext registry.
type passthroughExt struct {
	S string
}

func writeRaw(t *testing.T, w io.Writer, b []byte) {
	t.Helper()
	_, err := w.Write(b)
	require.NoError(t, err)
}

func (ext passthroughExt) MarshalMsgpack() ([]byte, error) {
	return msgpack.Marshal("reg:" + ext.S)
}

func (ext *passthroughExt) UnmarshalMsgpack(b []byte) error {
	return msgpack.Unmarshal(b, &ext.S)
}

// buildExtStream encodes a registered ext, two unknown exts (FixExt1 and an
// Ext8 with a longer body) and a trailing ordinary value in one buffer.
func buildExtStream(t *testing.T) []byte {
	t.Helper()

	msgpack.RegisterExt(passthroughRegisteredID, (*passthroughExt)(nil))
	t.Cleanup(func() {
		msgpack.UnregisterExt(passthroughRegisteredID)
		msgpack.UnregisterExt(passthroughRegisteredID2)
	})

	var buf bytes.Buffer
	enc := msgpack.NewEncoder(&buf)

	// Known ext -> must always use the registered decoder.
	require.NoError(t, enc.Encode(&passthroughExt{S: "known"}))

	// Unknown ext type 42, 1-byte payload (FixExt1).
	require.NoError(t, enc.EncodeExtHeader(passthroughUnknownExtID, 1))
	writeRaw(t, enc.Writer(), []byte{0xAA})

	// Unknown ext type 42 again, 5-byte payload (Ext8).
	require.NoError(t, enc.EncodeExtHeader(passthroughUnknownExtID, 5))
	writeRaw(t, enc.Writer(), []byte{1, 2, 3, 4, 5})

	// Trailing ordinary value to prove the read position is advanced exactly
	// past the ext payloads.
	require.NoError(t, enc.EncodeString("tail"))

	return buf.Bytes()
}

// With the option disabled unknown exts keep producing the historical error,
// i.e. the old interface works unchanged.
func TestUnknownExtPassthroughDisabled(t *testing.T) {
	data := buildExtStream(t)

	dec := msgpack.NewDecoder(bytes.NewReader(data))

	// First value is the registered ext and decodes normally.
	v, err := dec.DecodeInterface()
	require.NoError(t, err)
	reg, ok := v.(*passthroughExt)
	require.True(t, ok, "got %T", v)
	require.Equal(t, "reg:known", reg.S)

	_, err = dec.DecodeInterface()
	require.EqualError(t, err, "msgpack: unknown ext id=42")
}

// With the option enabled unknown exts are returned as UnknownExt carrying the
// original type id and an independent copy of the payload; registered exts
// still use their decoder; the reader position is preserved.
func TestUnknownExtPassthroughEnabled(t *testing.T) {
	data := buildExtStream(t)

	dec := msgpack.NewDecoder(bytes.NewReader(data))
	dec.UseUnknownExtPassthrough(true)

	// Registered ext takes priority over the passthrough strategy.
	v, err := dec.DecodeInterface()
	require.NoError(t, err)
	reg, ok := v.(*passthroughExt)
	require.True(t, ok, "got %T", v)
	require.Equal(t, "reg:known", reg.S)

	// First unknown ext: FixExt1.
	v, err = dec.DecodeInterface()
	require.NoError(t, err)
	first, ok := v.(msgpack.UnknownExt)
	require.True(t, ok, "got %T", v)
	require.Equal(t, passthroughUnknownExtID, first.Type)
	require.Equal(t, []byte{0xAA}, first.Payload)

	// Second unknown ext: Ext8 with a longer body. Decoding it reuses the
	// decoder scratch buffer; the first payload must be unaffected (ownership).
	v, err = dec.DecodeInterface()
	require.NoError(t, err)
	second, ok := v.(msgpack.UnknownExt)
	require.True(t, ok, "got %T", v)
	require.Equal(t, passthroughUnknownExtID, second.Type)
	require.Equal(t, []byte{1, 2, 3, 4, 5}, second.Payload)
	require.Equal(t, []byte{0xAA}, first.Payload, "first payload was mutated")

	// The trailing ordinary value is read from the correct position.
	tail, err := dec.DecodeInterface()
	require.NoError(t, err)
	require.Equal(t, "tail", tail)

	// Mutating a returned payload is an independent copy: re-decoding the
	// same wire bytes still yields the original payload.
	second.Payload[0] = 0xFF
	fresh := decodeUnknownPayloadAt(t, data, 1)
	require.Equal(t, []byte{1, 2, 3, 4, 5}, fresh.Payload)
}

// Unknown ext inside an array proves read-position bookkeeping inside
// containers, and that the loose path works too.
func TestUnknownExtPassthroughInArrayLoose(t *testing.T) {
	buildExtStream(t) // register the cleanup for the helper-registered ids

	var buf bytes.Buffer
	enc := msgpack.NewEncoder(&buf)
	require.NoError(t, enc.EncodeArrayLen(2))
	require.NoError(t, enc.EncodeExtHeader(passthroughUnknownExtID, 2))
	writeRaw(t, enc.Writer(), []byte{0xCD, 0xEF})
	require.NoError(t, enc.EncodeInt(7))

	dec := msgpack.NewDecoder(bytes.NewReader(buf.Bytes()))
	dec.UseUnknownExtPassthrough(true)
	dec.UseLooseInterfaceDecoding(true)

	arr, err := dec.DecodeInterface()
	require.NoError(t, err)
	items, ok := arr.([]interface{})
	require.True(t, ok, "got %T", arr)
	require.Len(t, items, 2)

	ext, ok := items[0].(msgpack.UnknownExt)
	require.True(t, ok, "got %T", items[0])
	require.Equal(t, passthroughUnknownExtID, ext.Type)
	require.Equal(t, []byte{0xCD, 0xEF}, ext.Payload)
	require.Equal(t, int64(7), items[1])
}

// UnknownExt round-trips: re-encoding the returned value reproduces the same
// wire bytes.
func TestUnknownExtRoundTrip(t *testing.T) {
	data := buildExtStream(t)

	dec := msgpack.NewDecoder(bytes.NewReader(data))
	dec.UseUnknownExtPassthrough(true)

	_, err := dec.DecodeInterface() // registered ext
	require.NoError(t, err)

	raw, err := dec.DecodeInterface()
	require.NoError(t, err)
	ext := raw.(msgpack.UnknownExt)

	var buf bytes.Buffer
	require.NoError(t, msgpack.NewEncoder(&buf).Encode(&ext))
	require.Equal(t,
		[]byte{byte(msgpcode.FixExt1), byte(passthroughUnknownExtID), 0xAA},
		buf.Bytes())
}

// Registering a decoder for an id after a value was returned never changes the
// already returned UnknownExt (snapshot semantics).
func TestUnknownExtNotAffectedByLaterRegistration(t *testing.T) {
	var buf bytes.Buffer
	enc := msgpack.NewEncoder(&buf)
	require.NoError(t, enc.EncodeExtHeader(passthroughRegisteredID2, 1))
	writeRaw(t, enc.Writer(), []byte{0x01})

	dec := msgpack.NewDecoder(bytes.NewReader(buf.Bytes()))
	dec.UseUnknownExtPassthrough(true)

	v, err := dec.DecodeInterface()
	require.NoError(t, err)
	ext, ok := v.(msgpack.UnknownExt)
	require.True(t, ok, "got %T", v)
	require.Equal(t, passthroughRegisteredID2, ext.Type)
	require.Equal(t, []byte{1}, ext.Payload)

	// Registering a decoder afterwards must not retroactively change ext.
	msgpack.RegisterExt(passthroughRegisteredID2, (*passthroughExt)(nil))
	defer msgpack.UnregisterExt(passthroughRegisteredID2)

	require.Equal(t, passthroughRegisteredID2, ext.Type)
	require.Equal(t, []byte{1}, ext.Payload)
}

// Turning the option back off restores the default error behavior; Reset also
// clears it.
func TestUnknownExtPassthroughToggleAndReset(t *testing.T) {
	buildExtStream(t)

	b := []byte{byte(msgpcode.FixExt1), byte(passthroughUnknownExtID), 0}

	dec := msgpack.NewDecoder(bytes.NewReader(b))
	dec.UseUnknownExtPassthrough(true)
	v, err := dec.DecodeInterface()
	require.NoError(t, err)
	_, ok := v.(msgpack.UnknownExt)
	require.True(t, ok)

	dec.UseUnknownExtPassthrough(false)
	dec.Reset(bytes.NewReader(b))
	_, err = dec.DecodeInterface()
	require.EqualError(t, err, "msgpack: unknown ext id=42")
}

// decodeUnknownPayloadAt decodes the stream with passthrough enabled and
// returns the unknown ext at the given 0-based position among unknown exts.
func decodeUnknownPayloadAt(t *testing.T, data []byte, idx int) msgpack.UnknownExt {
	t.Helper()
	dec := msgpack.NewDecoder(bytes.NewReader(data))
	dec.UseUnknownExtPassthrough(true)

	seen := 0
	for {
		v, err := dec.DecodeInterface()
		require.NoError(t, err)
		ext, ok := v.(msgpack.UnknownExt)
		if !ok {
			continue // registered exts / ordinary values do not count
		}
		if seen == idx {
			return ext
		}
		seen++
	}
}

func TestSliceOfTime(t *testing.T) {
	in := []interface{}{time.Now()}
	b, err := msgpack.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}

	var out []interface{}
	err = msgpack.Unmarshal(b, &out)
	if err != nil {
		t.Fatal(err)
	}

	outTime := out[0].(time.Time)
	inTime := in[0].(time.Time)
	if outTime.Unix() != inTime.Unix() {
		t.Fatalf("got %v, wanted %v", outTime, inTime)
	}
}

type customPayload struct {
	payload []byte
}

func (cp *customPayload) MarshalMsgpack() ([]byte, error) {
	return cp.payload, nil
}

func (cp *customPayload) UnmarshalMsgpack(b []byte) error {
	cp.payload = b
	return nil
}

func TestDecodeCustomPayload(t *testing.T) {
	b, err := hex.DecodeString("c70500c09eec3100")
	if err != nil {
		t.Fatal(err)
	}

	msgpack.RegisterExt(0, (*customPayload)(nil))

	var cp *customPayload
	err = msgpack.Unmarshal(b, &cp)
	if err != nil {
		t.Fatal(err)
	}

	payload := hex.EncodeToString(cp.payload)
	wanted := "c09eec3100"
	if payload != wanted {
		t.Fatalf("got %q, wanted %q", payload, wanted)
	}
}
