package vtt

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/encoding/unicode"
	"samuelks.com/timedtext"
)

const sample = "WEBVTT - Song\nKind: captions\n\n" +
	"STYLE\n::cue { color: white }\n\n" +
	"intro\n00:00:01.000 --> 00:00:02.500 align:start\n<v Bob>Hello &amp; <i>welcome</i>\n\n" +
	"NOTE a comment\n\n" +
	"00:00:03.000 --> 00:00:05.000\nOne <00:00:04.000>two\n\n"

func TestDecode(t *testing.T) {
	in := "\xEF\xBB\xBFWEBVTT\r\n\r\n01:02.000 --> 01:03.500\r\na\r\nb"
	f, err := Decode(strings.NewReader(in), DecodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	exp := &File{Cues: []Cue{{Start: 62 * time.Second, End: 63500 * time.Millisecond, Lines: []string{"a", "b"}}}}
	if !reflect.DeepEqual(f, exp) {
		t.Errorf("Decode() = %+v, expected %+v", f, exp)
	}
}

func TestDecodeErrors(t *testing.T) {
	cases := map[string]int{
		"":                                 1,
		"WEBVTTX\n":                        1,
		"00:01.000 --> 00:02.000\na\n":     1,
		"WEBVTT\n\nid\ntext\n":             4,
		"WEBVTT\n\n\n00:01.000 --> x\na\n": 4,
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
	f, err := Decode(strings.NewReader(sample), DecodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Blocks) != 1 || len(f.Cues) != 2 || len(f.Cues[0].After) != 1 {
		t.Fatalf("unexpected structure: %+v", f)
	}
	var b strings.Builder
	if err := Encode(&b, f); err != nil {
		t.Fatal(err)
	}
	if b.String() != sample {
		t.Errorf("Encode() = %q, expected %q", b.String(), sample)
	}
}

func TestTrack(t *testing.T) {
	f, err := Decode(strings.NewReader(sample), DecodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	tr := f.ToTrack()
	exp := []timedtext.Cue{
		{ID: "intro", Start: time.Second, End: 2500 * time.Millisecond, Text: "Hello & welcome", Voice: "Bob"},
		{
			Start: 3 * time.Second,
			End:   5 * time.Second,
			Text:  "One two",
			Spans: []timedtext.Span{
				{Start: 3 * time.Second, End: 4 * time.Second, Text: "One "},
				{Start: 4 * time.Second, End: 5 * time.Second, Text: "two"},
			},
		},
	}
	if !reflect.DeepEqual(tr.Cues, exp) {
		t.Errorf("ToTrack().Cues = %+v, expected %+v", tr.Cues, exp)
	}

	var b strings.Builder
	if err := Encode(&b, FromTrack(tr)); err != nil {
		t.Fatal(err)
	}
	expText := "WEBVTT\n\n" +
		"intro\n00:00:01.000 --> 00:00:02.500\n<v Bob>Hello &amp; welcome\n\n" +
		"00:00:03.000 --> 00:00:05.000\nOne <00:00:04.000>two\n\n"
	if b.String() != expText {
		t.Errorf("FromTrack() encoded = %q, expected %q", b.String(), expText)
	}
}

func TestDecodeCharset(t *testing.T) {
	in, err := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewEncoder().String("WEBVTT\n\n00:01.000 --> 00:02.000\nnaïve\n")
	if err != nil {
		t.Fatal(err)
	}
	f, err := Decode(strings.NewReader(in), DecodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Cues) != 1 || !reflect.DeepEqual(f.Cues[0].Lines, []string{"naïve"}) {
		t.Errorf("Decode() = %+v", f)
	}
}

func TestDecodeLineEndingsAndSettings(t *testing.T) {
	in := "WEBVTT\rLanguage: en\r\r00:01.000 --> 00:02.000\talign:start  line:0\ra\r"
	f, err := Decode(strings.NewReader(in), DecodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	exp := &File{
		Header: "\nLanguage: en",
		Cues:   []Cue{{Start: time.Second, End: 2 * time.Second, Settings: "align:start  line:0", Lines: []string{"a"}}},
	}
	if !reflect.DeepEqual(f, exp) {
		t.Errorf("Decode() = %+v, expected %+v", f, exp)
	}
	if lang := f.Language(); lang != "en" {
		t.Errorf("Language() = %q, expected en", lang)
	}
}

func TestToTrackCues(t *testing.T) {
	cases := []struct {
		name  string
		lines []string
		exp   timedtext.Cue
	}{
		{
			name:  "voice after a tab",
			lines: []string{"<v\tBob>hi"},
			exp:   timedtext.Cue{Start: time.Second, End: 5 * time.Second, Text: "hi", Voice: "Bob"},
		},
		{
			name:  "spans trimmed like the text",
			lines: []string{" hello <00:00:02.000>world ", ""},
			exp: timedtext.Cue{
				Start: time.Second,
				End:   5 * time.Second,
				Text:  "hello world",
				Spans: []timedtext.Span{
					{Start: time.Second, End: 2 * time.Second, Text: "hello "},
					{Start: 2 * time.Second, End: 5 * time.Second, Text: "world"},
				},
			},
		},
		{
			name:  "trailing whitespace span dropped",
			lines: []string{"a <00:00:02.000>b<00:00:03.000> "},
			exp: timedtext.Cue{
				Start: time.Second,
				End:   5 * time.Second,
				Text:  "a b",
				Spans: []timedtext.Span{
					{Start: time.Second, End: 2 * time.Second, Text: "a "},
					{Start: 2 * time.Second, End: 3 * time.Second, Text: "b"},
				},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := &File{Cues: []Cue{{Start: time.Second, End: 5 * time.Second, Lines: c.lines}}}
			if cue := f.ToTrack().Cues[0]; !reflect.DeepEqual(cue, c.exp) {
				t.Errorf("ToTrack() cue = %+v, expected %+v", cue, c.exp)
			}
		})
	}
}

func TestFromTrackEscaping(t *testing.T) {
	tr := &timedtext.Track{
		Metadata: timedtext.Metadata{Language: "en\nfr"},
		Cues: []timedtext.Cue{
			{ID: "NOTE this", Start: 0, End: time.Second, Text: "a --> b", Voice: "A\nB>"},
			{ID: "x-->y\nz", Start: time.Second, End: 2 * time.Second, Text: "c"},
			{ID: "STYLE", Start: 2 * time.Second, End: 3 * time.Second, Text: "d"},
		},
	}
	var b strings.Builder
	if err := Encode(&b, FromTrack(tr)); err != nil {
		t.Fatal(err)
	}
	exp := "WEBVTT\nLanguage: en fr\n\n" +
		"00:00:00.000 --> 00:00:01.000\n<v A B>a --&gt; b\n\n" +
		"x->y z\n00:00:01.000 --> 00:00:02.000\nc\n\n" +
		"00:00:02.000 --> 00:00:03.000\nd\n\n"
	if b.String() != exp {
		t.Errorf("Encode() = %q, expected %q", b.String(), exp)
	}
	f, err := Decode(strings.NewReader(b.String()), DecodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Cues) != 3 || f.Language() != "en fr" {
		t.Errorf("decoded %d cues and language %q, expected 3 and %q", len(f.Cues), f.Language(), "en fr")
	}
}
