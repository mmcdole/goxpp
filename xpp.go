package xpp

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

type XMLEventType int

const (
	StartDocument XMLEventType = iota
	EndDocument
	StartTag
	EndTag
	Text
	Comment
	ProcessingInstruction
	Directive
	IgnorableWhitespace
	// TODO: CDSECT ?
)

type XMLPullParser struct {
	Depth       int
	Event       XMLEventType
	Attrs       []xml.Attr
	Name        string
	SpacePrefix string
	Space       string
	Text        string

	decoder *xml.Decoder
	token   interface{}
}

func NewXMLPullParser(r io.Reader) *XMLPullParser {
	d := xml.NewDecoder(r)
	d.Strict = false
	return &XMLPullParser{decoder: d, token: StartDocument, Depth: 0}
}

func (p *XMLPullParser) NextTag() (event XMLEventType, err error) {
	t, err := p.Next()
	if err != nil {
		return
	}

	if t != StartTag && t != EndTag {
		fmt.Printf("Whoops - got token %d - %s - %sxx\n", t, p.Name, p.Text)
		return event, errors.New("Expected StartTag or EndTag.")
	}

	return t, nil
}

func (p *XMLPullParser) Next() (event XMLEventType, err error) {
	for {
		event, err = p.NextToken()
		if err != nil {
			return event, err
		}

		if event == StartTag ||
			event == EndTag ||
			event == Text ||
			event == EndDocument {
			break
		}
	}
	return event, nil
}

func (p *XMLPullParser) NextToken() (event XMLEventType, err error) {
	// Clear any state held for the previous token
	p.resetTokenState()

	tok, err := p.decoder.Token()
	if err != nil {
		if err != io.EOF {
			return event, err
		}

		// XML decoder returns the EOF as an error
		// but we want to return it as a valid
		// EndDocument token instead
		p.token = nil
		p.Event = EndDocument
		return p.Event, nil
	}

	p.token = xml.CopyToken(tok)
	switch tt := p.token.(type) {
	case xml.StartElement:
		p.processStartToken(tt)
	case xml.EndElement:
		p.processEndToken(tt)
	case xml.CharData:
		p.processCharDataToken(tt)
	case xml.Comment:
		p.processCommentToken(tt)
	case xml.ProcInst:
		p.processProcInstToken(tt)
	case xml.Directive:
		p.processDirectiveToken(tt)
	}

	fmt.Printf("Consumed token: %d - %s\n", p.Event, p.token)

	return p.Event, nil
}

func (p *XMLPullParser) NextText() (string, error) {
	if p.Event != StartTag {
		return "", errors.New("Parser must be on StartTag to get NextText()")
	}

	t, err := p.Next()
	if err != nil {
		return "", err
	}

	if t == Text {
		result := p.Text
		nt, err := p.Next()
		if err != nil {
			return "", err
		}

		if nt != EndTag {
			return "", errors.New("Event Text must be immediately followed by EndTag")
		}

		return result, nil
	} else if t == EndTag {
		return "", nil
	} else {
		return "", errors.New("Parser must be on StartTag or Text to read text")
	}
}

func (p *XMLPullParser) Skip() error {
	for {
		tok, err := p.NextToken()
		if err != nil {
			return err
		}
		if tok == StartTag {
			if err := p.Skip(); err != nil {
				return err
			}
		} else if tok == EndTag {
			return nil
		}
	}
}

func (p *XMLPullParser) Attribute(name string) string {
	for _, attr := range p.Attrs {
		if attr.Name.Local == name {
			return attr.Value
		}
	}
	return ""
}

func (p *XMLPullParser) Matches(event XMLEventType, name string) bool {
	return p.Event == event && p.Name == name
}

func (p *XMLPullParser) MatchesAll(event XMLEventType, namespace string, name string) bool {
	return p.Event == event && p.Space == namespace && p.Name == name
}

func (p *XMLPullParser) processStartToken(t xml.StartElement) {
	p.Depth++
	p.Event = StartTag
	p.Attrs = t.Attr
	p.Name = t.Name.Local
	p.Space = t.Name.Space
}

func (p *XMLPullParser) processEndToken(t xml.EndElement) {
	p.Depth--
	p.Event = EndTag
	p.Name = t.Name.Local

}

func (p *XMLPullParser) processCharDataToken(t xml.CharData) {
	p.Text = string([]byte(t))
	p.Event = Text
	if p.isWhitespace() {
		p.Event = IgnorableWhitespace
	} else {
		p.Event = Text
	}
}

func (p *XMLPullParser) processCommentToken(t xml.Comment) {
	p.Event = Comment
	p.Text = string([]byte(t))
}

func (p *XMLPullParser) processProcInstToken(t xml.ProcInst) {
	p.Event = ProcessingInstruction
	p.Text = fmt.Sprintf("%s %s", t.Target, string(t.Inst))
}

func (p *XMLPullParser) processDirectiveToken(t xml.Directive) {
	p.Event = Directive
	p.Text = string([]byte(t))
}

func (p *XMLPullParser) resetTokenState() {
	p.Attrs = nil
	p.Name = ""
	p.Space = ""
	p.Text = ""
}

func (p *XMLPullParser) isWhitespace() bool {
	return strings.TrimSpace(p.Text) == ""
}
