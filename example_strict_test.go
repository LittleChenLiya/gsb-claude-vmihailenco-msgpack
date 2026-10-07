package msgpack_test

import (
	"bytes"
	"fmt"

	"github.com/vmihailenco/msgpack/v5"
)

// Short regression example for the original (default) decoding interface:
// a historical sample using a bin marker still decodes into a Go string,
// while the new strict mode rejects the same implicit conversion.
func ExampleDecoder_UseStrictStringBinTypes() {
	// Historical sample: bin 8 marker carrying "foo".
	sample := []byte{0xc4, 0x03, 'f', 'o', 'o'}

	// Default compatibility mode: bin marker is accepted as a Go string.
	var s string
	err := msgpack.Unmarshal(sample, &s)
	fmt.Println(s, err)

	// Strict mode rejects the implicit bin -> string conversion.
	var s2 string
	dec := msgpack.NewDecoder(bytes.NewReader(sample))
	dec.UseStrictStringBinTypes(true)
	err = dec.Decode(&s2)
	fmt.Println(err)

	// Interface targets keep the original type in both modes.
	for _, strict := range []bool{false, true} {
		dec := msgpack.NewDecoder(bytes.NewReader(sample))
		dec.UseStrictStringBinTypes(strict)
		v, err := dec.DecodeInterface()
		fmt.Printf("%T %v %v\n", v, v, err)
	}

	// Output:
	// foo <nil>
	// msgpack: strict mode: can't decode msgpack bin code=c4 into Go string
	// []uint8 [102 111 111] <nil>
	// []uint8 [102 111 111] <nil>
}
