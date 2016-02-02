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
	IgnorableWhitespace // TODO: ?
	// TODO: CDSECT ?
)

type XMLPullParser struct {
	Depth int
	Event XMLEventType
	Attrs []xml.Attr
	Name  string
	Space string
	Text  string

	decoder   *xml.Decoder
	token     interface{}
	peekToken interface{}
	peekEvent XMLEventType
	peekErr   error
}

func NewXMLPullParser(r io.Reader) *XMLPullParser {
	d := xml.NewDecoder(r)
	d.Strict = false
	return &XMLPullParser{
		decoder: d,
		Event:   StartDocument,
		Depth:   0,
	}
}

func (p *XMLPullParser) NextTag() (event XMLEventType, err error) {
	t, err := p.Next()
	if err != nil {
		return
	}

	if t != StartTag && t != EndTag {
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

		if event == Text {
			// Coalesce all contiguous text events together
			text := p.Text
			for p.peekEvent == Text && p.peekErr == nil {
				p.NextToken()
				text += p.Text
			}
			p.Text = text
			break
		}

		if event == StartTag ||
			event == EndTag ||
			event == EndDocument {
			break
		}
	}
	return event, nil
}

func (p *XMLPullParser) NextToken() (event XMLEventType, err error) {
	// Clear any state held for the previous token
	p.resetTokenState()

	// If there was an error when peeking return it now
	if p.peekErr != nil {
		return event, p.peekErr
	}

	// If the peek token was EndDocument, dont bother
	// retrieving any more tokens.  Just return EndDocument
	if p.peekEvent == EndDocument {
		return EndDocument, nil
	}

	// Switch peek token/event to the current token/event
	p.Event = p.peekEvent
	p.token = p.peekToken
	p.processToken(p.token)

	// Peek the next token/event
	peekToken, err := p.decoder.Token()
	if err != nil {
		if err != io.EOF {
			p.peekErr = err
		}

		// XML decoder returns the EOF as an error
		// but we want to return it as a valid
		// EndDocument token instead
		p.peekToken = nil
		p.peekEvent = EndDocument
	}
	p.peekToken = xml.CopyToken(peekToken)
	p.peekEvent = p.eventType(peekToken)

	// Return current event (previously the peek token)
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

func (p *XMLPullParser) Expect(event XMLEventType, name string) (err error) {
	return p.ExpectAll(event, "*", name)
}

func (p *XMLPullParser) ExpectAll(event XMLEventType, space string, name string) (err error) {
	if !(p.Event == event && (p.Space == space || space == "*") && (p.Name == name || name == "*")) {
		err = fmt.Errorf("Expected Space:%s Name:%s Event:%s but got Space:%s Name:%s Event:%s", space, name, p.eventName(event), p.Space, p.Name, p.eventName(p.Event))
	}
	return
}

func (p *XMLPullParser) processToken(t xml.Token) {
	switch tt := t.(type) {
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
}

func (p *XMLPullParser) processStartToken(t xml.StartElement) {
	p.Depth++
	p.Attrs = t.Attr
	p.Name = t.Name.Local
	p.Space = t.Name.Space
}

func (p *XMLPullParser) processEndToken(t xml.EndElement) {
	p.Depth--
	p.Name = t.Name.Local
}

func (p *XMLPullParser) processCharDataToken(t xml.CharData) {
	p.Text = string([]byte(t))
}

func (p *XMLPullParser) processCommentToken(t xml.Comment) {
	p.Text = string([]byte(t))
}

func (p *XMLPullParser) processProcInstToken(t xml.ProcInst) {
	p.Event = ProcessingInstruction
	p.Text = fmt.Sprintf("%s %s", t.Target, string(t.Inst))
}

func (p *XMLPullParser) processDirectiveToken(t xml.Directive) {
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

func (p *XMLPullParser) eventName(e XMLEventType) (name string) {
	switch e {
	case StartTag:
		name = "StartTag"
	case EndTag:
		name = "EndTag"
	case ProcessingInstruction:
		name = "ProcessingInstruction"
	case Directive:
		name = "Directive"
	case Comment:
		name = "Comment"
	case Text:
		name = "Text"
	case IgnorableWhitespace:
		name = "IgnorableWhitespace"
	}
	return
}

func (p *XMLPullParser) eventType(t xml.Token) (event XMLEventType) {
	switch t.(type) {
	case xml.StartElement:
		event = StartTag
	case xml.EndElement:
		event = EndTag
	case xml.CharData:
		event = Text
	case xml.Comment:
		event = Comment
	case xml.ProcInst:
		event = ProcessingInstruction
	case xml.Directive:
		event = Directive
	}
	return
}
