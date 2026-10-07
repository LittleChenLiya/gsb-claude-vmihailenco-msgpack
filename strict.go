package msgpack

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/vmihailenco/msgpack/v5/msgpcode"
)

// StrictStringBinError is returned by the Decoder when strict string/bin
// mode is enabled (see Decoder.UseStrictStringBinTypes) and a msgpack bin
// marker would be implicitly decoded into a Go string, or a msgpack str
// marker would be implicitly decoded into a Go []byte/[N]byte.
type StrictStringBinError struct {
	// Code is the msgpack code that was encountered.
	Code byte
	// Want is the Go target type: "string" or "[]byte".
	Want string
}

func (e *StrictStringBinError) Error() string {
	var got string
	switch {
	case msgpcode.IsBin(e.Code):
		got = "bin"
	case msgpcode.IsString(e.Code):
		got = "str"
	default:
		got = "unknown"
	}
	return fmt.Sprintf("msgpack: strict mode: can't decode msgpack %s code=%x into Go %s",
		got, e.Code, e.Want)
}

// FieldError describes an error that occurred while decoding a struct field.
// Path contains the nested field path, e.g. "Outer.Inner.Name". It is only
// returned when strict string/bin mode is enabled.
type FieldError struct {
	Path string
	Err  error
}

func (e *FieldError) Error() string {
	return fmt.Sprintf("msgpack: field %q: %v", e.Path, e.Err)
}

func (e *FieldError) Unwrap() error {
	return e.Err
}

// checkStrictStringCode rejects bin markers when the target is a Go string
// and strict string/bin mode is enabled.
func (d *Decoder) checkStrictStringCode(c byte) error {
	if d.flags&strictStringBinFlag != 0 && msgpcode.IsBin(c) {
		return &StrictStringBinError{Code: c, Want: "string"}
	}
	return nil
}

// checkStrictBytesCode rejects str markers when the target is a Go
// []byte/[N]byte and strict string/bin mode is enabled.
func (d *Decoder) checkStrictBytesCode(c byte) error {
	if d.flags&strictStringBinFlag != 0 && msgpcode.IsString(c) {
		return &StrictStringBinError{Code: c, Want: "[]byte"}
	}
	return nil
}

// decodeStructField decodes a single struct field. In strict string/bin mode
// it tracks the nested field path and wraps field errors with a FieldError.
func (d *Decoder) decodeStructField(f *field, strct reflect.Value) error {
	if d.flags&strictStringBinFlag == 0 {
		return f.DecodeValue(d, strct)
	}

	d.path = append(d.path, f.name)
	err := f.DecodeValue(d, strct)
	if err == nil {
		d.path = d.path[:len(d.path)-1]
		return nil
	}

	path := strings.Join(d.path, ".")
	d.path = d.path[:len(d.path)-1]

	var fe *FieldError
	if errors.As(err, &fe) {
		// The innermost field already recorded the full nested path.
		return err
	}
	return &FieldError{Path: path, Err: err}
}
