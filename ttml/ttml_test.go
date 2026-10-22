package ttml

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
	"samuelks.com/timedtext"
)

const sample = `<?xml version="1.0" encoding="UTF-8"?>
<tt xmlns="http://www.w3.org/ns/ttml" xmlns:ttm="http://www.w3.org/ns/ttml#metadata"
    xmlns:itunes="http://music.apple.com/lyric-ttml-internal" xml:lang="en" itunes:timing="Word">
  <head>
    <metadata>
      <ttm:title>Song</ttm:title>
      <ttm:agent type="person" xml:id="v1"><ttm:name type="full">Alice</ttm:name></ttm:agent>
      <ttm:agent type="group" xml:id="v2"/>
      <iTunesMetadata xmlns="http://music.apple.com/lyric-ttml-internal">
        <songwriters><songwriter>Writer One</songwriter><songwriter> Writer &amp; Two </songwriter></songwriters>
      </iTunesMetadata>
    </metadata>
  </head>
  <body dur="3:05.5">
    <div begin="1.000" end="9.5s" itunes:tr-part="Verse">
      <p xml:id="L1" begin="1s" end="3s" ttm:agent="v1">
        <span begin="1s" end="1.5s">Hel</span><span begin="1.5s" end="2s">lo</span>
        <span begin="2s" end="3s">world</span>
        <span ttm:role="x-bg"><span begin="2.5s" end="3s">(oh)</span></span>
      </p>
      <p begin="00:00:04.000" dur="2s" ttm:agent="v2">Plain &amp; simple<br/>
        second line</p>
    </div>
  </body>
</tt>`

func TestDecode(t *testing.T) {
	doc, err := Decode(strings.NewReader(sample), DecodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	ms := func(n int) time.Duration { return time.Duration(n) * time.Millisecond }
	exp := &Document{
		Lang:        "en",
		Title:       "Song",
		Songwriters: []string{"Writer One", "Writer & Two"},
		Agents:      []Agent{{ID: "v1", Type: "person", Name: "Alice"}, {ID: "v2", Type: "group"}},
		Duration:    3*time.Minute + ms(5500),
		Divs: []Div{{
			Begin: ms(1000),
			End:   ms(9500),
			Paragraphs: []Paragraph{
				{
					ID:    "L1",
					Begin: ms(1000),
					End:   ms(3000),
					Agent: "v1",
					Spans: []Span{
						{Timed: true, Begin: ms(1000), End: ms(1500), Text: "Hel"},
						{Timed: true, Begin: ms(1500), End: ms(2000), Text: "lo"},
						{Text: " "},
						{Timed: true, Begin: ms(2000), End: ms(3000), Text: "world"},
						{Text: " "},
						{Timed: true, Begin: ms(2500), End: ms(3000), Role: "x-bg", Text: "(oh)"},
					},
				},
				{
					Begin: ms(4000),
					End:   ms(6000),
					Agent: "v2",
					Spans: []Span{{Text: "Plain & simple"}, {Text: "\n"}, {Text: "second line"}},
				},
			},
		}},
	}
	if !reflect.DeepEqual(doc, exp) {
		t.Errorf("Decode() = %+v\nexpected %+v", doc, exp)
	}
}

func TestDecodeErrors(t *testing.T) {
	cases := map[string]int{
		`<html/>`: 1,
		"<tt><body><div>\n<p begin=\"x\">a</p></div></body></tt>": 2,
		"<tt>\n<body>":         2,
		"<tt>\n<body></tt>":    2,
		"<tt><p a=b></p></tt>": 1,
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

func TestDecodeWhitespace(t *testing.T) {
	const in = `<tt><body><div><p> <x/> a <x/> <span>b</span> <br/> <x/> c <x/> </p></div></body></tt>`
	doc, err := Decode(strings.NewReader(in), DecodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if text := doc.Divs[0].Paragraphs[0].Text(); text != "a b\nc" {
		t.Errorf("Text() = %q, expected %q", text, "a b\nc")
	}
}

func TestEncodeZeroTimes(t *testing.T) {
	doc := &Document{Divs: []Div{{Paragraphs: []Paragraph{
		{End: time.Second, Spans: []Span{{Timed: true, Text: "a"}, {Text: " "}, {Timed: true, End: time.Second, Text: "b"}}},
		{Spans: []Span{{Text: "c"}}},
	}}}}
	var b strings.Builder
	if err := Encode(&b, doc); err != nil {
		t.Fatal(err)
	}
	exp := `<p begin="00:00:00.000" end="00:00:01.000"><span begin="00:00:00.000">a</span> ` +
		`<span begin="00:00:00.000" end="00:00:01.000">b</span></p>` + "\n      <p>c</p>"
	if !strings.Contains(b.String(), exp) {
		t.Errorf("Encode() =\n%s\nexpected it to contain\n%s", b.String(), exp)
	}
	doc2, err := Decode(strings.NewReader(b.String()), DecodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(doc2, doc) {
		t.Errorf("round trip = %+v, expected %+v", doc2, doc)
	}
}

func TestRoundTrip(t *testing.T) {
	doc, err := Decode(strings.NewReader(sample), DecodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	if err := Encode(&b, doc); err != nil {
		t.Fatal(err)
	}
	doc2, err := Decode(strings.NewReader(b.String()), DecodeOptions{})
	if err != nil {
		t.Fatalf("decoding encoded document: %s\n%s", err, b.String())
	}
	if !reflect.DeepEqual(doc, doc2) {
		t.Errorf("round trip changed the document:\n%+v\n%+v\n%s", doc, doc2, b.String())
	}
}

func TestParseTime(t *testing.T) {
	cases := map[string]time.Duration{
		"1.5":          1500 * time.Millisecond,
		"1.5s":         1500 * time.Millisecond,
		"250ms":        250 * time.Millisecond,
		"2m":           2 * time.Minute,
		"1h":           time.Hour,
		"1:02.5":       62500 * time.Millisecond,
		"00:01:02.500": 62500 * time.Millisecond,
	}
	for s, exp := range cases {
		d, err := ParseTime(s)
		if err != nil {
			t.Errorf("ParseTime(%q) failed: %s", s, err)
		} else if d != exp {
			t.Errorf("ParseTime(%q) = %s, expected %s", s, d, exp)
		}
	}
	for _, s := range []string{"", "s", "10f", "-1s", "1e3s", "1..5s"} {
		if _, err := ParseTime(s); err == nil {
			t.Errorf("ParseTime(%q) succeeded, expected an error", s)
		}
	}
}

func TestTrack(t *testing.T) {
	doc, err := Decode(strings.NewReader(sample), DecodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	tr := doc.ToTrack()
	ms := func(n int) time.Duration { return time.Duration(n) * time.Millisecond }
	exp := &timedtext.Track{
		Metadata: timedtext.Metadata{
			Title:    "Song",
			Author:   "Writer One, Writer & Two",
			Language: "en",
			Duration: 3*time.Minute + ms(5500),
		},
		Cues: []timedtext.Cue{
			{
				ID:    "L1",
				Start: ms(1000),
				End:   ms(3000),
				Text:  "Hello world (oh)",
				Voice: "Alice",
				Spans: []timedtext.Span{
					{Start: ms(1000), End: ms(1500), Text: "Hel"},
					{Start: ms(1500), End: ms(2000), Text: "lo "},
					{Start: ms(2000), End: ms(3000), Text: "world "},
					{Start: ms(2500), End: ms(3000), Text: "(oh)"},
				},
			},
			{Start: ms(4000), End: ms(6000), Text: "Plain & simple\nsecond line", Voice: "v2"},
		},
	}
	if !reflect.DeepEqual(tr, exp) {
		t.Errorf("ToTrack() = %+v\nexpected %+v", tr, exp)
	}

	tr.Cues[0].Spans[1].Text = "lo\n"
	doc = FromTrack(tr)
	var b strings.Builder
	if err := Encode(&b, doc); err != nil {
		t.Fatal(err)
	}
	expText := `<?xml version="1.0" encoding="UTF-8"?>
<tt xmlns="http://www.w3.org/ns/ttml" xmlns:ttm="http://www.w3.org/ns/ttml#metadata" xml:lang="en">
  <head>
    <metadata>
      <ttm:title>Song</ttm:title>
      <ttm:agent xml:id="v1" type="person"><ttm:name type="full">Alice</ttm:name></ttm:agent>
      <ttm:agent xml:id="v2" type="person"><ttm:name type="full">v2</ttm:name></ttm:agent>
      <iTunesMetadata xmlns="http://music.apple.com/lyric-ttml-internal"><songwriters><songwriter>Writer One, Writer &amp; Two</songwriter></songwriters></iTunesMetadata>
    </metadata>
  </head>
  <body dur="00:03:05.500">
    <div begin="00:00:01.000" end="00:00:06.000">
      <p xml:id="L1" begin="00:00:01.000" end="00:00:03.000" ttm:agent="v1">` +
		`<span begin="00:00:01.000" end="00:00:01.500">Hel</span>` +
		`<span begin="00:00:01.500" end="00:00:02.000">lo</span><br/>` +
		`<span begin="00:00:02.000" end="00:00:03.000">world</span> ` +
		`<span begin="00:00:02.500" end="00:00:03.000">(oh)</span></p>
      <p begin="00:00:04.000" end="00:00:06.000" ttm:agent="v2">Plain &amp; simple<br/>second line</p>
    </div>
  </body>
</tt>
`
	if b.String() != expText {
		t.Errorf("FromTrack() encoded:\n%s\nexpected:\n%s", b.String(), expText)
	}
}

func TestDecodeCharset(t *testing.T) {
	const body = `<tt xmlns="http://www.w3.org/ns/ttml"><body><div><p>%s</p></div></body></tt>`
	utf16, err := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewEncoder().
		String(`<?xml version="1.0" encoding="UTF-16"?>` + fmt.Sprintf(body, "naïve"))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		in   string
		opt  DecodeOptions
		exp  string
	}{
		{"declared", `<?xml version="1.0" encoding="ISO-8859-1"?>` + fmt.Sprintf(body, "caf\xe9"), DecodeOptions{}, "café"},
		{"declared but UTF-8", `<?xml version="1.0" encoding="ISO-8859-1"?>` + fmt.Sprintf(body, "café"), DecodeOptions{}, "café"},
		{"UTF-16", utf16, DecodeOptions{}, "naïve"},
		{"charset option", fmt.Sprintf(body, "\xb3\xf3d\xbc"), DecodeOptions{Charset: charmap.ISO8859_2}, "łódź"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc, err := Decode(strings.NewReader(c.in), c.opt)
			if err != nil {
				t.Fatal(err)
			}
			if p := doc.Divs[0].Paragraphs[0]; p.Text() != c.exp {
				t.Errorf("text = %q, expected %q", p.Text(), c.exp)
			}
		})
	}
}
