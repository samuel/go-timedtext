// Package ttml reads and writes the subset of TTML (Timed Text Markup Language) used for lyrics, as in
// Apple Music and Spotify. Elements and attributes outside that subset are dropped. Times on nested
// elements are read as absolute, as lyrics files use them, not relative to the parent.
//
// See https://www.w3.org/TR/ttml2/.
package ttml

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/encoding"
	"samuelks.com/timedtext/charset"
	"samuelks.com/timedtext/internal/clock"
)

const (
	nsTT       = "http://www.w3.org/ns/ttml"
	nsMetadata = "http://www.w3.org/ns/ttml#metadata"
	nsITunes   = "http://music.apple.com/lyric-ttml-internal"
)

// Document is a TTML lyrics document.
type Document struct {
	Lang        string
	Title       string
	Songwriters []string // From Apple's iTunesMetadata
	Agents      []Agent
	Duration    time.Duration // Zero when unknown
	Divs        []Div
}

// Agent is a performer that paragraphs refer to by ID.
type Agent struct {
	ID   string
	Type string // e.g. "person" or "group"
	Name string
}

// Div is a group of paragraphs, usually a song section.
type Div struct {
	Begin      time.Duration
	End        time.Duration
	Paragraphs []Paragraph
}

// Paragraph is one lyric line.
type Paragraph struct {
	ID    string // xml:id
	Begin time.Duration
	End   time.Duration
	Agent string // Agent ID
	Spans []Span
}

// Span is a run of text. Line breaks are spans with the text "\n".
type Span struct {
	Timed bool // Begin and End are set
	Begin time.Duration
	End   time.Duration
	Role  string // e.g. "x-bg" for background vocals
	Text  string
}

// Text returns the paragraph's text with whitespace collapsed.
func (p *Paragraph) Text() string {
	var b strings.Builder
	for _, s := range p.Spans {
		b.WriteString(s.Text)
	}
	return b.String()
}

// SyntaxError reports malformed input.
type SyntaxError struct {
	Line int // 1-based line number
	Err  error
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("ttml: line %d: %s", e.Line, e.Err)
}

func (e *SyntaxError) Unwrap() error {
	return e.Err
}

// spanContext is the timing and role of an open span element.
type spanContext struct {
	timed      bool
	begin, end time.Duration
	role       string
}

type decoder struct {
	xd    *xml.Decoder
	doc   *Document
	stack []string // Local names of open elements
	spans []spanContext
	para  *Paragraph
	// textSpan is the index in para.Spans of the span that character data extends, or -1.
	textSpan int
}

// reXMLEncoding matches the encoding in an XML declaration.
var reXMLEncoding = regexp.MustCompile(`^\s*<\?xml[^>]*?\sencoding\s*=\s*["']([^"']+)["']`)

// DecodeOptions controls how Decode interprets its input.
type DecodeOptions struct {
	// Charset is the encoding of input that has no byte order mark, isn't valid UTF-8, and has no known
	// encoding in its XML declaration. Defaults to Windows-1252.
	Charset encoding.Encoding
}

// Decode reads a TTML document and converts it to UTF-8. A byte order mark or UTF-16 is detected first,
// then valid UTF-8, then the XML declaration's encoding, then opt.Charset.
func Decode(r io.Reader, opt DecodeOptions) (*Document, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var declared string
	if m := reXMLEncoding.FindSubmatch(raw); m != nil {
		declared = string(m[1])
	}
	data, err := charset.Decode(raw, declared, opt.Charset)
	if err != nil {
		return nil, err
	}
	xd := xml.NewDecoder(bytes.NewReader(data))
	// The input is already UTF-8, whatever the declaration says.
	xd.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	d := &decoder{xd: xd, doc: &Document{}, textSpan: -1}
	var sawRoot bool
	for {
		tok, err := d.xd.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if se, ok := errors.AsType[*xml.SyntaxError](err); ok {
			return nil, &SyntaxError{Line: se.Line, Err: errors.New(se.Msg)}
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if !sawRoot && (t.Name.Local != "tt" || (t.Name.Space != nsTT && t.Name.Space != "")) {
				return nil, d.errorf("root element is %s, expected tt", t.Name.Local)
			}
			sawRoot = true
			if err := d.start(t); err != nil {
				return nil, err
			}
		case xml.EndElement:
			d.end()
		case xml.CharData:
			d.charData(string(t))
		}
	}
	if !sawRoot {
		return nil, d.errorf("missing tt element")
	}
	for i := range d.doc.Divs {
		for j := range d.doc.Divs[i].Paragraphs {
			trimSpans(&d.doc.Divs[i].Paragraphs[j])
		}
	}
	return d.doc, nil
}

func (d *decoder) errorf(format string, args ...any) error {
	line, _ := d.xd.InputPos()
	return &SyntaxError{Line: line, Err: fmt.Errorf(format, args...)}
}

func (d *decoder) parent() string {
	if len(d.stack) == 0 {
		return ""
	}
	return d.stack[len(d.stack)-1]
}

func (d *decoder) start(t xml.StartElement) error {
	parent := d.parent()
	d.stack = append(d.stack, t.Name.Local)
	d.textSpan = -1
	switch t.Name.Local {
	case "tt":
		d.doc.Lang = attr(t, "lang")
	case "agent":
		d.doc.Agents = append(d.doc.Agents, Agent{ID: attr(t, "id"), Type: attr(t, "type")})
	case "songwriter":
		if parent == "songwriters" {
			d.doc.Songwriters = append(d.doc.Songwriters, "")
		}
	case "body":
		dur, err := d.parseTime(t, "dur")
		if err != nil {
			return err
		}
		d.doc.Duration = dur
	case "div":
		begin, end, _, err := d.timing(t)
		if err != nil {
			return err
		}
		d.doc.Divs = append(d.doc.Divs, Div{Begin: begin, End: end})
	case "p":
		begin, end, _, err := d.timing(t)
		if err != nil {
			return err
		}
		if len(d.doc.Divs) == 0 || parent != "div" {
			d.doc.Divs = append(d.doc.Divs, Div{})
		}
		div := &d.doc.Divs[len(d.doc.Divs)-1]
		div.Paragraphs = append(div.Paragraphs, Paragraph{
			ID:    attr(t, "id"),
			Begin: begin,
			End:   end,
			Agent: attr(t, "agent"),
		})
		d.para = &div.Paragraphs[len(div.Paragraphs)-1]
		d.spans = nil
	case "span":
		if d.para == nil {
			return nil
		}
		begin, end, timed, err := d.timing(t)
		if err != nil {
			return err
		}
		ctx := spanContext{timed: timed, begin: begin, end: end, role: attr(t, "role")}
		if n := len(d.spans); n != 0 {
			outer := d.spans[n-1]
			if !timed {
				ctx.timed, ctx.begin, ctx.end = outer.timed, outer.begin, outer.end
			}
			if ctx.role == "" {
				ctx.role = outer.role
			}
		}
		d.spans = append(d.spans, ctx)
	case "br":
		if d.para != nil {
			d.para.Spans = append(d.para.Spans, Span{Text: "\n"})
		}
	}
	return nil
}

func (d *decoder) end() {
	if len(d.stack) == 0 {
		return
	}
	switch d.parent() {
	case "p":
		d.para = nil
	case "span":
		if len(d.spans) != 0 {
			d.spans = d.spans[:len(d.spans)-1]
		}
	}
	d.stack = d.stack[:len(d.stack)-1]
	d.textSpan = -1
}

func (d *decoder) charData(s string) {
	switch d.parent() {
	case "title":
		if len(d.stack) >= 2 && d.stack[len(d.stack)-2] == "metadata" {
			d.doc.Title += strings.TrimSpace(s)
		}
		return
	case "name":
		if n := len(d.doc.Agents); n != 0 && len(d.stack) >= 2 && d.stack[len(d.stack)-2] == "agent" {
			d.doc.Agents[n-1].Name += strings.TrimSpace(s)
		}
		return
	case "songwriter":
		if n := len(d.doc.Songwriters); n != 0 && len(d.stack) >= 2 && d.stack[len(d.stack)-2] == "songwriters" {
			d.doc.Songwriters[n-1] += strings.TrimSpace(s)
		}
		return
	}
	if d.para == nil {
		return
	}
	s = collapseSpace(s)
	// Whitespace collapses across element boundaries, and is dropped at the start of a line.
	if n := len(d.para.Spans); strings.HasPrefix(s, " ") && (n == 0 || strings.HasSuffix(d.para.Spans[n-1].Text, " ") || d.para.Spans[n-1].Text == "\n") {
		s = s[1:]
	}
	if s == "" {
		return
	}
	if d.textSpan >= 0 {
		d.para.Spans[d.textSpan].Text += s
		return
	}
	var sp Span
	if n := len(d.spans); n != 0 {
		ctx := d.spans[n-1]
		sp = Span{Timed: ctx.timed, Begin: ctx.begin, End: ctx.end, Role: ctx.role}
	}
	sp.Text = s
	d.para.Spans = append(d.para.Spans, sp)
	d.textSpan = len(d.para.Spans) - 1
}

// collapseSpace replaces each run of whitespace with a single space.
func collapseSpace(s string) string {
	var b strings.Builder
	var space bool
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			space = true
			continue
		}
		if space {
			b.WriteByte(' ')
			space = false
		}
		b.WriteRune(r)
	}
	if space {
		b.WriteByte(' ')
	}
	return b.String()
}

// trimSpans removes whitespace at the start and end of the paragraph and around line breaks, and drops
// spans that become empty.
func trimSpans(p *Paragraph) {
	for i := range p.Spans {
		s := &p.Spans[i]
		if s.Text == "\n" {
			continue
		}
		if i == 0 || p.Spans[i-1].Text == "\n" {
			s.Text = strings.TrimLeft(s.Text, " ")
		}
		if i == len(p.Spans)-1 || p.Spans[i+1].Text == "\n" {
			s.Text = strings.TrimRight(s.Text, " ")
		}
	}
	var spans []Span
	for _, s := range p.Spans {
		if s.Text != "" {
			spans = append(spans, s)
		}
	}
	p.Spans = spans
}

func attr(t xml.StartElement, local string) string {
	for _, a := range t.Attr {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

// timing returns the begin and end attributes. A dur attribute sets the end when there is none.
func (d *decoder) timing(t xml.StartElement) (begin, end time.Duration, timed bool, err error) {
	timed = attr(t, "begin") != ""
	if begin, err = d.parseTime(t, "begin"); err != nil {
		return
	}
	if end, err = d.parseTime(t, "end"); err != nil {
		return
	}
	if attr(t, "end") == "" {
		var dur time.Duration
		if dur, err = d.parseTime(t, "dur"); err != nil {
			return
		}
		if dur != 0 {
			end = begin + dur
		}
	}
	return
}

func (d *decoder) parseTime(t xml.StartElement, name string) (time.Duration, error) {
	v := attr(t, name)
	if v == "" {
		return 0, nil
	}
	tc, err := ParseTime(v)
	if err != nil {
		return 0, d.errorf("%s: %w", name, err)
	}
	return tc, nil
}

// ParseTime parses a TTML time expression: a clock time such as "00:01:02.5" or "1:02.5", or an offset time
// such as "62.5", "62.5s", "1500ms", "2m", or "1h". Frame and tick times are not supported.
func ParseTime(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if strings.Contains(s, ":") {
		return clock.Parse(s)
	}
	num, unit := s, time.Second
	for _, u := range []struct {
		suffix string
		unit   time.Duration
	}{{"ms", time.Millisecond}, {"h", time.Hour}, {"m", time.Minute}, {"s", time.Second}} {
		if n, ok := strings.CutSuffix(s, u.suffix); ok {
			num, unit = n, u.unit
			break
		}
	}
	if num == "" || strings.TrimLeft(num, "0123456789.") != "" {
		return 0, fmt.Errorf("invalid time %q", s)
	}
	f, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid time %q: %w", s, err)
	}
	if f*float64(unit) >= float64(1<<63-1) {
		return 0, errors.New("time too large")
	}
	return time.Duration(f * float64(unit)).Round(time.Microsecond), nil
}

// Encode writes a TTML document. Times are written as hh:mm:ss.fff and negative times are an error.
func Encode(w io.Writer, doc *Document) error {
	if err := checkTimes(doc); err != nil {
		return err
	}
	bw := bufio.NewWriter(w)
	p := func(format string, args ...any) {
		fmt.Fprintf(bw, format, args...) //nolint:errcheck // bufio.Writer keeps the first error for Flush
	}
	p(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	p(`<tt xmlns="%s" xmlns:ttm="%s"`, nsTT, nsMetadata)
	if doc.Lang != "" {
		p(` xml:lang="%s"`, escape(doc.Lang))
	}
	p(">\n")
	if doc.Title != "" || len(doc.Agents) != 0 || len(doc.Songwriters) != 0 {
		p("  <head>\n    <metadata>\n")
		if doc.Title != "" {
			p("      <ttm:title>%s</ttm:title>\n", escape(doc.Title))
		}
		for _, a := range doc.Agents {
			p(`      <ttm:agent xml:id="%s"`, escape(a.ID))
			if a.Type != "" {
				p(` type="%s"`, escape(a.Type))
			}
			if a.Name == "" {
				p("/>\n")
				continue
			}
			p(`><ttm:name type="full">%s</ttm:name></ttm:agent>`+"\n", escape(a.Name))
		}
		if len(doc.Songwriters) != 0 {
			p(`      <iTunesMetadata xmlns="%s"><songwriters>`, nsITunes)
			for _, s := range doc.Songwriters {
				p("<songwriter>%s</songwriter>", escape(s))
			}
			p("</songwriters></iTunesMetadata>\n")
		}
		p("    </metadata>\n  </head>\n")
	}
	p("  <body")
	if doc.Duration != 0 {
		p(` dur="%s"`, formatTime(doc.Duration))
	}
	p(">\n")
	for _, div := range doc.Divs {
		p("    <div%s>\n", timingAttrs(div.Begin != 0 || div.End != 0, div.Begin, div.End))
		for _, para := range div.Paragraphs {
			p("      <p")
			if para.ID != "" {
				p(` xml:id="%s"`, escape(para.ID))
			}
			p("%s", timingAttrs(para.Begin != 0 || para.End != 0, para.Begin, para.End))
			if para.Agent != "" {
				p(` ttm:agent="%s"`, escape(para.Agent))
			}
			p(">")
			for _, s := range para.Spans {
				if !s.Timed && s.Role == "" {
					p("%s", textWithBreaks(s.Text))
					continue
				}
				p("<span%s", timingAttrs(s.Timed, s.Begin, s.End))
				if s.Role != "" {
					p(` ttm:role="%s"`, escape(s.Role))
				}
				p(">%s</span>", textWithBreaks(s.Text))
			}
			p("</p>\n")
		}
		p("    </div>\n")
	}
	p("  </body>\n</tt>\n")
	return bw.Flush()
}

func checkTimes(doc *Document) error {
	if doc.Duration < 0 {
		return errors.New("negative duration")
	}
	for _, div := range doc.Divs {
		if div.Begin < 0 || div.End < 0 {
			return errors.New("negative div time")
		}
		for _, para := range div.Paragraphs {
			if para.Begin < 0 || para.End < 0 {
				return errors.New("negative paragraph time")
			}
			for _, s := range para.Spans {
				if s.Begin < 0 || s.End < 0 {
					return errors.New("negative span time")
				}
			}
		}
	}
	return nil
}

// timingAttrs returns the begin and end attributes for a timed element. The end is left out when it's
// unknown.
func timingAttrs(timed bool, begin, end time.Duration) string {
	if !timed {
		return ""
	}
	s := fmt.Sprintf(` begin="%s"`, formatTime(begin))
	if end > begin {
		s += fmt.Sprintf(` end="%s"`, formatTime(end))
	}
	return s
}

func formatTime(d time.Duration) string {
	return clock.Format(d, '.', false)
}

func textWithBreaks(s string) string {
	parts := strings.Split(s, "\n")
	for i, part := range parts {
		parts[i] = escape(part)
	}
	return strings.Join(parts, "<br/>")
}

func escape(s string) string {
	var b strings.Builder
	xml.EscapeText(&b, []byte(s)) //nolint:errcheck,gosec // strings.Builder never fails
	return b.String()
}
