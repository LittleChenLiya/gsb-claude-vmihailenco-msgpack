package msgpack_test

import (
	"bytes"
	"fmt"

	"github.com/vmihailenco/msgpack/v5"
)

func ExampleDecoder_SetDuplicateKeyPolicy() {
	type item struct {
		A int `msgpack:"a"`
		Z int `msgpack:"z"`
	}

	// Hand-crafted payload: {"a": 1, "a": 2, "z": 3}.
	payload := []byte{
		0x83, // map with 3 pairs
		0xa1, 'a', 0x01,
		0xa1, 'a', 0x02, // duplicate key "a"
		0xa1, 'z', 0x03,
	}

	first := msgpack.NewDecoder(bytes.NewReader(payload))
	first.SetDuplicateKeyPolicy(msgpack.DuplicateKeyFirst)
	var s1 item
	if err := first.Decode(&s1); err != nil {
		panic(err)
	}
	fmt.Printf("first: a=%d z=%d\n", s1.A, s1.Z)

	last := msgpack.NewDecoder(bytes.NewReader(payload))
	last.SetDuplicateKeyPolicy(msgpack.DuplicateKeyLast)
	var s2 item
	if err := last.Decode(&s2); err != nil {
		panic(err)
	}
	fmt.Printf("last: a=%d z=%d\n", s2.A, s2.Z)

	reject := msgpack.NewDecoder(bytes.NewReader(payload))
	reject.SetDuplicateKeyPolicy(msgpack.DuplicateKeyReject)
	var s3 item
	fmt.Printf("reject: %v\n", reject.Decode(&s3))

	// Output:
	// first: a=1 z=3
	// last: a=2 z=3
	// reject: msgpack: duplicate field "a"
}
