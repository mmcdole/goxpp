package xpp_test

import (
	"bytes"
	"io"
	"testing"

	xpp "github.com/mmcdole/goxpp"
	"github.com/stretchr/testify/assert"
)

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
		assert.Equal(t, actual, test.expected, "Expect event name %s did not match actual event name %s.\n", test.expected, actual)
	}
}

func TestSpaceStackSelfClosingTag(t *testing.T) {
	crReader := func(charset string, input io.Reader) (io.Reader, error) {
		return input, nil
	}
	r := bytes.NewBufferString(`<a:y xmlns:a="z"/><x>foo</x>`)
	p := xpp.NewXMLPullParser(r, false, crReader)
	toNextStart(t, p)
	assert.EqualValues(t, map[string]string{"z": "a"}, p.Spaces)
	toNextStart(t, p)
	assert.EqualValues(t, map[string]string{}, p.Spaces)
}

func TestSpaceStackNestedTag(t *testing.T) {
	crReader := func(charset string, input io.Reader) (io.Reader, error) {
		return input, nil
	}
	r := bytes.NewBufferString(`<y xmlns:a="z"><a:x>foo</a:x></y><w></w>`)
	p := xpp.NewXMLPullParser(r, false, crReader)
	toNextStart(t, p)
	assert.EqualValues(t, map[string]string{"z": "a"}, p.Spaces)
	toNextStart(t, p)
	assert.EqualValues(t, map[string]string{"z": "a"}, p.Spaces)
	toNextStart(t, p)
	assert.EqualValues(t, map[string]string{}, p.Spaces)
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
	assert.Equal(t, "root", p.Name)
	assert.Equal(t, 1, p.Depth)

	// decode first <d2>
	p.NextTag()
	assert.Equal(t, "d2", p.Name)
	assert.Equal(t, 2, p.Depth)
	p.DecodeElement(&v{})

	// decode second <d2>
	p.NextTag()
	assert.Equal(t, "d2", p.Name)
	assert.Equal(t, 2, p.Depth) // should still be 2, not 3
	p.DecodeElement(&v{})
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
	assert.Equal(t, "root", p.Name)
	assert.Equal(t, "https://example.org/path/", p.BaseStack.Top().String())

	// decode first <d2>
	p.NextTag()
	assert.Equal(t, "d2", p.Name)
	assert.Equal(t, "https://example.org/path/relative", p.BaseStack.Top().String())

	resolved, err := p.XmlBaseResolveUrl("test")
	assert.NoError(t, err)
	assert.Equal(t, "https://example.org/path/relative/test", resolved.String())
	p.DecodeElement(&v{})

	// decode second <d2>
	p.NextTag()
	assert.Equal(t, "d2", p.Name)
	assert.Equal(t, "https://example.org/absolute", p.BaseStack.Top().String())
	p.DecodeElement(&v{})

	// ensure xml:base is still set to root element's base
	p.NextTag()
	assert.Equal(t, "d2", p.Name)
	assert.Equal(t, "https://example.org/path/", p.BaseStack.Top().String())
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
			assert.NoError(t, err)
			
			result, err := p.NextText()
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
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
			assert.NoError(t, err)
			
			// Move to skip element
			_, err = p.NextTag()
			assert.NoError(t, err)
			
			// Skip the element
			err = p.Skip()
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			
			// Verify we're at the keep element
			_, err = p.NextTag()
			assert.NoError(t, err)
			assert.Equal(t, "keep", p.Name)
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
			
			assert.Equal(t, tt.expectedTypes, events, "Event sequence mismatch")
		})
	}
}
