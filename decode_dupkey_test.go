package msgpack_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vmihailenco/msgpack/v5"
)

// dupCustom is a type with a custom DecodeMsgpack that counts how many times
// it is invoked.
type dupCustom struct {
	count int
}

func (c *dupCustom) DecodeMsgpack(d *msgpack.Decoder) error {
	c.count++
	return d.Decode(&c.count)
}

type dupInner struct {
	X string `msgpack:"x"`
}

type dupOuter struct {
	X      string    `msgpack:"x"`
	Z      string    `msgpack:"z"`
	In     dupInner  `msgpack:"in"`
	Custom dupCustom `msgpack:"custom"`
}

func encodeMap(t *testing.T, kv ...interface{}) []byte {
	t.Helper()
	var buf bytes.Buffer
	enc := msgpack.NewEncoder(&buf)
	require.NoError(t, enc.EncodeMapLen(len(kv)/2))
	for i := 0; i < len(kv); i += 2 {
		require.NoError(t, enc.Encode(kv[i]))
		require.NoError(t, enc.Encode(kv[i+1]))
	}
	return buf.Bytes()
}

func TestDuplicateKey_DefaultLastWins(t *testing.T) {
	payload := encodeMap(t, "x", "first", "x", "last")

	var out map[string]interface{}
	dec := msgpack.NewDecoder(bytes.NewReader(payload))
	require.NoError(t, dec.Decode(&out))
	require.Equal(t, "last", out["x"])
}

func TestDuplicateKey_FirstWins(t *testing.T) {
	payload := encodeMap(t, "x", "first", "x", "last", "z", "zed")

	var out map[string]interface{}
	dec := msgpack.NewDecoder(bytes.NewReader(payload))
	dec.SetDuplicateKeyStrategy(msgpack.DuplicateKeyFirst)
	require.NoError(t, dec.Decode(&out))
	require.Equal(t, "first", out["x"])
	require.Equal(t, "zed", out["z"])
}

func TestDuplicateKey_Reject(t *testing.T) {
	payload := encodeMap(t, "x", "first", "x", "last")

	var out map[string]interface{}
	dec := msgpack.NewDecoder(bytes.NewReader(payload))
	dec.SetDuplicateKeyStrategy(msgpack.DuplicateKeyReject)
	err := dec.Decode(&out)
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate key")
}

func TestDuplicateKey_FirstThenValidField(t *testing.T) {
	// A duplicate field followed by a valid field must still decode the
	// valid field in first-value mode.
	payload := encodeMap(t, "x", "first", "x", "last", "z", "zed")

	var out dupOuter
	dec := msgpack.NewDecoder(bytes.NewReader(payload))
	dec.SetDuplicateKeyStrategy(msgpack.DuplicateKeyFirst)
	require.NoError(t, dec.Decode(&out))
	require.Equal(t, "first", out.X)
	require.Equal(t, "zed", out.Z)
}

func TestDuplicateKey_RejectLeavesStructUntouched(t *testing.T) {
	payload := encodeMap(t, "x", "first", "x", "last", "z", "zed")

	var out dupOuter
	dec := msgpack.NewDecoder(bytes.NewReader(payload))
	dec.SetDuplicateKeyStrategy(msgpack.DuplicateKeyReject)
	err := dec.Decode(&out)
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate")
	// The struct must not be half-populated.
	require.Equal(t, "", out.X)
	require.Equal(t, "", out.Z)
}

func TestDuplicateKey_AliasSameFieldRejected(t *testing.T) {
	type AliasStruct struct {
		F string `msgpack:"foo,alias:bar"`
	}
	// Both "foo" and "bar" hit the same struct field.
	payload := encodeMap(t, "foo", "a", "bar", "b")

	var out AliasStruct
	dec := msgpack.NewDecoder(bytes.NewReader(payload))
	dec.SetDuplicateKeyStrategy(msgpack.DuplicateKeyReject)
	err := dec.Decode(&out)
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate")
}

func TestDuplicateKey_AliasSameFieldFirstWins(t *testing.T) {
	type AliasStruct struct {
		F string `msgpack:"foo,alias:bar"`
	}
	payload := encodeMap(t, "foo", "a", "bar", "b")

	var out AliasStruct
	dec := msgpack.NewDecoder(bytes.NewReader(payload))
	dec.SetDuplicateKeyStrategy(msgpack.DuplicateKeyFirst)
	require.NoError(t, dec.Decode(&out))
	require.Equal(t, "a", out.F)
}

func TestDuplicateKey_NestedSameNameNoCrossLayer(t *testing.T) {
	// The outer struct and the nested struct both have a field "x". This
	// must not be treated as a duplicate across layers.
	var buf bytes.Buffer
	enc := msgpack.NewEncoder(&buf)
	require.NoError(t, enc.EncodeMapLen(2))
	require.NoError(t, enc.Encode("x"))
	require.NoError(t, enc.Encode("outer"))
	require.NoError(t, enc.Encode("in"))
	require.NoError(t, enc.EncodeMapLen(1))
	require.NoError(t, enc.Encode("x"))
	require.NoError(t, enc.Encode("inner"))

	var out dupOuter
	dec := msgpack.NewDecoder(&buf)
	dec.SetDuplicateKeyStrategy(msgpack.DuplicateKeyReject)
	require.NoError(t, dec.Decode(&out))
	require.Equal(t, "outer", out.X)
	require.Equal(t, "inner", out.In.X)
}

func TestDuplicateKey_CustomDecodeCallbackCount(t *testing.T) {
	// In first-value mode the second occurrence of a duplicate key must be
	// skipped without invoking the custom decoder again.
	var buf bytes.Buffer
	enc := msgpack.NewEncoder(&buf)
	require.NoError(t, enc.EncodeMapLen(2))
	require.NoError(t, enc.Encode("custom"))
	require.NoError(t, enc.Encode(1))
	require.NoError(t, enc.Encode("custom"))
	require.NoError(t, enc.Encode(2))

	var out dupOuter
	dec := msgpack.NewDecoder(&buf)
	dec.SetDuplicateKeyStrategy(msgpack.DuplicateKeyFirst)
	require.NoError(t, dec.Decode(&out))
	require.Equal(t, 1, out.Custom.count)
}

func TestDuplicateKey_SkipFullyConsumesContainer(t *testing.T) {
	// A duplicate value that is itself a map must be fully skipped so that
	// the next field still decodes.
	inner := encodeMap(t, "a", 1, "b", 2)
	payload := encodeMap(t, "x", "first", "x", inner, "z", "zed")

	var out dupOuter
	dec := msgpack.NewDecoder(bytes.NewReader(payload))
	dec.SetDuplicateKeyStrategy(msgpack.DuplicateKeyFirst)
	require.NoError(t, dec.Decode(&out))
	require.Equal(t, "first", out.X)
	require.Equal(t, "zed", out.Z)
}

func TestDuplicateKey_MapStringStringReject(t *testing.T) {
	payload := encodeMap(t, "a", "1", "a", "2")

	var out map[string]string
	dec := msgpack.NewDecoder(bytes.NewReader(payload))
	dec.SetDuplicateKeyStrategy(msgpack.DuplicateKeyReject)
	err := dec.Decode(&out)
	require.Error(t, err)
	require.Nil(t, out)
}

func TestDuplicateKey_MapStringInterfaceReject(t *testing.T) {
	payload := encodeMap(t, "a", 1, "a", 2)

	var out map[string]interface{}
	dec := msgpack.NewDecoder(bytes.NewReader(payload))
	dec.SetDuplicateKeyStrategy(msgpack.DuplicateKeyReject)
	err := dec.Decode(&out)
	require.Error(t, err)
	require.Nil(t, out)
}

func TestDuplicateKey_UntypedMapFirstWins(t *testing.T) {
	payload := encodeMap(t, "a", 1, "a", 2, "b", 3)

	var out map[interface{}]interface{}
	dec := msgpack.NewDecoder(bytes.NewReader(payload))
	dec.SetDuplicateKeyStrategy(msgpack.DuplicateKeyFirst)
	require.NoError(t, dec.Decode(&out))
	require.Equal(t, int8(1), out["a"])
	require.Equal(t, int8(3), out["b"])
}

func TestDuplicateKey_TypedMapFirstWins(t *testing.T) {
	payload := encodeMap(t, 1, "one", 1, "uno", 2, "two")

	var out map[int]string
	dec := msgpack.NewDecoder(bytes.NewReader(payload))
	dec.SetDuplicateKeyStrategy(msgpack.DuplicateKeyFirst)
	require.NoError(t, dec.Decode(&out))
	require.Equal(t, "one", out[1])
	require.Equal(t, "two", out[2])
}

func TestDuplicateKey_SameStringDifferentEncoding(t *testing.T) {
	// Force the same string key to be encoded once as fixstr and once as
	// str8.
	var buf bytes.Buffer
	enc := msgpack.NewEncoder(&buf)
	require.NoError(t, enc.EncodeMapLen(2))
	require.NoError(t, enc.Encode("x"))
	require.NoError(t, enc.Encode("first"))
	require.NoError(t, enc.EncodeBytes([]byte("x")))
	require.NoError(t, enc.Encode("last"))

	var out map[string]interface{}
	dec := msgpack.NewDecoder(&buf)
	dec.SetDuplicateKeyStrategy(msgpack.DuplicateKeyReject)
	err := dec.Decode(&out)
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate key")
}

func TestDuplicateKey_ResetClearsStrategy(t *testing.T) {
	payload := encodeMap(t, "x", "first", "x", "last")

	var out map[string]interface{}
	dec := msgpack.NewDecoder(bytes.NewReader(payload))
	dec.SetDuplicateKeyStrategy(msgpack.DuplicateKeyReject)
	dec.Reset(bytes.NewReader(payload))
	// After Reset the strategy is back to the default (last wins).
	require.NoError(t, dec.Decode(&out))
	require.Equal(t, "last", out["x"])
}

func ExampleDecoder_SetDuplicateKeyStrategy() {
	// Encode a small map with a duplicate key: {"x": "first", "x": "last"}.
	var buf bytes.Buffer
	enc := msgpack.NewEncoder(&buf)
	_ = enc.EncodeMapLen(2)
	_ = enc.Encode("x")
	_ = enc.Encode("first")
	_ = enc.Encode("x")
	_ = enc.Encode("last")

	var out map[string]interface{}
	dec := msgpack.NewDecoder(&buf)
	dec.SetDuplicateKeyStrategy(msgpack.DuplicateKeyFirst)
	if err := dec.Decode(&out); err != nil {
		panic(err)
	}
	fmt.Println("x =", out["x"])
	// Output: x = first
}

func TestDuplicateKey_ExplicitLastWins(t *testing.T) {
	payload := encodeMap(t, "x", "first", "x", "last")

	var out map[string]interface{}
	dec := msgpack.NewDecoder(bytes.NewReader(payload))
	dec.SetDuplicateKeyStrategy(msgpack.DuplicateKeyLast)
	require.NoError(t, dec.Decode(&out))
	require.Equal(t, "last", out["x"])
}

func TestDuplicateKey_NestedMapSameNameNoCrossLayer(t *testing.T) {
	// A nested map with the same key name must not be treated as a
	// duplicate of the outer key.
	var buf bytes.Buffer
	enc := msgpack.NewEncoder(&buf)
	require.NoError(t, enc.EncodeMapLen(2))
	require.NoError(t, enc.Encode("x"))
	require.NoError(t, enc.Encode("outer"))
	require.NoError(t, enc.Encode("inner"))
	require.NoError(t, enc.EncodeMapLen(1))
	require.NoError(t, enc.Encode("x"))
	require.NoError(t, enc.Encode("inner"))

	var out map[string]interface{}
	dec := msgpack.NewDecoder(&buf)
	dec.SetDuplicateKeyStrategy(msgpack.DuplicateKeyReject)
	require.NoError(t, dec.Decode(&out))
	require.Equal(t, "outer", out["x"])
	inner := out["inner"].(map[string]interface{})
	require.Equal(t, "inner", inner["x"])
}

func TestDuplicateKey_FirstModeSkipsArrayValue(t *testing.T) {
	// A duplicate value that is an array must be fully skipped so that the
	// next field still decodes.
	var buf bytes.Buffer
	enc := msgpack.NewEncoder(&buf)
	require.NoError(t, enc.EncodeMapLen(3))
	require.NoError(t, enc.Encode("x"))
	require.NoError(t, enc.Encode("first"))
	require.NoError(t, enc.Encode("x"))
	require.NoError(t, enc.EncodeArrayLen(2))
	require.NoError(t, enc.Encode(1))
	require.NoError(t, enc.Encode(2))
	require.NoError(t, enc.Encode("z"))
	require.NoError(t, enc.Encode("zed"))

	var out dupOuter
	dec := msgpack.NewDecoder(&buf)
	dec.SetDuplicateKeyStrategy(msgpack.DuplicateKeyFirst)
	require.NoError(t, dec.Decode(&out))
	require.Equal(t, "first", out.X)
	require.Equal(t, "zed", out.Z)
}

func TestDuplicateKey_RejectWithExistingMap(t *testing.T) {
	// A pre-existing non-nil map must not be half-populated when a
	// duplicate key is rejected.
	payload := encodeMap(t, "a", "1", "a", "2")

	out := map[string]string{"existing": "value"}
	dec := msgpack.NewDecoder(bytes.NewReader(payload))
	dec.SetDuplicateKeyStrategy(msgpack.DuplicateKeyReject)
	err := dec.Decode(&out)
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate key")
	// The map must be untouched.
	require.Equal(t, map[string]string{"existing": "value"}, out)
}

func TestDuplicateKey_RejectWithExistingTypedMap(t *testing.T) {
	payload := encodeMap(t, 1, "one", 1, "uno")

	out := map[int]string{99: "existing"}
	dec := msgpack.NewDecoder(bytes.NewReader(payload))
	dec.SetDuplicateKeyStrategy(msgpack.DuplicateKeyReject)
	err := dec.Decode(&out)
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate key")
	require.Equal(t, map[int]string{99: "existing"}, out)
}
