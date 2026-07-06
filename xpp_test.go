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
