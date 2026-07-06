# goxpp

[![Build Status](https://github.com/mmcdole/goxpp/actions/workflows/ci.yml/badge.svg)](https://github.com/mmcdole/goxpp/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/mmcdole/goxpp/branch/master/graph/badge.svg)](https://codecov.io/gh/mmcdole/goxpp)
[![License](http://img.shields.io/:license-mit-blue.svg)](http://doge.mit-license.org)
[![Go Reference](https://pkg.go.dev/badge/github.com/mmcdole/goxpp/v2.svg)](https://pkg.go.dev/github.com/mmcdole/goxpp/v2)

A lightweight XML pull parser for Go, inspired by [Java's XMLPullParser](http://www.xmlpull.org/v1/download/unpacked/doc/quick_intro.html). It provides fine-grained control over XML parsing with a small, cursor-style API.

## Features

- Pull-based parsing for fine-grained document control
- Scoped namespace and xml:base tracking
- Efficient navigation and element skipping
- Errors you can match with `errors.As` / `errors.Is`

## Installation

```bash
go get github.com/mmcdole/goxpp/v2
```

v1 remains available at `github.com/mmcdole/goxpp` and receives critical fixes on the `v1` branch.

## Quick Start

```go
import (
    "encoding/xml"

    xpp "github.com/mmcdole/goxpp/v2"
)

// Parse an RSS feed
file, _ := os.Open("feed.rss")
d := xml.NewDecoder(file)
d.Strict = false
p := xpp.New(d)

// Find the channel element
for tok, err := p.NextTag(); tok != xpp.EndDocument; tok, err = p.NextTag() {
    if err != nil {
        return err
    }
    if tok == xpp.StartTag && p.Name() == "channel" {
        // Process channel contents
        for tok, err = p.NextTag(); tok != xpp.EndTag; tok, err = p.NextTag() {
            if err != nil {
                return err
            }
            if tok == xpp.StartTag {
                switch p.Name() {
                case "title":
                    title, _ := p.NextText()
                    fmt.Printf("Feed: %s\n", title)
                case "item":
                    // Get the item title and skip the rest
                    p.NextTag()
                    title, _ := p.NextText()
                    fmt.Printf("Item: %s\n", title)
                    p.Skip()
                default:
                    p.Skip()
                }
            }
        }
        break
    }
}
```

## Token Types

- `StartDocument`, `EndDocument`
- `StartTag`, `EndTag`
- `Text`, `Comment`
- `ProcessingInstruction`, `Directive`

## Migrating from v1

The v2 changes are listed in [#34](https://github.com/mmcdole/goxpp/issues/34). In short:

- Construct with `xpp.New(*xml.Decoder)`; configure strictness and charset conversion on the decoder.
- Cursor state moved from exported fields to methods: `p.Name` becomes `p.Name()`, and so on.
- `Namespaces()` maps prefix to URI; `PrefixForURI` covers the reverse lookup.
- `BaseURL()` exposes the in-scope xml:base; resolving URLs against it is the caller's concern.
- Advancement calls after `EndDocument` return `io.EOF`; positional failures are `*xpp.ExpectError`.

## Documentation

For detailed documentation and examples, visit [pkg.go.dev](https://pkg.go.dev/github.com/mmcdole/goxpp/v2).

## License

This project is licensed under the [MIT License](LICENSE).
