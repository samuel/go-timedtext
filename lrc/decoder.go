package lrc

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"golang.org/x/text/encoding"
	"samuelks.com/timedtext/charset"
	"samuelks.com/timedtext/internal/clock"
	"samuelks.com/timedtext/internal/lines"
)

var (
	reIDTag = regexp.MustCompile(`^\s*\[([A-Za-z]+):(.*)\]\s*$`)
	// reTimecode matches mm:ss, mm:ss.f, hh:mm:ss.f, mm:ss:cc, and hh:mm:ss:cc.
	reTimecode = regexp.MustCompile(`\[(\d+(?::\d+){1,2}(?:\.\d+)?|\d+(?::\d+){3})\]`)
	// reEncodingTag matches an encoding tag in the raw input.
	reEncodingTag = regexp.MustCompile(`(?im)^[ \t]*\[encoding:[ \t]*([^\]\r\n]*?)[ \t]*\]`)
	// reWordTime matches an A2 word time tag such as <00:01.50>.
	reWordTime = regexp.MustCompile(`<(\d+(?::\d+){1,2}(?:\.\d+)?|\d+(?::\d+){3})>`)
)

// DecodeOptions controls how Decode interprets its input.
type DecodeOptions struct {
	// Lenient keeps invalid timecodes as text instead of returning an error.
	Lenient bool
	// Voices parses Walaoke M:, F:, and D: prefixes at the start of a phrase's text into Phrase.Voice.
	// It's off by default because lyrics can start with these.
	Voices bool
	// Charset is the encoding of input that has no byte order mark, isn't valid UTF-8, and has no known
	// encoding tag. Defaults to Windows-1252.
	Charset encoding.Encoding
}

// SyntaxError reports a malformed line in the input.
type SyntaxError struct {
	Line int // 1-based line number
	Err  error
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("lrc: line %d: %s", e.Line, e.Err)
}

func (e *SyntaxError) Unwrap() error {
	return e.Err
}

type decoder struct {
	opt DecodeOptions
	lyr *Lyrics
	// group holds the indexes of the phrases that text is appended to.
	group []int
	voice Voice
}

// Decode reads LRC lyrics and converts them to UTF-8. A byte order mark or UTF-16 is detected first,
// then valid UTF-8, then the encoding tag, then opt.Charset. Lines may end in "\n", "\r\n", or "\r". ID
// tags are only recognized before the first non-tag line. Adjacent timecodes on one line (e.g.
// [00:21.10][00:45.10]chorus) each get a copy of the text. A2 word time tags are parsed into
// Phrase.Words, and each copy's word times are shifted by its distance from the first timecode. Invalid
// word time tags are kept as text.
func Decode(r io.Reader, opt DecodeOptions) (*Lyrics, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	data, err := charset.Decode(raw, declaredCharset(raw), opt.Charset)
	if err != nil {
		return nil, err
	}
	header := true
	d := &decoder{
		opt: opt,
		// Placeholder phrase for any text before the first timecode.
		lyr:   &Lyrics{Phrases: []Phrase{{}}},
		group: []int{0},
	}
	for lineNum, text := range lines.All(string(data)) {
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		if header {
			if parts := reIDTag.FindStringSubmatch(text); parts != nil {
				d.lyr.Tags = append(d.lyr.Tags, Tag{
					Key:   TagKey(strings.ToLower(parts[1])),
					Value: strings.TrimSpace(parts[2]),
				})
				continue
			}
			header = false
		}
		if err := d.decodeLine(text); err != nil {
			return nil, &SyntaxError{Line: lineNum, Err: err}
		}
	}
	lyr := d.lyr
	if len(lyr.Phrases[0].Lines) == 0 {
		if lyr.Phrases = lyr.Phrases[1:]; len(lyr.Phrases) == 0 {
			lyr.Phrases = nil
		}
	}
	return lyr, nil
}

// DetectCharset returns the encoding Decode would use for data. Callers that write a file back can pass it
// to EncodeOptions to keep the file's encoding.
func DetectCharset(data []byte, opt DecodeOptions) charset.Detected {
	return charset.Detect(data, declaredCharset(data), opt.Charset)
}

// declaredCharset returns the value of the encoding tag in raw input, or "".
func declaredCharset(raw []byte) string {
	if m := reEncodingTag.FindSubmatch(raw); m != nil {
		return string(m[1])
	}
	return ""
}

func (d *decoder) decodeLine(text string) error {
	var lastIx int
	// adjacent is true when the previous match on this line was a timecode with no text after it.
	var adjacent bool
	for _, ix := range reTimecode.FindAllStringSubmatchIndex(text, -1) {
		tc, err := ParseTimecode(text[ix[2]:ix[3]])
		if err != nil {
			if d.opt.Lenient {
				continue
			}
			return err
		}
		if t := strings.TrimSpace(text[lastIx:ix[0]]); t != "" {
			d.appendText(t)
			adjacent = false
		}
		d.lyr.Phrases = append(d.lyr.Phrases, Phrase{Timecode: tc, Voice: d.voice})
		i := len(d.lyr.Phrases) - 1
		if adjacent {
			d.group = append(d.group, i)
		} else {
			d.group = []int{i}
		}
		adjacent = true
		lastIx = ix[1]
	}
	if t := strings.TrimSpace(text[lastIx:]); t != "" {
		d.appendText(t)
	}
	return nil
}

// appendText adds a line of text to the current group. The first line of a phrase may start with a voice
// marker.
func (d *decoder) appendText(text string) {
	// Phrases in a group always have the same lines, so checking one is enough.
	first := len(d.lyr.Phrases[d.group[0]].Lines) == 0
	if first && d.opt.Voices {
		if v, rest, ok := cutVoice(text); ok {
			d.voice = v
			for _, i := range d.group {
				d.lyr.Phrases[i].Voice = v
			}
			if text = rest; text == "" {
				return
			}
		}
	}
	lead, words := parseWords(text)
	plain := text
	if words != nil {
		var b strings.Builder
		b.WriteString(lead)
		for _, w := range words {
			b.WriteString(w.Text)
		}
		plain = strings.TrimSpace(b.String())
	}
	base := d.lyr.Phrases[d.group[0]].Timecode
	for _, i := range d.group {
		p := &d.lyr.Phrases[i]
		switch {
		case first && words != nil:
			if strings.TrimSpace(lead) != "" {
				p.Words = append(p.Words, Word{Timecode: p.Timecode, Text: lead})
			}
			p.Words = appendShifted(p.Words, words, p.Timecode-base)
		case !first && (words != nil || p.Words != nil):
			if p.Words == nil {
				p.Words = []Word{{Timecode: p.Timecode, Text: strings.Join(p.Lines, "\n")}}
			}
			p.Words[len(p.Words)-1].Text += "\n" + lead
			p.Words = appendShifted(p.Words, words, p.Timecode-base)
		}
		p.Lines = append(p.Lines, plain)
	}
}

// appendShifted appends words with their timecodes shifted by d, and clamped at zero.
func appendShifted(dst, words []Word, d time.Duration) []Word {
	for _, w := range words {
		w.Timecode = max(w.Timecode+d, 0)
		dst = append(dst, w)
	}
	return dst
}

func cutVoice(text string) (Voice, string, bool) {
	for _, v := range []Voice{VoiceMale, VoiceFemale, VoiceDuet} {
		if rest, ok := strings.CutPrefix(text, string(v)+":"); ok {
			return v, strings.TrimSpace(rest), true
		}
	}
	return "", text, false
}

// parseWords splits text at A2 word time tags. It returns the text before the first tag and a word for each
// tag, or nil words when there are no valid tags.
func parseWords(text string) (string, []Word) {
	var lead string
	var words []Word
	var lastIx int
	for _, ix := range reWordTime.FindAllStringSubmatchIndex(text, -1) {
		tc, err := ParseTimecode(text[ix[2]:ix[3]])
		if err != nil {
			continue
		}
		if words == nil {
			lead = text[:ix[0]]
		} else {
			words[len(words)-1].Text = text[lastIx:ix[0]]
		}
		words = append(words, Word{Timecode: tc})
		lastIx = ix[1]
	}
	if words == nil {
		return text, nil
	}
	words[len(words)-1].Text = text[lastIx:]
	return lead, words
}

// ParseTimecode parses the contents of a timecode without brackets. The fraction after a '.' may have any
// number of digits. Without a '.', three fields are read as mm:ss:cc when the last field has two digits,
// otherwise as hh:mm:ss. Four fields are read as hh:mm:ss:cc.
func ParseTimecode(s string) (time.Duration, error) {
	hms, frac, hasFrac := strings.Cut(s, ".")
	fields := strings.Split(hms, ":")
	if !hasFrac && (len(fields) == 4 || (len(fields) == 3 && len(fields[2]) == 2)) {
		frac = fields[len(fields)-1]
		fields = fields[:len(fields)-1]
	}
	if len(fields) < 2 || len(fields) > 3 {
		return 0, fmt.Errorf("invalid timecode %q", s)
	}
	d, err := clock.Duration(fields, frac)
	if err != nil {
		return 0, fmt.Errorf("invalid timecode %q: %w", s, err)
	}
	return d, nil
}
