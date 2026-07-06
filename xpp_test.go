package xpp_test

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strings"
	"testing"

	xpp "github.com/mmcdole/goxpp/v2"
)

func newParser(doc string) *xpp.Parser {
	d := xml.NewDecoder(bytes.NewReader([]byte(doc)))
	d.Strict = false
	return xpp.New(d)
}

// advanceTo positions the parser on the first StartTag with the given local
// name.
func advanceTo(t *testing.T, p *xpp.Parser, name string) {
	t.Helper()
	for {
		tok, err := p.NextToken()
		if err != nil {
			t.Fatalf("advanceTo(%s): %v", name, err)
		}
		if tok == xpp.StartTag && p.Name() == name {
			return
		}
		if tok == xpp.EndDocument {
			t.Fatalf("advanceTo(%s): document ended", name)
		}
	}
}

func TestEventTypeString(t *testing.T) {
	cases := map[xpp.EventType]string{
		xpp.StartDocument:         "StartDocument",
		xpp.EndDocument:           "EndDocument",
		xpp.StartTag:              "StartTag",
		xpp.EndTag:                "EndTag",
		xpp.Text:                  "Text",
		xpp.Comment:               "Comment",
		xpp.ProcessingInstruction: "ProcessingInstruction",
		xpp.Directive:             "Directive",
		xpp.EventType(99):         "EventType(99)",
	}
	for e, want := range cases {
		if got := e.String(); got != want {
			t.Errorf("String(%d) = %q, want %q", int(e), got, want)
		}
	}
}

func TestTokenWalk(t *testing.T) {
	p := newParser(`<?xml version="1.0"?><!-- c --><root a="1"><child>hi</child></root>`)

	if p.Event() != xpp.StartDocument {
		t.Fatalf("initial event = %v, want StartDocument", p.Event())
	}

	type step struct {
		event xpp.EventType
		name  string
		text  string
	}
	want := []step{
		{xpp.ProcessingInstruction, "", ""},
		{xpp.Comment, "", " c "},
		{xpp.StartTag, "root", ""},
		{xpp.StartTag, "child", ""},
		{xpp.Text, "", "hi"},
		{xpp.EndTag, "child", ""},
		{xpp.EndTag, "root", ""},
		{xpp.EndDocument, "", ""},
	}
	for i, w := range want {
		tok, err := p.NextToken()
		if err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		if tok != w.event || p.Event() != w.event {
			t.Fatalf("step %d: event = %v, want %v", i, tok, w.event)
		}
		if w.name != "" && p.Name() != w.name {
			t.Fatalf("step %d: name = %q, want %q", i, p.Name(), w.name)
		}
		if w.text != "" && p.Text() != w.text {
			t.Fatalf("step %d: text = %q, want %q", i, p.Text(), w.text)
		}
	}
}

func TestNextSkipsNonContent(t *testing.T) {
	p := newParser(`<?xml version="1.0"?><!-- c --><root><!-- inner -->x</root>`)

	tok, err := p.Next()
	if err != nil || tok != xpp.StartTag || p.Name() != "root" {
		t.Fatalf("Next = %v %q (%v), want StartTag root", tok, p.Name(), err)
	}
	tok, err = p.Next()
	if err != nil || tok != xpp.Text || p.Text() != "x" {
		t.Fatalf("Next = %v %q (%v), want Text x", tok, p.Text(), err)
	}
}

func TestEOFAfterEndDocument(t *testing.T) {
	p := newParser(`<root/>`)
	sawEnd := false
	for i := 0; i < 10; i++ {
		tok, err := p.NextToken()
		if !sawEnd {
			if err != nil {
				t.Fatalf("call %d: unexpected error %v", i, err)
			}
			if tok == xpp.EndDocument {
				sawEnd = true
			}
			continue
		}
		if !errors.Is(err, io.EOF) {
			t.Fatalf("call %d after EndDocument: err = %v, want io.EOF", i, err)
		}
		if tok != xpp.EndDocument {
			t.Fatalf("call %d after EndDocument: event = %v, want EndDocument", i, tok)
		}
	}
	if !sawEnd {
		t.Fatal("never saw EndDocument")
	}
	if p.Err() != nil {
		t.Fatalf("Err() after clean EOF = %v, want nil", p.Err())
	}
}

func TestSyntaxErrorPoisonsAndIsMatchable(t *testing.T) {
	p := newParser(`<root><unclosed>`)
	var last error
	for i := 0; i < 10; i++ {
		if _, err := p.NextToken(); err != nil {
			last = err
			break
		}
	}
	if last == nil {
		t.Fatal("truncated document produced no error")
	}
	var serr *xml.SyntaxError
	if !errors.As(last, &serr) {
		t.Fatalf("error %v is not matchable as *xml.SyntaxError", last)
	}
	if p.Err() == nil {
		t.Fatal("Err() should be sticky after a decoder error")
	}
	if _, err := p.NextToken(); err == nil {
		t.Fatal("NextToken after decoder error should keep failing")
	}
}

func TestDecodeElementErrorPoisonsParser(t *testing.T) {
	doc := `<root><d2><inner><n>notanumber</n><m>y</m></inner></d2><d3/></root>`
	p := newParser(doc)
	advanceTo(t, p, "d2")

	var v struct {
		Inner struct {
			N int `xml:"n"`
		} `xml:"inner"`
	}
	if err := p.DecodeElement(&v); err == nil {
		t.Fatal("DecodeElement should fail on notanumber -> int")
	}
	if p.Err() == nil {
		t.Fatal("Err() should be set after failed DecodeElement")
	}
	for i := 0; i < 20; i++ {
		if _, err := p.NextToken(); err == nil {
			t.Fatalf("NextToken call %d after failed DecodeElement should error", i)
		}
	}
	if _, err := p.NextTag(); err == nil {
		t.Fatal("NextTag after failed DecodeElement should error")
	}
	if err := p.DecodeElement(&v); err == nil {
		t.Fatal("DecodeElement after poison should error")
	}
}

func TestDecodeElementSuccess(t *testing.T) {
	doc := `<root xmlns:a="http://ns"><a:x><n>42</n></a:x><d3>y</d3></root>`
	p := newParser(doc)
	advanceTo(t, p, "x")
	startDepth := p.Depth()

	var v struct {
		N int `xml:"n"`
	}
	if err := p.DecodeElement(&v); err != nil {
		t.Fatalf("DecodeElement: %v", err)
	}
	if v.N != 42 {
		t.Fatalf("N = %d, want 42", v.N)
	}
	if p.Event() != xpp.EndTag || p.Name() != "x" || p.Space() != "http://ns" {
		t.Fatalf("cursor = %v %q space %q, want EndTag x http://ns", p.Event(), p.Name(), p.Space())
	}
	if p.Depth() != startDepth {
		t.Fatalf("Depth on synthetic EndTag = %d, want %d", p.Depth(), startDepth)
	}
	if p.Err() != nil {
		t.Fatalf("Err() after successful DecodeElement = %v", p.Err())
	}

	tok, err := p.NextTag()
	if err != nil || tok != xpp.StartTag || p.Name() != "d3" {
		t.Fatalf("NextTag = %v %q (%v), want StartTag d3", tok, p.Name(), err)
	}
}

func TestDecodeElementPrecondition(t *testing.T) {
	p := newParser(`<root>text</root>`)
	advanceTo(t, p, "root")
	if _, err := p.Next(); err != nil { // on Text
		t.Fatal(err)
	}
	var v struct{}
	err := p.DecodeElement(&v)
	var ee *xpp.ExpectError
	if !errors.As(err, &ee) {
		t.Fatalf("err = %v, want *ExpectError", err)
	}
	if ee.WantEvent != xpp.StartTag || ee.GotEvent != xpp.Text {
		t.Fatalf("ExpectError = %+v, want StartTag/Text", ee)
	}
}

func TestDepthEndTagMatchesStartTag(t *testing.T) {
	p := newParser(`<a><b><c/></b></a>`)
	depths := map[string][2]int{} // name -> [start, end]
	for {
		tok, err := p.NextToken()
		if err != nil {
			t.Fatal(err)
		}
		if tok == xpp.StartTag {
			d := depths[p.Name()]
			d[0] = p.Depth()
			depths[p.Name()] = d
		}
		if tok == xpp.EndTag {
			d := depths[p.Name()]
			d[1] = p.Depth()
			depths[p.Name()] = d
		}
		if tok == xpp.EndDocument {
			break
		}
	}
	want := map[string][2]int{"a": {1, 1}, "b": {2, 2}, "c": {3, 3}}
	for name, w := range want {
		if depths[name] != w {
			t.Errorf("depths[%s] = %v, want %v", name, depths[name], w)
		}
	}
}

func TestNamespaces(t *testing.T) {
	doc := `<root xmlns:DC="http://purl.org/dc" xmlns="http://default"><DC:title/></root>`
	p := newParser(doc)
	advanceTo(t, p, "title")

	ns := p.Namespaces()
	if ns["DC"] != "http://purl.org/dc" {
		t.Fatalf("Namespaces()[DC] = %q, want the URI with prefix case preserved", ns["DC"])
	}
	if ns[""] != "http://default" {
		t.Fatalf("default namespace = %q, want http://default", ns[""])
	}

	// Snapshot: mutating the returned map must not affect the parser.
	ns["DC"] = "corrupted"
	if got := p.Namespaces()["DC"]; got != "http://purl.org/dc" {
		t.Fatalf("parser bindings mutated through snapshot: %q", got)
	}
}

func TestDuplicateBindingsNotLossy(t *testing.T) {
	doc := `<root xmlns:a="http://ns" xmlns:b="http://ns"><a:x/></root>`
	p := newParser(doc)
	advanceTo(t, p, "x")

	ns := p.Namespaces()
	if ns["a"] != "http://ns" || ns["b"] != "http://ns" {
		t.Fatalf("both prefixes must be bound: %v", ns)
	}
	// Most recently declared prefix wins the reverse lookup.
	prefix, ok := p.PrefixForURI("http://ns")
	if !ok || prefix != "b" {
		t.Fatalf("PrefixForURI = %q %v, want b true", prefix, ok)
	}
}

func TestPrefixForURI(t *testing.T) {
	doc := `<root xmlns:out="http://u1"><mid xmlns:in="http://u1"><leaf/></mid></root>`
	p := newParser(doc)
	advanceTo(t, p, "leaf")

	// Innermost declaration wins.
	prefix, ok := p.PrefixForURI("http://u1")
	if !ok || prefix != "in" {
		t.Fatalf("PrefixForURI = %q %v, want in true", prefix, ok)
	}
	if _, ok := p.PrefixForURI("http://unbound"); ok {
		t.Fatal("PrefixForURI for unbound URI should report ok=false")
	}
	// Whitespace in the query is tolerated, matching declaration trimming.
	if prefix, ok := p.PrefixForURI(" http://u1 "); !ok || prefix != "in" {
		t.Fatalf("trimmed lookup = %q %v, want in true", prefix, ok)
	}
}

func TestPrefixForURIShadowed(t *testing.T) {
	// Prefix a is rebound to a different URI in the inner scope, so it is
	// no longer an in-scope prefix for the outer URI.
	doc := `<root xmlns:a="http://u1"><mid xmlns:a="http://u2"><leaf/></mid></root>`
	p := newParser(doc)
	advanceTo(t, p, "leaf")

	if _, ok := p.PrefixForURI("http://u1"); ok {
		t.Fatal("shadowed binding should not be returned")
	}
	if prefix, ok := p.PrefixForURI("http://u2"); !ok || prefix != "a" {
		t.Fatalf("PrefixForURI(u2) = %q %v, want a true", prefix, ok)
	}
}

func TestBaseURLNestedResolution(t *testing.T) {
	// A file-like base must resolve per RFC 3986: the last path segment is
	// replaced, not treated as a directory.
	doc := `<root xml:base="http://example.org/dir/file.xml"><mid xml:base="other/"><leaf/></mid></root>`
	p := newParser(doc)
	advanceTo(t, p, "leaf")

	base := p.BaseURL()
	if base == nil {
		t.Fatal("BaseURL = nil")
	}
	if got := base.String(); got != "http://example.org/dir/other/" {
		t.Fatalf("BaseURL = %q, want http://example.org/dir/other/", got)
	}
}

func TestBaseURLScopes(t *testing.T) {
	doc := `<root xml:base="http://a/"><mid xml:base="http://b/"><leaf/></mid><sib/></root>`
	p := newParser(doc)

	advanceTo(t, p, "leaf")
	if got := p.BaseURL().String(); got != "http://b/" {
		t.Fatalf("leaf base = %q, want http://b/", got)
	}

	// On mid's end tag, the base still describes mid.
	if _, err := p.NextToken(); err != nil { // </leaf>
		t.Fatal(err)
	}
	if _, err := p.NextToken(); err != nil { // </mid>
		t.Fatal(err)
	}
	if p.Event() != xpp.EndTag || p.Name() != "mid" {
		t.Fatalf("cursor = %v %q, want EndTag mid", p.Event(), p.Name())
	}
	if got := p.BaseURL().String(); got != "http://b/" {
		t.Fatalf("base on </mid> = %q, want http://b/", got)
	}

	// On the sibling, the scope has popped back to root's base.
	advanceTo(t, p, "sib")
	if got := p.BaseURL().String(); got != "http://a/" {
		t.Fatalf("sib base = %q, want http://a/", got)
	}
}

func TestBaseURLAfterDecodeElement(t *testing.T) {
	doc := `<root xml:base="http://a/"><item xml:base="sub/"><n>1</n></item><next/></root>`
	p := newParser(doc)
	advanceTo(t, p, "item")

	var v struct {
		N int `xml:"n"`
	}
	if err := p.DecodeElement(&v); err != nil {
		t.Fatal(err)
	}
	// The cursor sits on item's end tag; the base still describes item.
	if got := p.BaseURL().String(); got != "http://a/sub/" {
		t.Fatalf("base after DecodeElement = %q, want http://a/sub/", got)
	}
	advanceTo(t, p, "next")
	if got := p.BaseURL().String(); got != "http://a/" {
		t.Fatalf("base on sibling = %q, want http://a/", got)
	}
}

func TestBaseURLUnparseableInherits(t *testing.T) {
	doc := "<root xml:base=\"http://a/\"><mid xml:base=\"http://bad url\x7f::\"><leaf/></mid></root>"
	p := newParser(doc)
	advanceTo(t, p, "leaf")
	base := p.BaseURL()
	if base == nil || base.String() != "http://a/" {
		t.Fatalf("base = %v, want inherited http://a/", base)
	}
}

func TestBaseURLAbsent(t *testing.T) {
	p := newParser(`<root><leaf/></root>`)
	advanceTo(t, p, "leaf")
	if p.BaseURL() != nil {
		t.Fatalf("BaseURL = %v, want nil", p.BaseURL())
	}
}

func TestExpect(t *testing.T) {
	doc := `<a:Root xmlns:a="http://NS">v</a:Root>`
	p := newParser(doc)
	advanceTo(t, p, "Root")

	// Case-insensitive name and space, wildcard forms.
	if err := p.Expect(xpp.StartTag, "root"); err != nil {
		t.Fatalf("Expect fold: %v", err)
	}
	if err := p.ExpectAll(xpp.StartTag, "http://ns", "ROOT"); err != nil {
		t.Fatalf("ExpectAll fold: %v", err)
	}
	if err := p.ExpectAll(xpp.StartTag, "*", "*"); err != nil {
		t.Fatalf("ExpectAll wildcard: %v", err)
	}

	err := p.Expect(xpp.EndTag, "root")
	var ee *xpp.ExpectError
	if !errors.As(err, &ee) {
		t.Fatalf("err = %v, want *ExpectError", err)
	}
	if ee.WantEvent != xpp.EndTag || ee.GotEvent != xpp.StartTag || ee.GotName != "Root" {
		t.Fatalf("ExpectError fields = %+v", ee)
	}
	if !strings.Contains(err.Error(), "EndTag") || !strings.Contains(err.Error(), "StartTag") {
		t.Fatalf("Error() = %q, should mention both events", err.Error())
	}

	// End-tag namespace validation works (Space is populated on EndTag).
	if _, err := p.Next(); err != nil { // Text
		t.Fatal(err)
	}
	if _, err := p.Next(); err != nil { // EndTag
		t.Fatal(err)
	}
	if err := p.ExpectAll(xpp.EndTag, "http://ns", "root"); err != nil {
		t.Fatalf("ExpectAll on end tag: %v", err)
	}
}

func TestNextTag(t *testing.T) {
	p := newParser("<root>\n  <child/>\n</root>")
	advanceTo(t, p, "root")

	tok, err := p.NextTag()
	if err != nil || tok != xpp.StartTag || p.Name() != "child" {
		t.Fatalf("NextTag = %v %q (%v), want StartTag child", tok, p.Name(), err)
	}

	p2 := newParser(`<root>real text</root>`)
	advanceTo(t, p2, "root")
	_, err = p2.NextTag()
	var ee *xpp.ExpectError
	if !errors.As(err, &ee) {
		t.Fatalf("NextTag on text: err = %v, want *ExpectError", err)
	}
}

func TestNextText(t *testing.T) {
	p := newParser(`<root>a &amp; b<![CDATA[ & c]]></root>`)
	advanceTo(t, p, "root")
	text, err := p.NextText()
	if err != nil {
		t.Fatal(err)
	}
	if text != "a & b & c" {
		t.Fatalf("NextText = %q, want %q", text, "a & b & c")
	}
	if p.Event() != xpp.EndTag {
		t.Fatalf("cursor after NextText = %v, want EndTag", p.Event())
	}
}

func TestNextTextPreconditionAndMixedContent(t *testing.T) {
	p := newParser(`<root>t</root>`)
	if _, err := p.NextText(); err == nil {
		t.Fatal("NextText before StartTag should error")
	}

	p2 := newParser(`<root>text<child/></root>`)
	advanceTo(t, p2, "root")
	_, err := p2.NextText()
	var ee *xpp.ExpectError
	if !errors.As(err, &ee) {
		t.Fatalf("NextText on mixed content: err = %v, want *ExpectError", err)
	}
}

func TestSkip(t *testing.T) {
	doc := `<root><skipme><deep><deeper/></deep>text</skipme><after/></root>`
	p := newParser(doc)
	advanceTo(t, p, "skipme")

	if err := p.Skip(); err != nil {
		t.Fatal(err)
	}
	if p.Event() != xpp.EndTag || p.Name() != "skipme" {
		t.Fatalf("cursor after Skip = %v %q, want EndTag skipme", p.Event(), p.Name())
	}
	tok, err := p.NextTag()
	if err != nil || tok != xpp.StartTag || p.Name() != "after" {
		t.Fatalf("after Skip: %v %q (%v), want StartTag after", tok, p.Name(), err)
	}
}

func TestSkipPrecondition(t *testing.T) {
	p := newParser(`<root><a/></root>`)
	advanceTo(t, p, "a")
	// Advance to </a>.
	if _, err := p.NextToken(); err != nil {
		t.Fatal(err)
	}
	err := p.Skip()
	var ee *xpp.ExpectError
	if !errors.As(err, &ee) {
		t.Fatalf("Skip on EndTag: err = %v, want *ExpectError", err)
	}
	if ee.WantEvent != xpp.StartTag || ee.GotEvent != xpp.EndTag {
		t.Fatalf("ExpectError fields = %+v", ee)
	}
}

func TestAttributePreference(t *testing.T) {
	doc := `<root xmlns:o="http://other" o:href="WRONG" href="right" o:only="fallback"/>`
	p := newParser(doc)
	advanceTo(t, p, "root")

	if got := p.Attribute("href"); got != "right" {
		t.Fatalf("Attribute(href) = %q, want right", got)
	}
	if got := p.Attribute("only"); got != "fallback" {
		t.Fatalf("Attribute(only) = %q, want fallback", got)
	}
	if got := p.Attribute("absent"); got != "" {
		t.Fatalf("Attribute(absent) = %q, want empty", got)
	}
}

func TestAttrsLiveMutation(t *testing.T) {
	// gofeed rewrites attribute values in place to resolve relative URLs;
	// the live-slice contract makes later reads see the modification.
	p := newParser(`<root href="relative.html"/>`)
	advanceTo(t, p, "root")

	attrs := p.Attrs()
	for i := range attrs {
		if attrs[i].Name.Local == "href" {
			attrs[i].Value = "http://example.org/absolute.html"
		}
	}
	if got := p.Attribute("href"); got != "http://example.org/absolute.html" {
		t.Fatalf("Attribute after mutation = %q, want the rewritten value", got)
	}
}

func TestZeroValueParser(t *testing.T) {
	var p xpp.Parser
	if _, err := p.NextToken(); err == nil {
		t.Fatal("zero-value parser should error, not panic")
	}
	if p.Err() == nil {
		t.Fatal("zero-value parser error should be sticky")
	}
	if p.BaseURL() != nil || p.Attribute("x") != "" || p.Depth() != 0 {
		t.Fatal("zero-value accessors should return zero values")
	}
}

func TestInputOffset(t *testing.T) {
	p := newParser(`<root><child/></root>`)
	advanceTo(t, p, "root")
	first := p.InputOffset()
	advanceTo(t, p, "child")
	if p.InputOffset() <= first {
		t.Fatalf("InputOffset did not progress: %d then %d", first, p.InputOffset())
	}
}

func TestIsWhitespace(t *testing.T) {
	p := newParser("<root>  \n\t </root>")
	advanceTo(t, p, "root")
	if _, err := p.NextToken(); err != nil {
		t.Fatal(err)
	}
	if p.Event() != xpp.Text || !p.IsWhitespace() {
		t.Fatalf("event %v IsWhitespace %v, want whitespace Text", p.Event(), p.IsWhitespace())
	}
}
