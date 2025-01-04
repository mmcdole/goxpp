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
file, _ := os.Open("file.xml")
parser := xpp.NewXMLPullParser(file, false, nil)

// Navigate through the document
for {
    token, _ := parser.Next()
    if token == xpp.EndDocument {
        break
    }
    
    switch token {
    case xpp.StartTag:
        // Handle start tag
        fmt.Printf("Tag: %s\n", parser.Name)
    case xpp.Text:
        // Handle text content
        fmt.Printf("Text: %s\n", parser.Text)
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
