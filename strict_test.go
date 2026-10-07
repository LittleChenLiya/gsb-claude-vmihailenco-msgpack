package msgpack_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vmihailenco/msgpack/v5"
)

// strictPayload is a small object that contains both marker families:
// Name is encoded with a str marker, Data with a bin marker.
type strictPayload struct {
	Name string
	Data []byte
}

type strictNested struct {
	Inner strictPayload
}

// bin8Foo is a historical sample: bin 8 marker carrying "foo".
var bin8Foo = []byte{0xc4, 0x03, 'f', 'o', 'o'}

// fixstrFoo is a fixstr marker carrying "foo".
var fixstrFoo = []byte{0xa3, 'f', 'o', 'o'}

func strictDecoder(data []byte) *msgpack.Decoder {
	dec := msgpack.NewDecoder(bytes.NewReader(data))
	dec.UseStrictStringBinTypes(true)
	return dec
}

// Same-type assignments succeed in strict mode.
func TestStrictStringBinSameType(t *testing.T) {
	in := strictPayload{Name: "hello", Data: []byte{1, 2, 3}}

	b, err := msgpack.Marshal(in)
	require.NoError(t, err)
	// The fixture really contains both marker families.
	require.Contains(t, b, byte(0xa5)) // fixstr "hello"
	require.Contains(t, b, byte(0xc4)) // bin 8

	var out strictPayload
	err = strictDecoder(b).Decode(&out)
	require.NoError(t, err)
	require.Equal(t, in, out)
}

// Cross assignments fail in strict mode: bin -> string and str -> []byte.
func TestStrictStringBinCrossAssign(t *testing.T) {
	t.Run("bin into string", func(t *testing.T) {
		// {"Name": bin8("foo")}
		data := append([]byte{0x81, 0xa4, 'N', 'a', 'm', 'e'}, bin8Foo...)

		var out struct {
			Name string
		}
		err := strictDecoder(data).Decode(&out)
		require.Error(t, err)

		var serr *msgpack.StrictStringBinError
		require.True(t, errors.As(err, &serr), "got %T: %v", err, err)
		require.Equal(t, "string", serr.Want)

		var ferr *msgpack.FieldError
		require.True(t, errors.As(err, &ferr), "got %T: %v", err, err)
		require.Equal(t, "Name", ferr.Path)
	})

	t.Run("str into bytes", func(t *testing.T) {
		// {"Data": fixstr("foo")}
		data := append([]byte{0x81, 0xa4, 'D', 'a', 't', 'a'}, fixstrFoo...)

		var out struct {
			Data []byte
		}
		err := strictDecoder(data).Decode(&out)
		require.Error(t, err)

		var serr *msgpack.StrictStringBinError
		require.True(t, errors.As(err, &serr), "got %T: %v", err, err)
		require.Equal(t, "[]byte", serr.Want)

		var ferr *msgpack.FieldError
		require.True(t, errors.As(err, &ferr), "got %T: %v", err, err)
		require.Equal(t, "Data", ferr.Path)
	})
}

// Nested field errors carry the full field path.
func TestStrictStringBinNestedPath(t *testing.T) {
	// {"Inner": {"Data": fixstr("x")}}
	data := []byte{
		0x81, 0xa5, 'I', 'n', 'n', 'e', 'r',
		0x81, 0xa4, 'D', 'a', 't', 'a',
		0xa1, 'x',
	}

	var out strictNested
	err := strictDecoder(data).Decode(&out)
	require.Error(t, err)

	var ferr *msgpack.FieldError
	require.True(t, errors.As(err, &ferr), "got %T: %v", err, err)
	require.Equal(t, "Inner.Data", ferr.Path)
	require.Contains(t, err.Error(), "Inner.Data")

	var serr *msgpack.StrictStringBinError
	require.True(t, errors.As(err, &serr), "got %T: %v", err, err)
}

// Interface targets keep the original types in strict mode.
func TestStrictStringBinInterfaceKeepsType(t *testing.T) {
	v, err := strictDecoder(bin8Foo).DecodeInterface()
	require.NoError(t, err)
	require.Equal(t, []byte("foo"), v)

	v, err = strictDecoder(fixstrFoo).DecodeInterface()
	require.NoError(t, err)
	require.Equal(t, "foo", v)

	// Even together with loose interface decoding, bin stays []byte.
	dec := strictDecoder(bin8Foo)
	dec.UseLooseInterfaceDecoding(true)
	v, err = dec.DecodeInterface()
	require.NoError(t, err)
	require.Equal(t, []byte("foo"), v)
}

// Default compatibility mode is unchanged: historical samples with
// cross markers still decode.
func TestCompatModeHistoricalSamples(t *testing.T) {
	// bin marker -> string (historical sample).
	var s string
	require.NoError(t, msgpack.Unmarshal(bin8Foo, &s))
	require.Equal(t, "foo", s)

	// str marker -> []byte.
	var b []byte
	require.NoError(t, msgpack.Unmarshal(fixstrFoo, &b))
	require.Equal(t, []byte("foo"), b)

	// Cross fixtures inside a struct also decode in default mode.
	data := append([]byte{0x81, 0xa4, 'N', 'a', 'm', 'e'}, bin8Foo...)
	var out struct {
		Name string
	}
	require.NoError(t, msgpack.Unmarshal(data, &out))
	require.Equal(t, "foo", out.Name)

	// Interface decoding in default mode is unchanged.
	v, err := msgpack.NewDecoder(bytes.NewReader(bin8Foo)).DecodeInterface()
	require.NoError(t, err)
	require.Equal(t, []byte("foo"), v)
}
