// Package srt reads and writes SubRip (.srt) subtitles.
package srt

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/encoding"
	"samuelks.com/timedtext"
	"samuelks.com/timedtext/charset"
	"samuelks.com/timedtext/internal/clock"
	"samuelks.com/timedtext/internal/lines"
)

var (
	reTag = regexp.MustCompile(`</?[A-Za-z][^>]*>|\{\\[^}]*\}`)
	// reCommaTiming matches a timing line that separates the times with a comma instead of an arrow.
	reCommaTiming = regexp.MustCompile(`^(\d+:\d{2}:\d{2}[.,]\d{1,9}),(\d+:\d{2}:\d{2}[.,]\d{1,9})$`)
)

// Subtitles is the contents of an SRT file.
type Subtitles struct {
	Cues []Cue
}

// Cue is one numbered subtitle.
type Cue struct {
	Index int // Zero when missing. The encoder numbers such cues by position.
	Start time.Duration
	End   time.Duration
	// Settings is any text after the end time, such as position coordinates.
	Settings string
	// Lines may contain formatting tags such as <i>.
	Lines []string
}

// SyntaxError reports a malformed line in the input.
type SyntaxError struct {
	Line int // 1-based line number
	Err  error
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("srt: line %d: %s", e.Line, e.Err)
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

// Decode reads SRT subtitles and converts them to UTF-8. A byte order mark or UTF-16 is detected first,
// then valid UTF-8, then opt.Charset. Cues are separated by blank lines, and the index line is optional.
// A timing line may also be "hh:mm:ss.mmm,hh:mm:ss.mmm", without an arrow or settings.
// Lines may end in "\n", "\r\n", or "\r". Text after a blank line that isn't followed by a timing line is
// added to the cue before it, since some files have blank lines inside cues.
func Decode(r io.Reader, opt DecodeOptions) (*Subtitles, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	data, err := charset.Decode(raw, "", opt.Charset)
	if err != nil {
		return nil, err
	}
	subs := &Subtitles{}
	// cue is the cue that text lines are added to, or nil after a blank line.
	var cue *Cue
	// pending holds the line after a blank line until the next line shows whether it's an index.
	var pending string
	// stray adds lines to the last cue, or fails if there is none.
	stray := func(lineNum int, lines ...string) error {
		if len(subs.Cues) == 0 {
			return &SyntaxError{Line: lineNum, Err: errors.New("missing timing line")}
		}
		cue = &subs.Cues[len(subs.Cues)-1]
		cue.Lines = append(cue.Lines, lines...)
		return nil
	}
	var lineNum int
	for n, text := range lines.All(string(data)) {
		lineNum = n
		text = strings.TrimRight(text, " \t")
		switch {
		case strings.TrimSpace(text) == "":
			if pending != "" {
				if err := stray(n, pending); err != nil {
					return nil, err
				}
				pending = ""
			}
			cue = nil
		case cue != nil:
			cue.Lines = append(cue.Lines, text)
		case strings.Contains(text, "-->") || reCommaTiming.MatchString(text):
			var c Cue
			if c.Start, c.End, c.Settings, err = parseTiming(text); err != nil {
				return nil, &SyntaxError{Line: n, Err: err}
			}
			if pending != "" {
				if c.Index, err = strconv.Atoi(strings.TrimSpace(pending)); err != nil {
					if stray(n-1, pending) != nil {
						return nil, &SyntaxError{Line: n - 1, Err: fmt.Errorf("invalid index %q", pending)}
					}
				}
				pending = ""
			}
			subs.Cues = append(subs.Cues, c)
			cue = &subs.Cues[len(subs.Cues)-1]
		case pending == "":
			pending = text
		default:
			if err := stray(n, pending, text); err != nil {
				return nil, err
			}
			pending = ""
		}
	}
	if pending != "" {
		if err := stray(lineNum, pending); err != nil {
			return nil, err
		}
	}
	return subs, nil
}

func parseTiming(line string) (start, end time.Duration, settings string, err error) {
	m := reCommaTiming.FindStringSubmatch(line)
	if m == nil {
		return clock.ParseTiming(line)
	}
	if start, err = clock.Parse(m[1]); err != nil {
		return 0, 0, "", err
	}
	if end, err = clock.Parse(m[2]); err != nil {
		return 0, 0, "", err
	}
	return start, end, "", nil
}

// Encode writes SRT subtitles. Times are rounded to the nearest millisecond, and negative times are an
// error. Embedded line breaks in a line are split into separate lines and empty lines are dropped, since
// a blank line ends a cue.
func Encode(w io.Writer, subs *Subtitles) error {
	bw := bufio.NewWriter(w)
	for i, c := range subs.Cues {
		if c.Start < 0 || c.End < 0 {
			return fmt.Errorf("cue %d: negative time", i+1)
		}
		index := c.Index
		if index == 0 {
			index = i + 1
		}
		timing := clock.Format(c.Start, ',', false) + " --> " + clock.Format(c.End, ',', false)
		if c.Settings != "" {
			timing += " " + c.Settings
		}
		if _, err := fmt.Fprintf(bw, "%d\n%s\n", index, timing); err != nil {
			return err
		}
		for _, l := range c.Lines {
			for _, s := range lines.All(l) {
				if s = strings.TrimRight(s, " \t"); strings.TrimSpace(s) == "" {
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
	}
	return bw.Flush()
}

// ToTrack converts the subtitles to the format-neutral model. Formatting tags are removed. An index
// becomes the cue's ID when it isn't the cue's position.
func (s *Subtitles) ToTrack() *timedtext.Track {
	t := &timedtext.Track{}
	for i, c := range s.Cues {
		text := make([]string, len(c.Lines))
		for j, l := range c.Lines {
			text[j] = strings.TrimSpace(reTag.ReplaceAllString(l, ""))
		}
		cue := timedtext.Cue{
			Start: c.Start,
			End:   c.End,
			Text:  strings.Join(text, "\n"),
		}
		if c.Index != 0 && c.Index != i+1 {
			cue.ID = strconv.Itoa(c.Index)
		}
		t.Cues = append(t.Cues, cue)
	}
	return t
}

// FromTrack converts a track from the format-neutral model. Cues without text are skipped, and metadata,
// IDs, voices, and spans are dropped.
func FromTrack(t *timedtext.Track) *Subtitles {
	s := &Subtitles{}
	for i, c := range t.Cues {
		if strings.TrimSpace(c.Text) == "" {
			continue
		}
		s.Cues = append(s.Cues, Cue{
			Index: len(s.Cues) + 1,
			Start: c.Start,
			End:   t.CueEnd(i),
			Lines: strings.Split(c.Text, "\n"),
		})
	}
	return s
}
