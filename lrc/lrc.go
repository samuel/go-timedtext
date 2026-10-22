// Package lrc reads and writes LRC synchronized lyrics, including the A2 (enhanced) word timing and
// Walaoke voice extensions.
//
// See https://en.wikipedia.org/wiki/LRC_(file_format), https://docs.fileformat.com/misc/lrc/, and
// http://p.cweiske.de/458.
package lrc

import (
	"bytes"
	"cmp"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"samuelks.com/timedtext/internal/clock"
)

// TagKey is the lowercase key of an ID tag.
type TagKey string

// ID tag keys.
const (
	TagAlbum    TagKey = "al"
	TagArtist   TagKey = "ar"
	TagAuthor   TagKey = "au"
	TagBy       TagKey = "by"       // Creator of the LRC file
	TagEncoding TagKey = "encoding" // Non-standard, e.g. "utf-8"
	TagID       TagKey = "id"
	TagLanguage TagKey = "la"     // e.g. "eng"
	TagLength   TagKey = "length" // e.g. "12:27", "1:15"
	TagOffset   TagKey = "offset" // +/- Overall timestamp adjustment in milliseconds. Positive values make lyrics appear sooner.
	TagSoftware TagKey = "re"
	TagTitle    TagKey = "ti"
	TagVersion  TagKey = "ve"
)

func (tk TagKey) String() string {
	return string(tk)
}

// Name returns a human-readable name for the key.
func (tk TagKey) Name() string {
	switch tk {
	case TagAlbum:
		return "Album"
	case TagArtist:
		return "Artist"
	case TagAuthor:
		return "Author"
	case TagBy:
		return "By"
	case TagEncoding:
		return "Encoding"
	case TagID:
		return "ID"
	case TagLanguage:
		return "Language"
	case TagLength:
		return "Length"
	case TagOffset:
		return "Offset"
	case TagSoftware:
		return "Software"
	case TagTitle:
		return "Title"
	case TagVersion:
		return "Version"
	}
	r, size := utf8.DecodeRuneInString(string(tk))
	if size == 0 {
		return ""
	}
	return string(unicode.ToUpper(r)) + string(tk)[size:]
}

// Tag is an ID tag such as [ar:Artist].
type Tag struct {
	Key   TagKey
	Value string
}

// Voice is a Walaoke singer marker.
type Voice string

// Walaoke voice markers.
const (
	VoiceMale   Voice = "M"
	VoiceFemale Voice = "F"
	VoiceDuet   Voice = "D"
)

// Lyrics is the contents of an LRC file.
type Lyrics struct {
	Tags    []Tag    // In file order. Keys may repeat.
	Phrases []Phrase // In file order, which need not be sorted by time.
}

// Phrase is a timecode plus zero or more lines of text. A phrase with no text usually clears the display.
type Phrase struct {
	Timecode time.Duration
	// Voice is the singer. Phrases without a marker inherit the previous phrase's voice.
	Voice Voice
	Lines []string
	// Words holds A2 word timing for all of the phrase's text, in which case Lines is the plain text. Word
	// text may contain newlines where lines break. The encoder writes Words in place of Lines.
	Words []Word
}

// Word is an A2 word time tag and the text that follows it. A word with empty text marks the end of the
// previous word.
type Word struct {
	Timecode time.Duration
	Text     string
}

// Tag returns the value of the last tag with key, or "" if there is none.
func (l *Lyrics) Tag(key TagKey) string {
	for _, t := range slices.Backward(l.Tags) {
		if t.Key == key {
			return t.Value
		}
	}
	return ""
}

// SetTag sets the value of the first tag with key and removes any others, or appends a tag if there is none.
func (l *Lyrics) SetTag(key TagKey, value string) {
	i := slices.IndexFunc(l.Tags, func(t Tag) bool { return t.Key == key })
	if i < 0 {
		l.Tags = append(l.Tags, Tag{Key: key, Value: value})
		return
	}
	l.Tags[i].Value = value
	l.Tags = append(l.Tags[:i+1], deleteKey(l.Tags[i+1:], key)...)
}

// DeleteTag removes all tags with key.
func (l *Lyrics) DeleteTag(key TagKey) {
	l.Tags = deleteKey(l.Tags, key)
}

func deleteKey(tags []Tag, key TagKey) []Tag {
	return slices.DeleteFunc(tags, func(t Tag) bool { return t.Key == key })
}

// Title returns the value of the title tag.
func (l *Lyrics) Title() string {
	return l.Tag(TagTitle)
}

// Artist returns the value of the artist tag.
func (l *Lyrics) Artist() string {
	return l.Tag(TagArtist)
}

// Length parses the length tag, such as "3:25", or returns zero if it isn't set.
func (l *Lyrics) Length() (time.Duration, error) {
	v := l.Tag(TagLength)
	if v == "" {
		return 0, nil
	}
	fields, frac, _ := strings.Cut(v, ".")
	d, err := clock.Duration(strings.Split(fields, ":"), frac)
	if err != nil {
		return 0, fmt.Errorf("invalid length %q: %w", v, err)
	}
	return d, nil
}

// Offset returns the value of the offset tag, or zero if it isn't set.
// A positive offset makes lyrics appear sooner.
func (l *Lyrics) Offset() (time.Duration, error) {
	v := l.Tag(TagOffset)
	if v == "" {
		return 0, nil
	}
	ms, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid offset %q: %w", v, err)
	}
	if limit := int64(math.MaxInt64 / time.Millisecond); ms > limit || ms < -limit {
		return 0, fmt.Errorf("invalid offset %q: out of range", v)
	}
	return time.Duration(ms) * time.Millisecond, nil
}

// ApplyOffset shifts all phrases and words by the offset tag and removes the tag.
// Timecodes that would become negative are set to zero.
func (l *Lyrics) ApplyOffset() error {
	off, err := l.Offset()
	if err != nil {
		return err
	}
	for i := range l.Phrases {
		p := &l.Phrases[i]
		p.Timecode = max(p.Timecode-off, 0)
		for j := range p.Words {
			p.Words[j].Timecode = max(p.Words[j].Timecode-off, 0)
		}
	}
	l.DeleteTag(TagOffset)
	return nil
}

// Sort sorts phrases by timecode, keeping the order of phrases with equal timecodes.
func (l *Lyrics) Sort() {
	sortPhrases(l.Phrases)
}

func sortPhrases(phrases []Phrase) {
	slices.SortStableFunc(phrases, func(a, b Phrase) int { return cmp.Compare(a.Timecode, b.Timecode) })
}

// MarshalText encodes the lyrics with the default options.
func (l *Lyrics) MarshalText() ([]byte, error) {
	var b bytes.Buffer
	if err := Encode(&b, l, EncodeOptions{}); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// UnmarshalText decodes lyrics with the default options.
func (l *Lyrics) UnmarshalText(text []byte) error {
	lyr, err := Decode(bytes.NewReader(text), DecodeOptions{})
	if err != nil {
		return err
	}
	*l = *lyr
	return nil
}
