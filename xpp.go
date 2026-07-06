// Package xpp implements a cursor-style XML pull parser over encoding/xml,
// modeled on Java's XmlPullParser.
//
// The parser is a cursor: an advancement call (Next, NextToken, NextTag)
// positions it on a token, and accessor methods (Event, Name, Space, Text,
// Attrs, Depth) describe the token the parser is currently on. Convenience
// methods (Expect, NextText, Skip, DecodeElement) operate on the current
// token.
package xpp

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
)

// EventType identifies the kind of token the parser is positioned on.
type EventType int

const (
	// StartDocument is the parser's state before the first advancement
	// call. It is never returned by Next or NextToken.
	StartDocument EventType = iota
	// EndDocument is returned once when the document ends; every
	// advancement call after that returns io.EOF.
	EndDocument
	StartTag
	EndTag
	Text
	Comment
	ProcessingInstruction
	Directive
)

func (e EventType) String() string {
	switch e {
	case StartDocument:
		return "StartDocument"
	case EndDocument:
		return "EndDocument"
	case StartTag:
		return "StartTag"
	case EndTag:
		return "EndTag"
	case Text:
		return "Text"
	case Comment:
		return "Comment"
	case ProcessingInstruction:
		return "ProcessingInstruction"
	case Directive:
		return "Directive"
	}
	return fmt.Sprintf("EventType(%d)", int(e))
}

const xmlNSURI = "http://www.w3.org/XML/1998/namespace"

// ExpectError reports a positional assertion failure: the parser was not on
// the event or name the caller required. It is returned by Expect and
// ExpectAll, and by the preconditions of NextTag, NextText, Skip and
// DecodeElement. Want fields hold "*" where anything was acceptable.
type ExpectError struct {
	WantEvent           EventType
	WantSpace, WantName string
	GotEvent            EventType
	GotSpace, GotName   string
	Offset              int64
}

func (e *ExpectError) Error() string {
	return fmt.Sprintf("xpp: expected space:%s name:%s event:%s but got space:%s name:%s event:%s at offset %d",
		e.WantSpace, e.WantName, e.WantEvent, e.GotSpace, e.GotName, e.GotEvent, e.Offset)
}

// nsScope is one element's namespace scope: the full merged prefix -> URI
// view, plus the element's own declarations in document order (needed for
// PrefixForURI's most-recently-declared rule).
type nsScope struct {
	bindings map[string]string
	decls    []nsDecl
}

type nsDecl struct {
	prefix, uri string
}

// Parser is a cursor-style XML pull parser. Create one with New; the zero
// value returns an error from every advancement call.
type Parser struct {
	decoder *xml.Decoder
	token   xml.Token

	event EventType
	name  string
	space string
	text  string
	attrs []xml.Attr
	depth int

	nsStack   []nsScope
	baseStack []*url.URL

	// pendingPop defers the scope/depth pop for an EndTag until the next
	// advancement call, so Depth, Namespaces and BaseURL describe the
	// element itself while the cursor is on its end tag, matching the
	// behavior at its start tag.
	pendingPop bool
	docEnded   bool
	err        error
}

// New returns a parser reading from d. Configure strictness and charset
// conversion on the decoder directly (d.Strict, d.CharsetReader); the parser
// adds no configuration of its own.
func New(d *xml.Decoder) *Parser {
	return &Parser{decoder: d, event: StartDocument}
}

// NextToken advances to the next raw token, including comments, processing
// instructions and directives. The first call after the document ends
// returns (EndDocument, nil); every call after that returns io.EOF. After a
// decoder error or a failed DecodeElement the parser is poisoned and every
// call returns that error; see Err.
func (p *Parser) NextToken() (EventType, error) {
	if p.err != nil {
		return p.event, p.err
	}
	if p.decoder == nil {
		p.err = errors.New("xpp: parser has no decoder; use New")
		return p.event, p.err
	}
	if p.docEnded {
		return p.event, io.EOF
	}

	p.applyPendingPop()
	p.resetTokenState()

	tok, err := p.decoder.Token()
	if err != nil {
		if err == io.EOF {
			p.token = nil
			p.event = EndDocument
			p.docEnded = true
			return p.event, nil
		}
		p.err = err
		return p.event, err
	}

	p.token = xml.CopyToken(tok)
	p.processToken(p.token)
	return p.event, nil
}

// Next advances like NextToken but skips Comment, ProcessingInstruction and
// Directive tokens.
func (p *Parser) Next() (EventType, error) {
	for {
		event, err := p.NextToken()
		if err != nil {
			return event, err
		}
		switch event {
		case Comment, ProcessingInstruction, Directive:
			continue
		}
		return event, nil
	}
}

// NextTag advances past any whitespace text and returns the next StartTag or
// EndTag. Anything else is an error.
func (p *Parser) NextTag() (EventType, error) {
	t, err := p.Next()
	if err != nil {
		return t, err
	}
	for t == Text && p.IsWhitespace() {
		t, err = p.Next()
		if err != nil {
			return t, err
		}
	}
	if t != StartTag && t != EndTag {
		return t, p.expectErr(StartTag, "*", "*")
	}
	return t, nil
}

// NextText requires the parser to be on a StartTag, consumes the element's
// text content, and leaves the parser on the matching EndTag.
func (p *Parser) NextText() (string, error) {
	if p.event != StartTag {
		return "", p.expectErr(StartTag, "*", "*")
	}

	t, err := p.Next()
	if err != nil {
		return "", err
	}

	// The decoder emits a separate CharData token at every entity and CDATA
	// boundary, so entity-heavy text arrives as many small fragments. Use a
	// Builder to avoid quadratic string concatenation.
	var sb strings.Builder
	for t == Text {
		sb.WriteString(p.text)
		t, err = p.Next()
		if err != nil {
			return "", err
		}
	}
	if t != EndTag {
		return "", p.expectErr(EndTag, "*", "*")
	}
	return sb.String(), nil
}

// Skip requires the parser to be on a StartTag and consumes tokens through
// the matching end tag. It is iterative (a depth counter rather than
// recursion) so deeply nested input can't overflow the goroutine stack.
func (p *Parser) Skip() error {
	if p.event != StartTag {
		return p.expectErr(StartTag, "*", "*")
	}
	depth := 0
	for {
		tok, err := p.NextToken()
		if err != nil {
			return err
		}
		switch tok {
		case StartTag:
			depth++
		case EndTag:
			if depth == 0 {
				return nil
			}
			depth--
		case EndDocument:
			return errors.New("xpp: document ended while skipping element")
		}
	}
}

// DecodeElement requires the parser to be on a StartTag and unmarshals the
// element into v using encoding/xml. On success the cursor is left on the
// element's end tag. On failure the decoder has stopped at an unknown
// position inside the element, so the parser is poisoned: DecodeElement
// returns the decoder's error and every later call returns the wrapped form.
func (p *Parser) DecodeElement(v any) error {
	if p.err != nil {
		return p.err
	}
	if p.event != StartTag {
		return p.expectErr(StartTag, "*", "*")
	}

	start := p.token.(xml.StartElement)
	name, space := p.name, p.space

	if err := p.decoder.DecodeElement(v, &start); err != nil {
		p.err = fmt.Errorf("xpp: parser state desynced by DecodeElement error: %w", err)
		return err
	}

	// The decoder consumed through the matching end tag; present the cursor
	// as that end tag. The pending pop keeps Depth, Namespaces and BaseURL
	// describing the element until the next advancement, exactly as for an
	// end tag delivered by NextToken.
	p.resetTokenState()
	p.token = nil
	p.event = EndTag
	p.name = name
	p.space = space
	p.pendingPop = true
	return nil
}

// Event returns the type of the token the parser is on. Before the first
// advancement call it is StartDocument.
func (p *Parser) Event() EventType { return p.event }

// Name returns the local name of the current start or end tag.
func (p *Parser) Name() string { return p.name }

// Space returns the namespace URI of the current start or end tag.
func (p *Parser) Space() string { return p.space }

// Text returns the content of the current Text, Comment or Directive token.
func (p *Parser) Text() string { return p.text }

// Depth returns the element nesting depth of the current token. An EndTag
// reports the same depth as its matching StartTag; the root element is
// depth 1.
func (p *Parser) Depth() int { return p.depth }

// Attrs returns the attributes of the current StartTag. The slice is the
// parser's live per-token slice: it is valid until the next advancement
// call, and callers may modify attribute values in place (later reads
// through Attribute see the modification).
func (p *Parser) Attrs() []xml.Attr { return p.attrs }

// Attribute returns the value of the named attribute on the current
// StartTag, or "" if absent. Matching is exact and prefers an un-namespaced
// attribute; a namespaced attribute is returned only when no plain one
// shares the local name.
func (p *Parser) Attribute(name string) string {
	var fallback string
	found := false
	for _, attr := range p.attrs {
		if attr.Name.Local == name {
			if attr.Name.Space == "" {
				return attr.Value
			}
			if !found {
				fallback = attr.Value
				found = true
			}
		}
	}
	return fallback
}

// IsWhitespace reports whether the current Text token is entirely
// whitespace.
func (p *Parser) IsWhitespace() bool { return strings.TrimSpace(p.text) == "" }

// InputOffset returns the input stream byte offset of the current decoder
// position.
func (p *Parser) InputOffset() int64 {
	if p.decoder == nil {
		return 0
	}
	return p.decoder.InputOffset()
}

// Err returns the sticky error, or nil while the parser is healthy. It is
// set by a decoder error or a failed DecodeElement, after which every
// advancement call returns it.
func (p *Parser) Err() error { return p.err }

// Expect returns nil when the parser is on the given event with the given
// local name. Name matching is case-insensitive, a documented leniency for
// real-world feed input; "*" matches any name.
func (p *Parser) Expect(event EventType, name string) error {
	return p.ExpectAll(event, "*", name)
}

// ExpectAll is Expect with an additional namespace assertion, matched the
// same way.
func (p *Parser) ExpectAll(event EventType, space, name string) error {
	if p.event == event &&
		(space == "*" || strings.EqualFold(p.space, space)) &&
		(name == "*" || strings.EqualFold(p.name, name)) {
		return nil
	}
	return &ExpectError{
		WantEvent: event, WantSpace: space, WantName: name,
		GotEvent: p.event, GotSpace: p.space, GotName: p.name,
		Offset: p.InputOffset(),
	}
}

// Namespaces returns the prefix -> URI bindings in scope for the current
// element, with prefix case preserved. The default namespace is bound to
// the empty prefix. The map is a snapshot, safe to retain and modify.
func (p *Parser) Namespaces() map[string]string {
	out := map[string]string{}
	if n := len(p.nsStack); n > 0 {
		for k, v := range p.nsStack[n-1].bindings {
			out[k] = v
		}
	}
	return out
}

// PrefixForURI returns the most recently declared in-scope prefix bound to
// uri. It reports ok=false when no in-scope prefix is bound to it.
func (p *Parser) PrefixForURI(uri string) (prefix string, ok bool) {
	uri = strings.TrimSpace(uri)
	for i := len(p.nsStack) - 1; i >= 0; i-- {
		decls := p.nsStack[i].decls
		for j := len(decls) - 1; j >= 0; j-- {
			if decls[j].uri != uri {
				continue
			}
			// The declaration must not be shadowed by an inner
			// redeclaration of the same prefix to a different URI.
			if cur, bound := p.currentBinding(decls[j].prefix); bound && cur == uri {
				return decls[j].prefix, true
			}
		}
	}
	return "", false
}

func (p *Parser) currentBinding(prefix string) (string, bool) {
	if n := len(p.nsStack); n > 0 {
		uri, ok := p.nsStack[n-1].bindings[prefix]
		return uri, ok
	}
	return "", false
}

// BaseURL returns the xml:base in scope for the current element, or nil when
// none is declared. Nested xml:base values resolve against their parent per
// RFC 3986. An unparseable xml:base is treated as absent (the element
// inherits its parent's base). Resolving document URLs against the base is
// the caller's concern.
func (p *Parser) BaseURL() *url.URL {
	if n := len(p.baseStack); n > 0 {
		return p.baseStack[n-1]
	}
	return nil
}

func (p *Parser) processToken(t xml.Token) {
	switch tt := t.(type) {
	case xml.StartElement:
		p.depth++
		p.attrs = tt.Attr
		p.name = tt.Name.Local
		p.space = tt.Name.Space
		p.event = StartTag
		p.pushNamespaces(tt)
		p.pushBase()
	case xml.EndElement:
		p.name = tt.Name.Local
		p.space = tt.Name.Space
		p.event = EndTag
		p.pendingPop = true
	case xml.CharData:
		p.text = string(tt)
		p.event = Text
	case xml.Comment:
		p.text = string(tt)
		p.event = Comment
	case xml.ProcInst:
		p.text = fmt.Sprintf("%s %s", tt.Target, string(tt.Inst))
		p.event = ProcessingInstruction
	case xml.Directive:
		p.text = string(tt)
		p.event = Directive
	}
}

func (p *Parser) applyPendingPop() {
	if !p.pendingPop {
		return
	}
	p.pendingPop = false
	p.depth--
	if n := len(p.nsStack); n > 0 {
		p.nsStack = p.nsStack[:n-1]
	}
	if n := len(p.baseStack); n > 0 {
		p.baseStack = p.baseStack[:n-1]
	}
}

func (p *Parser) resetTokenState() {
	p.attrs = nil
	p.name = ""
	p.space = ""
	p.text = ""
}

func (p *Parser) pushNamespaces(t xml.StartElement) {
	merged := map[string]string{}
	if n := len(p.nsStack); n > 0 {
		for k, v := range p.nsStack[n-1].bindings {
			merged[k] = v
		}
	}
	var decls []nsDecl
	for _, attr := range t.Attr {
		switch {
		case attr.Name.Space == "xmlns":
			d := nsDecl{prefix: attr.Name.Local, uri: strings.TrimSpace(attr.Value)}
			merged[d.prefix] = d.uri
			decls = append(decls, d)
		case attr.Name.Space == "" && attr.Name.Local == "xmlns":
			d := nsDecl{prefix: "", uri: strings.TrimSpace(attr.Value)}
			merged[d.prefix] = d.uri
			decls = append(decls, d)
		}
	}
	p.nsStack = append(p.nsStack, nsScope{bindings: merged, decls: decls})
}

func (p *Parser) pushBase() {
	var parent *url.URL
	if n := len(p.baseStack); n > 0 {
		parent = p.baseStack[n-1]
	}

	var raw string
	for _, attr := range p.attrs {
		if attr.Name.Local == "base" && attr.Name.Space == xmlNSURI {
			raw = attr.Value
			break
		}
	}
	if raw == "" {
		p.baseStack = append(p.baseStack, parent)
		return
	}
	u, err := url.Parse(raw)
	if err != nil {
		// An unparseable xml:base is treated as absent; the element
		// inherits its parent's base.
		p.baseStack = append(p.baseStack, parent)
		return
	}
	if parent != nil {
		u = parent.ResolveReference(u)
	}
	p.baseStack = append(p.baseStack, u)
}

func (p *Parser) expectErr(event EventType, space, name string) *ExpectError {
	return &ExpectError{
		WantEvent: event, WantSpace: space, WantName: name,
		GotEvent: p.event, GotSpace: p.space, GotName: p.name,
		Offset: p.InputOffset(),
	}
}
