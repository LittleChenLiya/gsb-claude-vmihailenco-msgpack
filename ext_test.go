package msgpack_test

import (
	"bytes"
	"encoding/hex"
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

type rawExtLate struct {
	payload []byte
}

func (e *rawExtLate) MarshalMsgpack() ([]byte, error) { return e.payload, nil }
func (e *rawExtLate) UnmarshalMsgpack(b []byte) error {
	e.payload = append([]byte(nil), b...)
	return nil
}

func TestRawExtForUnknownExt(t *testing.T) {
	const unknownExtID = 42

	knownPayload, err := (&ExtTest{"world"}).MarshalMsgpack()
	require.NoError(t, err)
	unknownPayload := []byte{0xde, 0xad, 0xbe, 0xef}

	// Encode: known ext (id=9, registered), unknown ext (id=42), plain value.
	var buf bytes.Buffer
	enc := msgpack.NewEncoder(&buf)
	require.NoError(t, enc.EncodeExtHeader(9, len(knownPayload)))
	_, err = buf.Write(knownPayload)
	require.NoError(t, err)
	require.NoError(t, enc.EncodeExtHeader(unknownExtID, len(unknownPayload)))
	_, err = buf.Write(unknownPayload)
	require.NoError(t, err)
	require.NoError(t, enc.Encode("tail"))

	// Default behavior is unchanged: unknown ext id is an error.
	var dst interface{}
	err = msgpack.Unmarshal(buf.Bytes(), &dst)
	require.NoError(t, err) // first value is the registered ext
	decode := func(dec *msgpack.Decoder) interface{} {
		var v interface{}
		require.NoError(t, dec.Decode(&v))
		return v
	}
	dec := msgpack.NewDecoder(bytes.NewReader(buf.Bytes()))
	_ = decode(dec) // known ext
	var v interface{}
	err = dec.Decode(&v)
	require.EqualError(t, err, "msgpack: unknown ext id=42")

	// With the option enabled the unknown ext is returned as-is.
	dec = msgpack.NewDecoder(bytes.NewReader(buf.Bytes()))
	dec.UseRawExtForUnknownExt(true)

	// Registered extensions still take priority and use their decoder.
	require.Equal(t, &ExtTest{S: "hello world"}, decode(dec))

	// Unknown ext: type code and payload copy are preserved.
	raw, ok := decode(dec).(*msgpack.RawExt)
	require.True(t, ok)
	require.Equal(t, int8(unknownExtID), raw.Type)
	require.Equal(t, unknownPayload, raw.Data)

	// Read position is intact: the following plain value decodes normally.
	require.Equal(t, "tail", decode(dec))

	// Payload ownership: decoding more values must not clobber the copy.
	dec = msgpack.NewDecoder(bytes.NewReader(buf.Bytes()))
	dec.UseRawExtForUnknownExt(true)
	_ = decode(dec) // known ext
	raw = decode(dec).(*msgpack.RawExt)
	require.Equal(t, "tail", decode(dec)) // reuses the decoder buffer
	require.Equal(t, unknownPayload, raw.Data)

	// Later registration changes do not affect already returned values,
	// but take priority for new decodes.
	msgpack.RegisterExt(unknownExtID, (*rawExtLate)(nil))
	defer msgpack.UnregisterExt(unknownExtID)
	require.Equal(t, int8(unknownExtID), raw.Type)
	require.Equal(t, unknownPayload, raw.Data)

	dec = msgpack.NewDecoder(bytes.NewReader(buf.Bytes()))
	dec.UseRawExtForUnknownExt(true)
	_ = decode(dec) // known ext
	require.Equal(t, &rawExtLate{payload: unknownPayload}, decode(dec))
}
