package lrc

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"

	"golang.org/x/text/encoding"
	"samuelks.com/timedtext/charset"
	"samuelks.com/timedtext/internal/lines"
)

// EncodeOptions controls the output format of Encode.
type EncodeOptions struct {
	// HoursInTimecode if true makes any timecode over an hour include a fourth value with the hours.
	// Otherwise, the minutes may be >= 60. Setting this to true is non-standard and may affect compatibility.
	HoursInTimecode bool
	// Multiline encodes any phrase with multilines as separate lines with newlines and no timecodes.
	// If false all lines for a phrase are concatenated into a single line with a timecode.
	// Setting this to true is non-standard and may affect compatibility.
	Multiline bool
	// Charset is the output encoding. Nil writes UTF-8 and sets the value of any encoding tag to utf-8.
	// Otherwise the output is converted, characters it can't represent are an error, and the encoding tag
	// is written as is.
	Charset encoding.Encoding
	// BOM starts the output with a byte order mark. Only UTF encodings can represent one.
	BOM bool
}

// Encode writes lyrics in LRC format. Tags are written in order as [key: value], with any line breaks
// replaced by spaces. Timecodes are rounded to the nearest centisecond, and negative timecodes are an
// error. Embedded line breaks in a line are split into separate lines and empty lines are dropped. A voice
// marker is written when a phrase's voice differs from the previous phrase's. Text that begins with
// something shaped like a timecode or ID tag is not escaped, so it decodes differently.
func Encode(w io.Writer, lyr *Lyrics, opt EncodeOptions) error {
	if opt.Charset == nil && !opt.BOM {
		return encodeUTF8(w, lyr, opt)
	}
	var b bytes.Buffer
	if err := encodeUTF8(&b, lyr, opt); err != nil {
		return err
	}
	out, err := charset.Encode(b.Bytes(), opt.Charset, opt.BOM)
	if err != nil {
		return err
	}
	_, err = w.Write(out)
	return err
}

func encodeUTF8(w io.Writer, lyr *Lyrics, opt EncodeOptions) error {
	for _, p := range lyr.Phrases {
		if p.Timecode < 0 {
			return fmt.Errorf("negative timecode %s", p.Timecode)
		}
		for _, w := range p.Words {
			if w.Timecode < 0 {
				return fmt.Errorf("negative word timecode %s", w.Timecode)
			}
		}
	}
	bw := bufio.NewWriter(w)
	for _, t := range lyr.Tags {
		if t.Key == TagEncoding && opt.Charset == nil {
			t.Value = "utf-8"
		}
		if _, err := fmt.Fprintf(bw, "[%s: %s]\n", oneLine(string(t.Key)), oneLine(t.Value)); err != nil {
			return err
		}
	}
	if len(lyr.Tags) != 0 {
		if err := bw.WriteByte('\n'); err != nil {
			return err
		}
	}
	// sep separates a phrase's timecode and lines from each other.
	sep := " "
	if opt.Multiline {
		sep = "\n"
	}
	var voice Voice
	for i, p := range lyr.Phrases {
		if opt.Multiline && i != 0 {
			if err := bw.WriteByte('\n'); err != nil {
				return err
			}
		}
		ls := textLines(p, opt)
		// A marker is also needed when the text itself starts with one.
		if p.Voice != "" && (p.Voice != voice || (len(ls) != 0 && startsWithVoice(ls[0]))) {
			if len(ls) == 0 {
				ls = []string{string(p.Voice) + ":"}
			} else {
				ls[0] = string(p.Voice) + ": " + ls[0]
			}
		}
		voice = p.Voice
		text := strings.Join(ls, sep)
		if opt.Multiline && text != "" {
			text = "\n" + text
		}
		if _, err := fmt.Fprintf(bw, "[%s]%s\n", FormatTimecode(p.Timecode, opt), text); err != nil {
			return err
		}
	}
	return bw.Flush()
}

var lineBreaks = strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ")

// oneLine replaces line breaks with spaces.
func oneLine(s string) string {
	return lineBreaks.Replace(s)
}

func startsWithVoice(s string) bool {
	_, _, ok := cutVoice(s)
	return ok
}

// textLines returns the lines of a phrase with words formatted, embedded newlines split out, and empty
// lines removed.
func textLines(p Phrase, opt EncodeOptions) []string {
	src := p.Lines
	if len(p.Words) != 0 {
		src = []string{formatWords(p.Words, opt)}
	}
	var out []string
	for _, l := range src {
		for _, s := range lines.All(l) {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

func formatWords(words []Word, opt EncodeOptions) string {
	var b strings.Builder
	for _, w := range words {
		b.WriteString("<" + FormatTimecode(w.Timecode, opt) + ">")
		b.WriteString(w.Text)
	}
	return b.String()
}

// FormatTimecode formats a non-negative timecode without brackets as mm:ss.cc, rounded to the nearest
// centisecond. Minutes may exceed 59 unless opt.HoursInTimecode is set, in which case timecodes of an hour
// or more are formatted as hh:mm:ss.cc.
func FormatTimecode(tc time.Duration, opt EncodeOptions) string {
	tc = tc.Round(time.Millisecond * 10)
	hours := tc / time.Hour
	tc -= hours * time.Hour
	minutes := tc / time.Minute
	tc -= minutes * time.Minute
	seconds := tc / time.Second
	tc -= seconds * time.Second
	centiseconds := tc / (time.Millisecond * 10)
	if opt.HoursInTimecode && hours != 0 {
		return fmt.Sprintf("%02d:%02d:%02d.%02d", hours, minutes, seconds, centiseconds)
	}
	return fmt.Sprintf("%02d:%02d.%02d", hours*60+minutes, seconds, centiseconds)
}
