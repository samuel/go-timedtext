// Package vtt reads and writes WebVTT (.vtt) subtitles.
//
// See https://www.w3.org/TR/webvtt1/.
package vtt

import (
	"bufio"
	"errors"
	"fmt"
	"html"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/encoding"
	"samuelks.com/timedtext"
	"samuelks.com/timedtext/charset"
	"samuelks.com/timedtext/internal/clock"
	"samuelks.com/timedtext/internal/lines"
)

var reTag = regexp.MustCompile(`<[^>]*>`)

// File is the contents of a WebVTT file.
type File struct {
	// Header is the text after "WEBVTT" in the header block, including any leading space and the header
	// lines that follow.
	Header string
	// Blocks holds the raw NOTE, STYLE, and REGION blocks before the first cue.
	Blocks []string
	Cues   []Cue
}

// Cue is one timed subtitle.
type Cue struct {
	ID       string
	Start    time.Duration
	End      time.Duration
	Settings string   // Cue settings after the end time, such as "align:start"
	Lines    []string // Payload, which may contain markup
	// After holds the raw blocks, usually NOTEs, between this cue and the next.
	After []string
}

// SyntaxError reports a malformed line in the input.
type SyntaxError struct {
	Line int // 1-based line number
	Err  error
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("vtt: line %d: %s", e.Line, e.Err)
}

func (e *SyntaxError) Unwrap() error {
	return e.Err
}

// DecodeOptions controls how Decode interprets its input.
type DecodeOptions struct {
	// Charset is the encoding of input that has no byte order mark and isn't valid UTF-8.
	// Defaults to Windows-1252.
	Charset encoding.Encoding
}

// Decode reads WebVTT subtitles and converts them to UTF-8. WebVTT requires UTF-8, but older files may
// not follow that, so a byte order mark or UTF-16 is detected first, then valid UTF-8, then opt.Charset.
// Lines may end in "\n", "\r\n", or "\r".
func Decode(r io.Reader, opt DecodeOptions) (*File, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	data, err := charset.Decode(raw, "", opt.Charset)
	if err != nil {
		return nil, err
	}
	f := &File{}
	var block []string
	var blockLine, lineNum int
	var sawHeader bool
	flush := func() error {
		if len(block) == 0 {
			return nil
		}
		defer func() { block = nil }()
		if !sawHeader {
			sawHeader = true
			rest, ok := strings.CutPrefix(block[0], "WEBVTT")
			if !ok || (rest != "" && rest[0] != ' ' && rest[0] != '\t') {
				return &SyntaxError{Line: blockLine, Err: errors.New("missing WEBVTT header")}
			}
			f.Header = strings.Join(append([]string{rest}, block[1:]...), "\n")
			return nil
		}
		if isRawBlock(block[0]) {
			raw := strings.Join(block, "\n")
			if n := len(f.Cues); n != 0 {
				f.Cues[n-1].After = append(f.Cues[n-1].After, raw)
			} else {
				f.Blocks = append(f.Blocks, raw)
			}
			return nil
		}
		var c Cue
		timing := 0
		if !strings.Contains(block[0], "-->") {
			c.ID = block[0]
			timing = 1
		}
		if timing >= len(block) || !strings.Contains(block[timing], "-->") {
			return &SyntaxError{Line: blockLine + timing, Err: errors.New("missing timing line")}
		}
		var err error
		if c.Start, c.End, c.Settings, err = clock.ParseTiming(block[timing]); err != nil {
			return &SyntaxError{Line: blockLine + timing, Err: err}
		}
		c.Lines = block[timing+1:]
		f.Cues = append(f.Cues, c)
		return nil
	}
	for n, text := range lines.All(string(data)) {
		lineNum = n
		if strings.TrimSpace(text) == "" {
			if err := flush(); err != nil {
				return nil, err
			}
			continue
		}
		if len(block) == 0 {
			blockLine = lineNum
		}
		block = append(block, text)
	}
	if err := flush(); err != nil {
		return nil, err
	}
	if !sawHeader {
		return nil, &SyntaxError{Line: 1, Err: errors.New("missing WEBVTT header")}
	}
	return f, nil
}

func isRawBlock(first string) bool {
	for _, kw := range []string{"NOTE", "STYLE", "REGION"} {
		if rest, ok := strings.CutPrefix(first, kw); ok && (rest == "" || rest[0] == ' ' || rest[0] == '\t') {
			return true
		}
	}
	return false
}

// Encode writes WebVTT subtitles. Times are rounded to the nearest millisecond, and negative times are an
// error. Embedded line breaks in a payload line are split into separate lines and empty lines are
// dropped, since a blank line ends a cue.
func Encode(w io.Writer, f *File) error {
	bw := bufio.NewWriter(w)
	if _, err := fmt.Fprintf(bw, "WEBVTT%s\n\n", f.Header); err != nil {
		return err
	}
	if err := writeBlocks(bw, f.Blocks); err != nil {
		return err
	}
	for i, c := range f.Cues {
		if c.Start < 0 || c.End < 0 {
			return fmt.Errorf("cue %d: negative time", i+1)
		}
		if c.ID != "" {
			if _, err := fmt.Fprintln(bw, c.ID); err != nil {
				return err
			}
		}
		timing := clock.Format(c.Start, '.', false) + " --> " + clock.Format(c.End, '.', false)
		if c.Settings != "" {
			timing += " " + c.Settings
		}
		if _, err := fmt.Fprintln(bw, timing); err != nil {
			return err
		}
		for _, l := range c.Lines {
			for _, s := range lines.All(l) {
				if strings.TrimSpace(s) == "" {
					continue
				}
				if _, err := fmt.Fprintln(bw, s); err != nil {
					return err
				}
			}
		}
		if err := bw.WriteByte('\n'); err != nil {
			return err
		}
		if err := writeBlocks(bw, c.After); err != nil {
			return err
		}
	}
	return bw.Flush()
}

func writeBlocks(bw *bufio.Writer, blocks []string) error {
	for _, b := range blocks {
		if _, err := fmt.Fprintf(bw, "%s\n\n", b); err != nil {
			return err
		}
	}
	return nil
}

// Language returns the value of the Language header line, or "" if there is none.
func (f *File) Language() string {
	for _, line := range strings.Split(f.Header, "\n")[1:] {
		if k, v, ok := strings.Cut(line, ":"); ok && strings.TrimSpace(k) == "Language" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// ToTrack converts the subtitles to the format-neutral model. A leading voice span (<v Name>) sets the
// cue's voice, timestamp tags become spans, other markup is removed, and the Language header becomes the
// track's language.
func (f *File) ToTrack() *timedtext.Track {
	t := &timedtext.Track{Metadata: timedtext.Metadata{Language: f.Language()}}
	for _, c := range f.Cues {
		t.Cues = append(t.Cues, toCue(c))
	}
	return t
}

func toCue(c Cue) timedtext.Cue {
	payload := strings.Join(c.Lines, "\n")
	cue := timedtext.Cue{ID: c.ID, Start: c.Start, End: c.End}
	var text strings.Builder
	var spans []timedtext.Span
	cur := timedtext.Span{Start: c.Start}
	var lastIx int
	var sawVoice bool
	for _, ix := range reTag.FindAllStringIndex(payload, -1) {
		seg := html.UnescapeString(payload[lastIx:ix[0]])
		text.WriteString(seg)
		cur.Text += seg
		lastIx = ix[1]
		tag := payload[ix[0]+1 : ix[1]-1]
		if ts, err := clock.Parse(tag); err == nil {
			if len(spans) != 0 || strings.TrimSpace(cur.Text) != "" {
				cur.End = ts
				spans = append(spans, cur)
			}
			cur = timedtext.Span{Start: ts}
			continue
		}
		if !sawVoice && strings.TrimSpace(text.String()) == "" {
			if name, ok := voiceName(tag); ok {
				cue.Voice = timedtext.Voice(name)
				sawVoice = true
			}
		}
	}
	seg := html.UnescapeString(payload[lastIx:])
	text.WriteString(seg)
	cue.Text = strings.TrimSpace(text.String())
	if len(spans) != 0 {
		cur.Text += seg
		cur.End = c.End
		spans = append(spans, cur)
		// Trim like the cue's text, so that the spans still concatenate to it.
		spans[0].Text = strings.TrimLeftFunc(spans[0].Text, unicode.IsSpace)
		last := &spans[len(spans)-1]
		if last.Text = strings.TrimRightFunc(last.Text, unicode.IsSpace); last.Text == "" {
			spans = spans[:len(spans)-1]
		}
		if len(spans) != 0 {
			cue.Spans = spans
		}
	}
	return cue
}

// voiceName returns the annotation of a voice start tag such as "v.loud Bob".
func voiceName(tag string) (string, bool) {
	if tag == "" || tag[0] != 'v' || (len(tag) > 1 && tag[1] != ' ' && tag[1] != '.' && tag[1] != '\t') {
		return "", false
	}
	i := strings.IndexAny(tag, " \t")
	if i < 0 {
		return "", false
	}
	if name := strings.TrimSpace(tag[i:]); name != "" {
		return name, true
	}
	return "", false
}

// cueID makes id safe to write as a cue identifier. One that would read back as a NOTE, STYLE, or REGION
// block is dropped.
func cueID(id string) string {
	// A line break or "-->" would break the cue.
	id = strings.ReplaceAll(oneLine(id), "-->", "->")
	if isRawBlock(id) {
		return ""
	}
	return id
}

var (
	escaper    = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	lineBreaks = strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ")
)

// oneLine replaces line breaks with spaces.
func oneLine(s string) string {
	return lineBreaks.Replace(s)
}

// FromTrack converts a track from the format-neutral model. Cues without text are skipped, voices are
// written as voice spans, spans as timestamp tags, and the language as a header line. Other metadata is
// dropped, as are cue IDs that would read back as NOTE, STYLE, or REGION blocks.
func FromTrack(t *timedtext.Track) *File {
	f := &File{}
	if lang := oneLine(t.Metadata.Language); lang != "" {
		f.Header = "\nLanguage: " + lang
	}
	for i, c := range t.Cues {
		if strings.TrimSpace(c.Text) == "" {
			continue
		}
		var b strings.Builder
		if c.Voice != "" {
			b.WriteString("<v " + strings.ReplaceAll(oneLine(string(c.Voice)), ">", "") + ">")
		}
		if len(c.Spans) == 0 {
			b.WriteString(escaper.Replace(c.Text))
		}
		for _, s := range c.Spans {
			if s.Start > c.Start {
				b.WriteString("<" + clock.Format(s.Start, '.', false) + ">")
			}
			b.WriteString(escaper.Replace(s.Text))
		}
		f.Cues = append(f.Cues, Cue{
			ID:    cueID(c.ID),
			Start: c.Start,
			End:   t.CueEnd(i),
			Lines: strings.Split(b.String(), "\n"),
		})
	}
	return f
}
