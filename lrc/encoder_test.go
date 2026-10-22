package lrc

import (
	"bytes"
	"flag"
	"os"
	"strings"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "update golden files")

func TestEncoding(t *testing.T) {
	testGolden(t, "testdata/example-out.lrc", EncodeOptions{})
}

func TestEncodingMultiline(t *testing.T) {
	testGolden(t, "testdata/example-out-multiline.lrc", EncodeOptions{
		HoursInTimecode: true,
		Multiline:       true,
	})
}

// TestRoundTrip checks that a file in the default output format is unchanged by decoding and encoding.
func TestRoundTrip(t *testing.T) {
	in, err := os.ReadFile("testdata/test.lrc")
	if err != nil {
		t.Fatal(err)
	}
	lyr, err := Decode(bytes.NewReader(in), DecodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := Encode(&b, lyr, EncodeOptions{}); err != nil {
		t.Fatal(err)
	}
	assertSameLines(t, b.String(), string(in))
}

// testGolden decodes the example input, encodes it with opt, and compares the result to the file at goldenPath.
// With -update, it writes the result to goldenPath instead.
func testGolden(t *testing.T, goldenPath string, opt EncodeOptions) {
	t.Helper()
	in, err := os.ReadFile("testdata/example-in.lrc")
	if err != nil {
		t.Fatal(err)
	}
	lyr, err := Decode(bytes.NewReader(in), DecodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := Encode(&b, lyr, opt); err != nil {
		t.Fatal(err)
	}
	if *update {
		if err := os.WriteFile(goldenPath, b.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	exp, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	assertSameLines(t, b.String(), string(exp))
}

func assertSameLines(t *testing.T, got, exp string) {
	t.Helper()
	if got == exp {
		return
	}
	gl := strings.Split(got, "\n")
	el := strings.Split(exp, "\n")
	for i := range max(len(gl), len(el)) {
		var g, e string
		if i < len(gl) {
			g = gl[i]
		}
		if i < len(el) {
			e = el[i]
		}
		if g != e {
			t.Fatalf("Line %d. Expected\n%q\nGot\n%q", i+1, e, g)
		}
	}
	t.Fatalf("Output differs. Expected\n%q\nGot\n%q", exp, got)
}

func TestEncode(t *testing.T) {
	cases := []struct {
		name string
		lyr  *Lyrics
		opt  EncodeOptions
		exp  string
	}{
		{
			name: "newlines and empty lines",
			lyr:  &Lyrics{Phrases: []Phrase{{Timecode: 59996 * time.Millisecond, Lines: []string{"a\nb", "", "c"}}}},
			exp:  "[01:00.00]a b c\n",
		},
		{
			name: "newlines and empty lines multiline",
			lyr:  &Lyrics{Phrases: []Phrase{{Timecode: 59996 * time.Millisecond, Lines: []string{"a\nb", "", "c"}}}},
			opt:  EncodeOptions{Multiline: true},
			exp:  "[01:00.00]\na\nb\nc\n",
		},
		{
			name: "line breaks in tags",
			lyr:  &Lyrics{Tags: []Tag{{TagTitle, "a\n[00:01.00]b\r\nc\rd"}}},
			exp:  "[ti: a [00:01.00]b c d]\n\n",
		},
		{
			name: "carriage returns in lines",
			lyr:  &Lyrics{Phrases: []Phrase{{Timecode: time.Second, Lines: []string{"a\rb\r\nc"}}}},
			exp:  "[00:01.00]a b c\n",
		},
		{
			name: "tag order",
			lyr:  &Lyrics{Tags: []Tag{{TagTitle, "t"}, {TagArtist, "a"}}},
			exp:  "[ti: t]\n[ar: a]\n\n",
		},
		{
			name: "words",
			lyr: &Lyrics{Phrases: []Phrase{{
				Timecode: time.Second,
				Lines:    []string{"ignored"},
				Words:    []Word{{time.Second, "a "}, {2 * time.Second, "b\nc"}, {3 * time.Second, ""}},
			}}},
			exp: "[00:01.00]<00:01.00>a <00:02.00>b c<00:03.00>\n",
		},
		{
			name: "words multiline",
			lyr: &Lyrics{Phrases: []Phrase{{
				Timecode: time.Second,
				Words:    []Word{{time.Second, "a "}, {2 * time.Second, "b\nc"}},
			}}},
			opt: EncodeOptions{Multiline: true},
			exp: "[00:01.00]\n<00:01.00>a <00:02.00>b\nc\n",
		},
		{
			name: "voices",
			lyr: &Lyrics{Phrases: []Phrase{
				{Timecode: 1 * time.Second, Lines: []string{"a"}},
				{Timecode: 2 * time.Second, Voice: VoiceFemale, Lines: []string{"b"}},
				{Timecode: 3 * time.Second, Voice: VoiceFemale, Lines: []string{"c"}},
				{Timecode: 4 * time.Second, Voice: VoiceMale},
			}},
			exp: "[00:01.00]a\n[00:02.00]F: b\n[00:03.00]c\n[00:04.00]M:\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var b bytes.Buffer
			if err := Encode(&b, c.lyr, c.opt); err != nil {
				t.Fatal(err)
			}
			assertSameLines(t, b.String(), c.exp)
		})
	}
}

func TestEncodeNegativeTimecode(t *testing.T) {
	for _, lyr := range []*Lyrics{
		{Phrases: []Phrase{{Timecode: -time.Second}}},
		{Phrases: []Phrase{{Words: []Word{{-time.Second, "a"}}}}},
	} {
		if err := Encode(&bytes.Buffer{}, lyr, EncodeOptions{}); err == nil {
			t.Error("expected error for negative timecode")
		}
	}
}

func TestFormatTimecode(t *testing.T) {
	cases := []struct {
		tc  time.Duration
		opt EncodeOptions
		exp string
	}{
		{0, EncodeOptions{}, "00:00.00"},
		{1234 * time.Millisecond, EncodeOptions{}, "00:01.23"},
		{1235 * time.Millisecond, EncodeOptions{}, "00:01.24"},
		{75 * time.Minute, EncodeOptions{}, "75:00.00"},
		{75 * time.Minute, EncodeOptions{HoursInTimecode: true}, "01:15:00.00"},
		{15 * time.Minute, EncodeOptions{HoursInTimecode: true}, "15:00.00"},
	}
	for _, c := range cases {
		if s := FormatTimecode(c.tc, c.opt); s != c.exp {
			t.Errorf("FormatTimecode(%s, %+v) = %q, expected %q", c.tc, c.opt, s, c.exp)
		}
	}
}
