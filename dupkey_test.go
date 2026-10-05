package msgpack_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vmihailenco/msgpack/v5"
)

// encodeMapPayload encodes key/value pairs as a single msgpack map.
// Duplicate keys are preserved as-is.
func encodeMapPayload(t *testing.T, pairs ...interface{}) []byte {
	t.Helper()
	require.Zero(t, len(pairs)%2, "pairs must be even")

	var buf bytes.Buffer
	enc := msgpack.NewEncoder(&buf)
	require.NoError(t, enc.EncodeMapLen(len(pairs)/2))
	for i := 0; i < len(pairs); i += 2 {
		require.NoError(t, enc.Encode(pairs[i]))
		require.NoError(t, enc.Encode(pairs[i+1]))
	}
	return buf.Bytes()
}

func decodeWithPolicy(t *testing.T, policy msgpack.DuplicateKeyPolicy, payload []byte, v interface{}) error {
	t.Helper()
	dec := msgpack.NewDecoder(bytes.NewReader(payload))
	dec.SetDuplicateKeyPolicy(policy)
	return dec.Decode(v)
}

type dupKeyStruct struct {
	A int `msgpack:"a"`
	Z int `msgpack:"z"`
}

func TestDuplicateKeyStructDefaultKeepsLast(t *testing.T) {
	payload := encodeMapPayload(t, "a", 1, "a", 2, "z", 3)

	// Default Decoder keeps the historical behavior: last value wins.
	var s dupKeyStruct
	require.NoError(t, msgpack.Unmarshal(payload, &s))
	require.Equal(t, 2, s.A)
	require.Equal(t, 3, s.Z)

	// Explicit DuplicateKeyLast behaves the same.
	s = dupKeyStruct{}
	require.NoError(t, decodeWithPolicy(t, msgpack.DuplicateKeyLast, payload, &s))
	require.Equal(t, 2, s.A)
	require.Equal(t, 3, s.Z)
}

func TestDuplicateKeyStructFirstReadsFollowingField(t *testing.T) {
	payload := encodeMapPayload(t, "a", 1, "a", 2, "z", 3)

	var s dupKeyStruct
	require.NoError(t, decodeWithPolicy(t, msgpack.DuplicateKeyFirst, payload, &s))
	require.Equal(t, 1, s.A)
	// The skipped duplicate value must be fully consumed so that the
	// following field is still decoded.
	require.Equal(t, 3, s.Z)
}

func TestDuplicateKeyStructReject(t *testing.T) {
	payload := encodeMapPayload(t, "a", 1, "a", 2, "z", 3)

	var s dupKeyStruct
	err := decodeWithPolicy(t, msgpack.DuplicateKeyReject, payload, &s)
	require.Error(t, err)
	require.Contains(t, err.Error(), `duplicate field "a"`)
}

type dupKeyAliasStruct struct {
	A int `msgpack:"a,alias:b"`
	Z int `msgpack:"z"`
}

func TestDuplicateKeyStructAliasRejected(t *testing.T) {
	// "a" and its alias "b" resolve to the same struct field.
	payload := encodeMapPayload(t, "a", 1, "b", 2, "z", 3)

	var s dupKeyAliasStruct
	err := decodeWithPolicy(t, msgpack.DuplicateKeyReject, payload, &s)
	require.Error(t, err)
	require.Contains(t, err.Error(), `duplicate field "b"`)

	// First-value policy keeps the value decoded through the primary name.
	s = dupKeyAliasStruct{}
	require.NoError(t, decodeWithPolicy(t, msgpack.DuplicateKeyFirst, payload, &s))
	require.Equal(t, 1, s.A)
	require.Equal(t, 3, s.Z)
}

func TestDuplicateKeyFirstSkipsNestedContainer(t *testing.T) {
	type nested struct {
		A map[string]int `msgpack:"a"`
		Z int            `msgpack:"z"`
	}
	inner1 := encodeMapPayload(t, "x", 1)
	inner2 := encodeMapPayload(t, "y", 2)

	var buf bytes.Buffer
	enc := msgpack.NewEncoder(&buf)
	require.NoError(t, enc.EncodeMapLen(3))
	require.NoError(t, enc.Encode("a"))
	_, err := buf.Write(inner1)
	require.NoError(t, err)
	require.NoError(t, enc.Encode("a"))
	_, err = buf.Write(inner2)
	require.NoError(t, err)
	require.NoError(t, enc.Encode("z"))
	require.NoError(t, enc.Encode(3))

	var s nested
	require.NoError(t, decodeWithPolicy(t, msgpack.DuplicateKeyFirst, buf.Bytes(), &s))
	require.Equal(t, map[string]int{"x": 1}, s.A)
	require.Equal(t, 3, s.Z)
}

func TestDuplicateKeyNestedScopesAreIndependent(t *testing.T) {
	type outer struct {
		M map[string]int `msgpack:"m"`
		A int            `msgpack:"a"`
	}
	inner := encodeMapPayload(t, "a", 1)

	var buf bytes.Buffer
	enc := msgpack.NewEncoder(&buf)
	require.NoError(t, enc.EncodeMapLen(2))
	require.NoError(t, enc.Encode("m"))
	_, err := buf.Write(inner)
	require.NoError(t, err)
	require.NoError(t, enc.Encode("a"))
	require.NoError(t, enc.Encode(2))

	// "a" appears once in the nested map and once in the outer struct:
	// different scopes, no duplicate.
	var s outer
	require.NoError(t, decodeWithPolicy(t, msgpack.DuplicateKeyReject, buf.Bytes(), &s))
	require.Equal(t, map[string]int{"a": 1}, s.M)
	require.Equal(t, 2, s.A)

	// But a duplicate inside the nested map itself is rejected.
	innerDup := encodeMapPayload(t, "a", 1, "a", 2)
	buf.Reset()
	require.NoError(t, enc.EncodeMapLen(2))
	require.NoError(t, enc.Encode("m"))
	_, err = buf.Write(innerDup)
	require.NoError(t, err)
	require.NoError(t, enc.Encode("a"))
	require.NoError(t, enc.Encode(2))

	s = outer{}
	err = decodeWithPolicy(t, msgpack.DuplicateKeyReject, buf.Bytes(), &s)
	require.Error(t, err)
	require.Contains(t, err.Error(), `duplicate map key`)
}

func TestDuplicateKeyTypedMap(t *testing.T) {
	payload := encodeMapPayload(t, "a", 1, "a", 2, "z", 3)

	var m map[string]int
	require.NoError(t, decodeWithPolicy(t, msgpack.DuplicateKeyFirst, payload, &m))
	require.Equal(t, map[string]int{"a": 1, "z": 3}, m)

	m = nil
	require.NoError(t, decodeWithPolicy(t, msgpack.DuplicateKeyLast, payload, &m))
	require.Equal(t, map[string]int{"a": 2, "z": 3}, m)

	m = nil
	err := decodeWithPolicy(t, msgpack.DuplicateKeyReject, payload, &m)
	require.Error(t, err)
	require.Contains(t, err.Error(), `duplicate map key "a"`)
}

func TestDuplicateKeyMapStringString(t *testing.T) {
	payload := encodeMapPayload(t, "a", "1", "a", "2", "z", "3")

	var m map[string]string
	require.NoError(t, decodeWithPolicy(t, msgpack.DuplicateKeyFirst, payload, &m))
	require.Equal(t, map[string]string{"a": "1", "z": "3"}, m)

	m = nil
	err := decodeWithPolicy(t, msgpack.DuplicateKeyReject, payload, &m)
	require.Error(t, err)
	require.Contains(t, err.Error(), `duplicate map key "a"`)
}

func TestDuplicateKeyMapStringInterface(t *testing.T) {
	payload := encodeMapPayload(t, "a", 1, "a", 2, "z", 3)

	dec := msgpack.NewDecoder(bytes.NewReader(payload))
	dec.SetDuplicateKeyPolicy(msgpack.DuplicateKeyFirst)
	m, err := dec.DecodeMap()
	require.NoError(t, err)
	require.Equal(t, map[string]interface{}{"a": int8(1), "z": int8(3)}, m)

	dec = msgpack.NewDecoder(bytes.NewReader(payload))
	dec.SetDuplicateKeyPolicy(msgpack.DuplicateKeyReject)
	m, err = dec.DecodeMap()
	require.Error(t, err)
	require.Nil(t, m) // strict failure does not return a half-decoded map
}

func TestDuplicateKeyUntypedMap(t *testing.T) {
	payload := encodeMapPayload(t, "a", 1, "a", 2, "z", 3)

	dec := msgpack.NewDecoder(bytes.NewReader(payload))
	dec.SetDuplicateKeyPolicy(msgpack.DuplicateKeyFirst)
	m, err := dec.DecodeUntypedMap()
	require.NoError(t, err)
	require.Equal(t, map[interface{}]interface{}{"a": int8(1), "z": int8(3)}, m)

	dec = msgpack.NewDecoder(bytes.NewReader(payload))
	dec.SetDuplicateKeyPolicy(msgpack.DuplicateKeyReject)
	m, err = dec.DecodeUntypedMap()
	require.Error(t, err)
	require.Nil(t, m)
}

func TestDuplicateKeyTypedMapInterface(t *testing.T) {
	payload := encodeMapPayload(t, "a", 1, "a", 2, "z", 3)

	dec := msgpack.NewDecoder(bytes.NewReader(payload))
	dec.SetDuplicateKeyPolicy(msgpack.DuplicateKeyFirst)
	v, err := dec.DecodeTypedMap()
	require.NoError(t, err)
	require.Equal(t, map[string]int8{"a": 1, "z": 3}, v)

	dec = msgpack.NewDecoder(bytes.NewReader(payload))
	dec.SetDuplicateKeyPolicy(msgpack.DuplicateKeyReject)
	v, err = dec.DecodeTypedMap()
	require.Error(t, err)
	require.Nil(t, v)
}

type dupKeyCountedValue struct {
	calls *int
	value int
}

func (c *dupKeyCountedValue) DecodeMsgpack(d *msgpack.Decoder) error {
	*c.calls++
	v, err := d.DecodeInt()
	c.value = v
	return err
}

func TestDuplicateKeyCustomDecoderCallCount(t *testing.T) {
	type customStruct struct {
		C dupKeyCountedValue `msgpack:"c"`
		Z int                `msgpack:"z"`
	}
	payload := encodeMapPayload(t, "c", 1, "c", 2, "z", 3)

	// First: the duplicate value is skipped without invoking the custom decoder.
	var calls int
	s := customStruct{C: dupKeyCountedValue{calls: &calls}}
	require.NoError(t, decodeWithPolicy(t, msgpack.DuplicateKeyFirst, payload, &s))
	require.Equal(t, 1, calls)
	require.Equal(t, 1, s.C.value)
	require.Equal(t, 3, s.Z)

	// Last: every occurrence is decoded.
	calls = 0
	s = customStruct{C: dupKeyCountedValue{calls: &calls}}
	require.NoError(t, decodeWithPolicy(t, msgpack.DuplicateKeyLast, payload, &s))
	require.Equal(t, 2, calls)
	require.Equal(t, 2, s.C.value)
	require.Equal(t, 3, s.Z)

	// Reject: decoding fails before the duplicate value is decoded.
	calls = 0
	s = customStruct{C: dupKeyCountedValue{calls: &calls}}
	err := decodeWithPolicy(t, msgpack.DuplicateKeyReject, payload, &s)
	require.Error(t, err)
	require.Equal(t, 1, calls)
}
