package xpp_test

import (
	"bytes"
	"io"
	"reflect"
	"testing"

	xpp "github.com/mmcdole/goxpp"
)

// Small assertion helpers so this package has no test dependencies.

func eq(t *testing.T, got, want interface{}) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("not equal:\n  got:  %#v\n  want: %#v", got, want)
	}
}

func noErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func hasErr(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Error("expected an error, got nil")
	}
}

func lenIs(t *testing.T, got, want int) {
	t.Helper()
	if got != want {
		t.Errorf("length = %d, want %d", got, want)
	}
}

func TestEventName(t *testing.T) {
	var eventNameTests = []struct {
		event    xpp.XMLEventType
		expected string
	}{
		{xpp.StartTag, "StartTag"},
		{xpp.EndTag, "EndTag"},
		{xpp.StartDocument, "StartDocument"},
		{xpp.EndDocument, "EndDocument"},
		{xpp.ProcessingInstruction, "ProcessingInstruction"},
		{xpp.Directive, "Directive"},
		{xpp.Comment, "Comment"},
		{xpp.Text, "Text"},
		{xpp.IgnorableWhitespace, "IgnorableWhitespace"},
	}

	p := xpp.XMLPullParser{}
	for _, test := range eventNameTests {
		actual := p.EventName(test.event)
		eq(t, actual, test.expected)
	}
}

func TestSpaceStackSelfClosingTag(t *testing.T) {
	crReader := func(charset string, input io.Reader) (io.Reader, error) {
		return input, nil
	}
	r := bytes.NewBufferString(`<a:y xmlns:a="z"/><x>foo</x>`)
	p := xpp.NewXMLPullParser(r, false, crReader)
	toNextStart(t, p)
	eq(t, p.Spaces, map[string]string{"z": "a"})
	toNextStart(t, p)
	eq(t, p.Spaces, map[string]string{})
}

func TestSpaceStackNestedTag(t *testing.T) {
	crReader := func(charset string, input io.Reader) (io.Reader, error) {
		return input, nil
	}
	r := bytes.NewBufferString(`<y xmlns:a="z"><a:x>foo</a:x></y><w></w>`)
	p := xpp.NewXMLPullParser(r, false, crReader)
	toNextStart(t, p)
	eq(t, p.Spaces, map[string]string{"z": "a"})
	toNextStart(t, p)
	eq(t, p.Spaces, map[string]string{"z": "a"})
	toNextStart(t, p)
	eq(t, p.Spaces, map[string]string{})
}

func TestDecodeElementDepth(t *testing.T) {
	crReader := func(charset string, input io.Reader) (io.Reader, error) {
		return input, nil
	}
	r := bytes.NewBufferString(`<root><d2>foo</d2><d2>bar</d2></root>`)
	p := xpp.NewXMLPullParser(r, false, crReader)

	type v struct{}

	// move to root
	p.NextTag()
	eq(t, p.Name, "root")
	eq(t, p.Depth, 1)

	// decode first <d2>
	p.NextTag()
	eq(t, p.Name, "d2")
	eq(t, p.Depth, 2)
	p.DecodeElement(&v{})

	// decode second <d2>
	p.NextTag()
	eq(t, p.Name, "d2")
	eq(t, p.Depth, 2) // should still be 2, not 3
	p.DecodeElement(&v{})
}

func TestDecodeElementNamespaceStack(t *testing.T) {
	crReader := func(charset string, input io.Reader) (io.Reader, error) {
		return input, nil
	}
	// The first <d2> declares its own namespace (b:w). After decoding it, that
	// scope must be popped so the parser is back to root's namespaces.
	r := bytes.NewBufferString(`<root xmlns:a="z"><d2 xmlns:b="w">foo</d2><d2>bar</d2></root>`)
	p := xpp.NewXMLPullParser(r, false, crReader)

	type v struct{}

	p.NextTag() // root
	eq(t, p.Spaces, map[string]string{"z": "a"})
	lenIs(t, len(p.SpacesStack), 1)

	p.NextTag() // first <d2>, adds b:w
	eq(t, p.Spaces, map[string]string{"z": "a", "w": "b"})
	lenIs(t, len(p.SpacesStack), 2)

	p.DecodeElement(&v{})
	// Scope must be back to root's: b:w no longer leaks and the stack shrank.
	eq(t, p.Spaces, map[string]string{"z": "a"})
	lenIs(t, len(p.SpacesStack), 1)

	p.NextTag() // second <d2>
	eq(t, p.Spaces, map[string]string{"z": "a"})
}

func TestXMLBase(t *testing.T) {
	crReader := func(charset string, input io.Reader) (io.Reader, error) {
		return input, nil
	}
	r := bytes.NewBufferString(`<root xml:base="https://example.org/path/"><d2 xml:base="relative">foo</d2><d2 xml:base="/absolute">bar</d2><d2>baz</d2></root>`)
	p := xpp.NewXMLPullParser(r, false, crReader)

	type v struct{}

	// move to root
	p.NextTag()
	eq(t, p.Name, "root")
	eq(t, p.BaseStack.Top().String(), "https://example.org/path/")

	// decode first <d2>
	p.NextTag()
	eq(t, p.Name, "d2")
	eq(t, p.BaseStack.Top().String(), "https://example.org/path/relative")

	resolved, err := p.XmlBaseResolveUrl("test")
	noErr(t, err)
	eq(t, resolved.String(), "https://example.org/path/relative/test")
	p.DecodeElement(&v{})

	// decode second <d2>
	p.NextTag()
	eq(t, p.Name, "d2")
	eq(t, p.BaseStack.Top().String(), "https://example.org/absolute")
	p.DecodeElement(&v{})

	// ensure xml:base is still set to root element's base
	p.NextTag()
	eq(t, p.Name, "d2")
	eq(t, p.BaseStack.Top().String(), "https://example.org/path/")
}

func TestXmlBaseResolveUrlDoesNotMutateBase(t *testing.T) {
	crReader := func(charset string, input io.Reader) (io.Reader, error) {
		return input, nil
	}
	r := bytes.NewBufferString(`<root xml:base="https://example.org/a/b"><d/></root>`)
	p := xpp.NewXMLPullParser(r, false, crReader)

	p.NextTag() // root, base is https://example.org/a/b
	before := p.BaseStack.Top().String()

	resolved, err := p.XmlBaseResolveUrl("x")
	noErr(t, err)
	eq(t, resolved.String(), "https://example.org/a/b/x")

	// The stacked base must be unchanged by the resolution.
	eq(t, p.BaseStack.Top().String(), before)
}

func TestXmlBaseSurvivesContainerClose(t *testing.T) {
	crReader := func(charset string, input io.Reader) (io.Reader, error) {
		return input, nil
	}
	// <child> has no xml:base of its own; closing it must not discard the root's
	// base for the following <after>.
	r := bytes.NewBufferString(`<root xml:base="http://example.org/a/"><child></child><after></after></root>`)
	p := xpp.NewXMLPullParser(r, false, crReader)

	p.NextTag() // <root>
	p.NextTag() // <child>
	p.NextTag() // </child>  (processEndToken pops a base)
	p.NextTag() // <after>
	eq(t, p.Name, "after")

	got, err := p.XmlBaseResolveUrl("rel")
	noErr(t, err)
	if got == nil || got.String() != "http://example.org/a/rel" {
		t.Errorf("resolved = %v, want http://example.org/a/rel", got)
	}
}

func toNextStart(t *testing.T, p *xpp.XMLPullParser) {
	for {
		tok, err := p.NextToken()
		if err != nil {
			t.Error(err)
			t.FailNow()
		}
		if tok == xpp.StartTag {
			break
		}
	}
}

func TestNextText(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{
			name:     "simple text",
			input:    "<root>simple text</root>",
			expected: "simple text",
			wantErr:  false,
		},
		{
			name:     "empty text",
			input:    "<root></root>",
			expected: "",
			wantErr:  false,
		},
		{
			name:     "mixed content",
			input:    "<root>text<child/>text2</root>",
			expected: "",
			wantErr:  true,
		},
		{
			name:     "whitespace text",
			input:    "<root>  \t\n  </root>",
			expected: "  \t\n  ",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := xpp.NewXMLPullParser(bytes.NewBufferString(tt.input), true, nil)

			// Move to first start tag
			_, err := p.NextTag()
			noErr(t, err)

			result, err := p.NextText()
			if tt.wantErr {
				hasErr(t, err)
				return
			}

			noErr(t, err)
			eq(t, result, tt.expected)
		})
	}
}

func TestSkip(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name: "skip simple element",
			input: `<root>
						<skip>content to skip</skip>
						<keep>content to keep</keep>
					</root>`,
			wantErr: false,
		},
		{
			name: "skip nested elements",
			input: `<root>
						<skip>
							<child1>skip this</child1>
							<child2>and this</child2>
						</skip>
						<keep>content to keep</keep>
					</root>`,
			wantErr: false,
		},
		{
			name: "skip with attributes",
			input: `<root>
						<skip attr="value">
							<child attr2="value2">skip this</child>
						</skip>
						<keep>content to keep</keep>
					</root>`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := xpp.NewXMLPullParser(bytes.NewBufferString(tt.input), true, nil)

			// Move to root
			_, err := p.NextTag()
			noErr(t, err)

			// Move to skip element
			_, err = p.NextTag()
			noErr(t, err)

			// Skip the element
			err = p.Skip()
			if tt.wantErr {
				hasErr(t, err)
				return
			}
			noErr(t, err)

			// Verify we're at the keep element
			_, err = p.NextTag()
			noErr(t, err)
			eq(t, p.Name, "keep")
		})
	}
}

func TestSpecialCases(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedTypes []xpp.XMLEventType
	}{
		{
			name:  "processing instruction",
			input: `<?target data?><root/>`,
			expectedTypes: []xpp.XMLEventType{
				xpp.ProcessingInstruction,
				xpp.StartTag,
				xpp.EndTag,
				xpp.EndDocument,
			},
		},
		{
			name:  "comments",
			input: `<!-- comment --><root/>`,
			expectedTypes: []xpp.XMLEventType{
				xpp.Comment,
				xpp.StartTag,
				xpp.EndTag,
				xpp.EndDocument,
			},
		},
		{
			name:  "mixed content",
			input: `<root>text<child/>text</root>`,
			expectedTypes: []xpp.XMLEventType{
				xpp.StartTag,
				xpp.Text,
				xpp.StartTag,
				xpp.EndTag,
				xpp.Text,
				xpp.EndTag,
				xpp.EndDocument,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Starting test case: %s with input: %s", tt.name, tt.input)
			p := xpp.NewXMLPullParser(bytes.NewBufferString(tt.input), true, nil)

			var events []xpp.XMLEventType
			for {
				event, err := p.NextToken()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				events = append(events, event)
				t.Logf("Event: %s (Name: %s, Text: %q)", p.EventName(event), p.Name, p.Text)

				// Stop when we reach EndDocument
				if event == xpp.EndDocument {
					break
				}
			}

			eq(t, events, tt.expectedTypes)
		})
	}
}

// A DecodeElement unmarshal error leaves the decoder mid-element. The parser
// must refuse to continue rather than run with desynced stacks, which used to
// end in a slice-bounds panic at the enclosing end tags (issue #28).
func TestDecodeElementErrorPoisonsParser(t *testing.T) {
	doc := `<root><d2><inner><n>notanumber</n><m>y</m></inner></d2><d3/></root>`
	p := xpp.NewXMLPullParser(bytes.NewReader([]byte(doc)), false, nil)

	// Position on <d2>.
	for {
		tok, err := p.NextToken()
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		if tok == xpp.StartTag && p.Name == "d2" {
			break
		}
	}

	var v struct {
		Inner struct {
			N int `xml:"n"`
		} `xml:"inner"`
	}
	if err := p.DecodeElement(&v); err == nil {
		t.Fatal("DecodeElement should fail on notanumber -> int")
	}

	// Every subsequent call must return an error; pulling tokens through the
	// rest of the document must not panic at </d2> or </root>.
	for i := 0; i < 20; i++ {
		if _, err := p.NextToken(); err == nil {
			t.Fatalf("NextToken call %d after failed DecodeElement should error", i)
		}
	}
	if _, err := p.Next(); err == nil {
		t.Fatal("Next after failed DecodeElement should error")
	}
	if _, err := p.NextTag(); err == nil {
		t.Fatal("NextTag after failed DecodeElement should error")
	}
	if err := p.Skip(); err == nil {
		t.Fatal("Skip after failed DecodeElement should error")
	}
}

// A successful DecodeElement must not set the sticky error; the parser
// continues normally.
func TestDecodeElementSuccessDoesNotPoison(t *testing.T) {
	doc := `<root><d2><n>42</n></d2><d3>x</d3></root>`
	p := xpp.NewXMLPullParser(bytes.NewReader([]byte(doc)), false, nil)

	for {
		tok, err := p.NextToken()
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		if tok == xpp.StartTag && p.Name == "d2" {
			break
		}
	}

	var v struct {
		N int `xml:"n"`
	}
	if err := p.DecodeElement(&v); err != nil {
		t.Fatalf("DecodeElement: %v", err)
	}
	if v.N != 42 {
		t.Fatalf("N = %d, want 42", v.N)
	}

	tok, err := p.NextTag()
	if err != nil {
		t.Fatalf("NextTag after successful DecodeElement: %v", err)
	}
	if tok != xpp.StartTag || p.Name != "d3" {
		t.Fatalf("got %s %q, want StartTag d3", p.EventName(tok), p.Name)
	}
}

// EndTag events must carry the element's namespace so ExpectAll can validate
// end tags (issue #29).
func TestEndTagSpacePopulated(t *testing.T) {
	doc := `<a:x xmlns:a="http://ns">v</a:x>`
	p := xpp.NewXMLPullParser(bytes.NewReader([]byte(doc)), false, nil)

	for {
		tok, err := p.NextToken()
		if err != nil {
			t.Fatalf("next: %v", err)
		}
		if tok == xpp.EndTag {
			break
		}
		if tok == xpp.EndDocument {
			t.Fatal("no end tag seen")
		}
	}

	if p.Space != "http://ns" {
		t.Fatalf("Space on EndTag = %q, want %q", p.Space, "http://ns")
	}
	if err := p.ExpectAll(xpp.EndTag, "http://ns", "x"); err != nil {
		t.Fatalf("ExpectAll on end tag: %v", err)
	}
}

// The synthetic EndTag left behind by DecodeElement must carry the decoded
// element's namespace too.
func TestDecodeElementEndTagSpace(t *testing.T) {
	doc := `<root xmlns:a="http://ns"><a:x><n>1</n></a:x></root>`
	p := xpp.NewXMLPullParser(bytes.NewReader([]byte(doc)), false, nil)

	for {
		tok, err := p.NextToken()
		if err != nil {
			t.Fatalf("next: %v", err)
		}
		if tok == xpp.StartTag && p.Name == "x" {
			break
		}
	}

	var v struct {
		N int `xml:"n"`
	}
	if err := p.DecodeElement(&v); err != nil {
		t.Fatalf("DecodeElement: %v", err)
	}
	if err := p.ExpectAll(xpp.EndTag, "http://ns", "x"); err != nil {
		t.Fatalf("ExpectAll on synthetic end tag: %v", err)
	}
}

// A foreign-namespaced attribute must not shadow the plain attribute with
// the same local name (issue #31).
func TestAttributePrefersUnNamespaced(t *testing.T) {
	doc := `<root xmlns:o="http://other" o:href="WRONG" href="right"/>`
	p := xpp.NewXMLPullParser(bytes.NewReader([]byte(doc)), false, nil)

	if _, err := p.NextTag(); err != nil {
		t.Fatalf("next: %v", err)
	}
	if got := p.Attribute("href"); got != "right" {
		t.Fatalf("Attribute(href) = %q, want %q", got, "right")
	}
}

// When only a namespaced attribute exists it is still returned by local name.
func TestAttributeNamespacedFallback(t *testing.T) {
	doc := `<root xmlns:o="http://other" o:href="only"/>`
	p := xpp.NewXMLPullParser(bytes.NewReader([]byte(doc)), false, nil)

	if _, err := p.NextTag(); err != nil {
		t.Fatalf("next: %v", err)
	}
	if got := p.Attribute("href"); got != "only" {
		t.Fatalf("Attribute(href) = %q, want %q", got, "only")
	}
}
