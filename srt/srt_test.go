package srt

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
	"samuelks.com/timedtext"
)

const sample = "1\n00:00:01,000 --> 00:00:02,500\n<i>Hello</i>\nworld\n\n" +
	"2\n00:00:03,000 --> 00:00:04,000 X1:10 X2:20\nagain\n\n"

func TestDecode(t *testing.T) {
	in := "\xEF\xBB\xBF1\r\n00:00:01,000 --> 00:00:02,500\r\n<i>Hello</i>\r\nworld\r\n\r\n\r\n" +
		"00:00:03.000 --> 00:00:04,000 X1:10 X2:20\nagain"
	subs, err := Decode(strings.NewReader(in), DecodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	exp := &Subtitles{Cues: []Cue{
		{Index: 1, Start: time.Second, End: 2500 * time.Millisecond, Lines: []string{"<i>Hello</i>", "world"}},
		{Start: 3 * time.Second, End: 4 * time.Second, Settings: "X1:10 X2:20", Lines: []string{"again"}},
	}}
	if !reflect.DeepEqual(subs, exp) {
		t.Errorf("Decode() = %+v, expected %+v", subs, exp)
	}
}

func TestDecodeErrors(t *testing.T) {
	cases := map[string]int{
		"1\n2\n":                             2,
		"1\n\n":                              2,
		"x\n00:00:01,000 --> 00:00:02,000\n": 1,
		"1\n00:00:01,000 --> bad\n":          2,
	}
	for in, line := range cases {
		_, err := Decode(strings.NewReader(in), DecodeOptions{})
		if se, ok := errors.AsType[*SyntaxError](err); !ok {
			t.Errorf("Decode(%q) error = %v, expected *SyntaxError", in, err)
		} else if se.Line != line {
			t.Errorf("Decode(%q) error on line %d, expected %d", in, se.Line, line)
		}
	}
}

func TestRoundTrip(t *testing.T) {
	subs, err := Decode(strings.NewReader(sample), DecodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	if err := Encode(&b, subs); err != nil {
		t.Fatal(err)
	}
	if b.String() != sample {
		t.Errorf("Encode() = %q, expected %q", b.String(), sample)
	}
}

func TestTrack(t *testing.T) {
	subs, err := Decode(strings.NewReader(sample), DecodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	tr := subs.ToTrack()
	exp := []timedtext.Cue{
		{Start: time.Second, End: 2500 * time.Millisecond, Text: "Hello\nworld"},
		{Start: 3 * time.Second, End: 4 * time.Second, Text: "again"},
	}
	if !reflect.DeepEqual(tr.Cues, exp) {
		t.Errorf("ToTrack().Cues = %+v, expected %+v", tr.Cues, exp)
	}

	tr.Cues[1].End = 0
	tr.Cues = append(tr.Cues, timedtext.Cue{Start: 10 * time.Second})
	var b strings.Builder
	if err := Encode(&b, FromTrack(tr)); err != nil {
		t.Fatal(err)
	}
	expText := "1\n00:00:01,000 --> 00:00:02,500\nHello\nworld\n\n" +
		"2\n00:00:03,000 --> 00:00:10,000\nagain\n\n"
	if b.String() != expText {
		t.Errorf("FromTrack() encoded = %q, expected %q", b.String(), expText)
	}
}

func TestDecodeCharset(t *testing.T) {
	utf16, err := unicode.UTF16(unicode.BigEndian, unicode.UseBOM).NewEncoder().String("1\n00:00:01,000 --> 00:00:02,000\nnaïve\n")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		in   string
		opt  DecodeOptions
		exp  string
	}{
		{"Windows-1252", "1\n00:00:01,000 --> 00:00:02,000\ncaf\xe9\n", DecodeOptions{}, "café"},
		{"Latin-2", "1\n00:00:01,000 --> 00:00:02,000\n\xb3\xf3d\xbc\n", DecodeOptions{Charset: charmap.ISO8859_2}, "łódź"},
		{"UTF-16", utf16, DecodeOptions{}, "naïve"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			subs, err := Decode(strings.NewReader(c.in), c.opt)
			if err != nil {
				t.Fatal(err)
			}
			if len(subs.Cues) != 1 || !reflect.DeepEqual(subs.Cues[0].Lines, []string{c.exp}) {
				t.Errorf("Decode() = %+v, expected text %q", subs, c.exp)
			}
		})
	}
}

func TestDecodeLenient(t *testing.T) {
	cases := []struct {
		name string
		in   string
		exp  []Cue
	}{
		{
			name: "blank line inside a cue",
			in:   "1\n00:00:01,000 --> 00:00:02,000\na\n\nb\nc\n\n2\n00:00:03,000 --> 00:00:04,000\nd\n",
			exp: []Cue{
				{Index: 1, Start: time.Second, End: 2 * time.Second, Lines: []string{"a", "b", "c"}},
				{Index: 2, Start: 3 * time.Second, End: 4 * time.Second, Lines: []string{"d"}},
			},
		},
		{
			name: "single stray line",
			in:   "1\n00:00:01,000 --> 00:00:02,000\na\n\nb\n\n2\n00:00:03,000 --> 00:00:04,000\nd\n\ne",
			exp: []Cue{
				{Index: 1, Start: time.Second, End: 2 * time.Second, Lines: []string{"a", "b"}},
				{Index: 2, Start: 3 * time.Second, End: 4 * time.Second, Lines: []string{"d", "e"}},
			},
		},
		{
			name: "stray text before a cue without an index",
			in:   "1\n00:00:01,000 --> 00:00:02,000\na\n\nb\n00:00:03,000 --> 00:00:04,000\nd\n",
			exp: []Cue{
				{Index: 1, Start: time.Second, End: 2 * time.Second, Lines: []string{"a", "b"}},
				{Start: 3 * time.Second, End: 4 * time.Second, Lines: []string{"d"}},
			},
		},
		{
			name: "CR line endings and tab before settings",
			in:   "1\r00:00:01,000 --> 00:00:02,000\tX1:1\ra\rb\r\r2\r00:00:03,000 --> 00:00:04,000\rc\r",
			exp: []Cue{
				{Index: 1, Start: time.Second, End: 2 * time.Second, Settings: "X1:1", Lines: []string{"a", "b"}},
				{Index: 2, Start: 3 * time.Second, End: 4 * time.Second, Lines: []string{"c"}},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			subs, err := Decode(strings.NewReader(c.in), DecodeOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(subs.Cues, c.exp) {
				t.Errorf("Decode() = %+v, expected %+v", subs.Cues, c.exp)
			}
		})
	}
}

func TestTrackIDs(t *testing.T) {
	subs := &Subtitles{Cues: []Cue{{Index: 1, Lines: []string{"a"}}, {Index: 5, Lines: []string{"b"}}, {Lines: []string{"c"}}}}
	var ids []string
	for _, c := range subs.ToTrack().Cues {
		ids = append(ids, c.ID)
	}
	if exp := []string{"", "5", ""}; !reflect.DeepEqual(ids, exp) {
		t.Errorf("IDs = %q, expected %q", ids, exp)
	}
}

func TestEncodeLineBreaks(t *testing.T) {
	var b strings.Builder
	if err := Encode(&b, &Subtitles{Cues: []Cue{{Start: time.Second, End: 2 * time.Second, Lines: []string{"a\rb\r\n\nc"}}}}); err != nil {
		t.Fatal(err)
	}
	if exp := "1\n00:00:01,000 --> 00:00:02,000\na\nb\nc\n\n"; b.String() != exp {
		t.Errorf("Encode() = %q, expected %q", b.String(), exp)
	}
}
