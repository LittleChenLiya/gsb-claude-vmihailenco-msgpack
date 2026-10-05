# MessagePack encoding for Golang

[![Build Status](https://travis-ci.org/vmihailenco/msgpack.svg)](https://travis-ci.org/vmihailenco/msgpack)
[![PkgGoDev](https://pkg.go.dev/badge/github.com/vmihailenco/msgpack/v5)](https://pkg.go.dev/github.com/vmihailenco/msgpack/v5)
[![Documentation](https://img.shields.io/badge/msgpack-documentation-informational)](https://msgpack.uptrace.dev/)
[![Chat](https://discordapp.com/api/guilds/752070105847955518/widget.png)](https://discord.gg/rWtp5Aj)

> msgpack is brought to you by :star: [**uptrace/uptrace**](https://github.com/uptrace/uptrace).
> Uptrace is an [open source APM](https://uptrace.dev/get/open-source-apm.html) and blazingly fast
> [distributed tracing tool](https://get.uptrace.dev/compare/distributed-tracing-tools.html) powered
> by OpenTelemetry and ClickHouse. Give it a star as well!

## Resources

- [Documentation](https://msgpack.uptrace.dev)
- [Chat](https://discord.gg/rWtp5Aj)
- [Reference](https://pkg.go.dev/github.com/vmihailenco/msgpack/v5)
- [Examples](https://pkg.go.dev/github.com/vmihailenco/msgpack/v5#pkg-examples)

## Features

- Primitives, arrays, maps, structs, time.Time and interface{}.
- Appengine \*datastore.Key and datastore.Cursor.
- [CustomEncoder]/[CustomDecoder] interfaces for custom encoding.
- [Extensions](https://pkg.go.dev/github.com/vmihailenco/msgpack/v5#example-RegisterExt) to encode
  type information.
- Renaming fields via `msgpack:"my_field_name"` and alias via `msgpack:"alias:another_name"`.
- Omitting individual empty fields via `msgpack:",omitempty"` tag or all
  [empty fields in a struct](https://pkg.go.dev/github.com/vmihailenco/msgpack/v5#example-Marshal-OmitEmpty).
- [Map keys sorting](https://pkg.go.dev/github.com/vmihailenco/msgpack/v5#Encoder.SetSortMapKeys).
- Encoding/decoding all
  [structs as arrays](https://pkg.go.dev/github.com/vmihailenco/msgpack/v5#Encoder.UseArrayEncodedStructs)
  or
  [individual structs](https://pkg.go.dev/github.com/vmihailenco/msgpack/v5#example-Marshal-AsArray).
- [Encoder.SetCustomStructTag] with [Decoder.SetCustomStructTag] can turn msgpack into drop-in
  replacement for any tag.
- Simple but very fast and efficient
  [queries](https://pkg.go.dev/github.com/vmihailenco/msgpack/v5#example-Decoder.Query).

[customencoder]: https://pkg.go.dev/github.com/vmihailenco/msgpack/v5#CustomEncoder
[customdecoder]: https://pkg.go.dev/github.com/vmihailenco/msgpack/v5#CustomDecoder
[encoder.setcustomstructtag]:
  https://pkg.go.dev/github.com/vmihailenco/msgpack/v5#Encoder.SetCustomStructTag
[decoder.setcustomstructtag]:
  https://pkg.go.dev/github.com/vmihailenco/msgpack/v5#Decoder.SetCustomStructTag

## Installation

msgpack supports 2 last Go versions and requires support for
[Go modules](https://github.com/golang/go/wiki/Modules). So make sure to initialize a Go module:

```shell
go mod init github.com/my/repo
```

And then install msgpack/v5 (note _v5_ in the import; omitting it is a popular mistake):

```shell
go get github.com/vmihailenco/msgpack/v5
```

## Quickstart

```go
import "github.com/vmihailenco/msgpack/v5"

func ExampleMarshal() {
    type Item struct {
        Foo string
    }

    b, err := msgpack.Marshal(&Item{Foo: "bar"})
    if err != nil {
        panic(err)
    }

    var item Item
    err = msgpack.Unmarshal(b, &item)
    if err != nil {
        panic(err)
    }
    fmt.Println(item.Foo)
    // Output: bar
}
```

## Timestamp extension

`time.Time` is encoded as the msgpack
[timestamp extension](https://github.com/msgpack/msgpack/blob/master/spec.md#timestamp-extension-type)
(type id `-1`). The codec is strict: it rejects illegal nanoseconds
(`>= 1_000_000_000`), wrong extension lengths and overflow before decoding,
and decodes to UTC so the instant does not depend on the host timezone.

The shortest legal layout is chosen from the seconds and nanoseconds:

- **timestamp 32** (fixext 4, 4 body bytes): seconds since 1970-01-01 UTC as an
  unsigned 32-bit big-endian integer; nanoseconds must be zero. Used when
  `0 <= sec < 2^32` and `nsec == 0`.
  Body: `[sec32 BE]`
- **timestamp 64** (fixext 8, 8 body bytes): the top 30 bits hold the unsigned
  nanoseconds (`0..999_999_999`) and the low 34 bits hold the unsigned seconds,
  both big-endian. Used when `0 <= sec < 2^34`.
  Body: `[nsec30 | sec34 BE]`
- **timestamp 96** (ext 8/16/32, 12 body bytes): an unsigned 32-bit big-endian
  nanoseconds field followed by a **signed** 64-bit big-endian seconds field
  (two's complement), so seconds before the Unix epoch are preserved. Used for
  negative seconds and for `sec >= 2^34`.
  Body: `[nsec32 BE][sec64 signed BE]`

The acceptance tests pin these layouts byte-for-byte, including the boundaries
`sec = 2^32`, `sec = 2^34`, negative seconds and `nsec = 999_999_999` /
`nsec = 1_000_000_000`. Run them with:

```shell
go test github.com/vmihailenco/msgpack/v5 -run 'TestTimestamp' -v
```

## See also

- [Golang ORM](https://github.com/uptrace/bun) for PostgreSQL, MySQL, MSSQL, and SQLite
- [Golang PostgreSQL](https://bun.uptrace.dev/postgres/)
- [Golang HTTP router](https://github.com/uptrace/bunrouter)
- [Golang ClickHouse ORM](https://github.com/uptrace/go-clickhouse)

## Contributors

Thanks to all the people who already contributed!

<a href="https://github.com/vmihailenco/msgpack/graphs/contributors">
  <img src="https://contributors-img.web.app/image?repo=vmihailenco/msgpack" />
</a>
