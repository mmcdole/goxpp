# goxpp

[![Build Status](https://github.com/mmcdole/goxpp/actions/workflows/ci.yml/badge.svg)](https://github.com/mmcdole/goxpp/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/mmcdole/goxpp/branch/master/graph/badge.svg)](https://codecov.io/gh/mmcdole/goxpp)
[![License](http://img.shields.io/:license-mit-blue.svg)](http://doge.mit-license.org)
[![Go Reference](https://pkg.go.dev/badge/github.com/mmcdole/goxpp.svg)](https://pkg.go.dev/github.com/mmcdole/goxpp)

A lightweight XML Pull Parser for Go, inspired by [Java's XMLPullParser](http://www.xmlpull.org/v1/download/unpacked/doc/quick_intro.html). It provides fine-grained control over XML parsing with a simple, intuitive API.

## Features

- Pull-based parsing for fine-grained document control
- Efficient navigation and element skipping
- Simple, idiomatic Go API

## Installation

```bash
go get github.com/mmcdole/goxpp
```

## Quick Start

```go
import "github.com/mmcdole/goxpp"

// Create a new parser
file, _ := os.Open("feed.rss")
p := xpp.NewXMLPullParser(file, false, nil)

// Find RSS root and channel
for {
    tok, err := p.NextTag()
    if err != nil {
        return err
    }
    if tok == xpp.StartTag && p.Name == "channel" {
        break
    }
}

// Process channel elements
for {
    tok, err := p.NextTag()
    if err != nil {
        return err
    }
    if tok == xpp.EndTag {
        break
    }

    if tok == xpp.StartTag {
        switch p.Name {
        case "title":
            title, _ := p.NextText()
            fmt.Printf("Feed: %s\n", title)
        case "item":
            // Process item fields
            for {
                tok, err := p.NextTag()
                if err != nil {
                    return err
                }
                if tok == xpp.EndTag {
                    break
                }
                if tok == xpp.StartTag {
                    switch p.Name {
                    case "title":
                        title, _ := p.NextText()
                        fmt.Printf("  Title: %s\n", title)
                    case "link":
                        link, _ := p.NextText()
                        fmt.Printf("  URL: %s\n", link)
                    case "description":
                        desc, _ := p.NextText()
                        fmt.Printf("  Description: %s\n", desc)
                    default:
                        p.Skip()
                    }
                }
            }
        default:
            p.Skip()
        }
    }
}
```

## Token Types

- `StartDocument`, `EndDocument`
- `StartTag`, `EndTag`
- `Text`, `Comment`
- `ProcessingInstruction`, `Directive`
- `IgnorableWhitespace`

## Documentation

For detailed documentation and examples, visit [pkg.go.dev](https://pkg.go.dev/github.com/mmcdole/goxpp).

## License

This project is licensed under the [MIT License](LICENSE).
