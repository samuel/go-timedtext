package lrc

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParseTimecode(t *testing.T) {
	cases := map[string]time.Duration{
		"00:00.03":    time.Millisecond * 30,
		"00:00:00.03": time.Millisecond * 30,
		"12:34:56.78": 12*time.Hour + 34*time.Minute + 56*time.Second + 780*time.Millisecond,
		"0:05.00":     5 * time.Second,
		"0:00:05.00":  5 * time.Second,
		"00:01.5":     1500 * time.Millisecond,
		"00:01.123":   1123 * time.Millisecond,
		"00:01.1234":  1123400 * time.Microsecond,
		"00:01":       time.Second,
		"01:02:03":    time.Minute + 2*time.Second + 30*time.Millisecond,
		"01:02:003":   time.Hour + 2*time.Minute + 3*time.Second,
		"1:01:02:03":  time.Hour + time.Minute + 2*time.Second + 30*time.Millisecond,
		"75:00.00":    75 * time.Minute,
	}
	for s, d := range cases {
		t.Run(s, func(t *testing.T) {
			tc, err := ParseTimecode(s)
			if err != nil {
				t.Fatalf("ParseTimecode(%q) failed: %s", s, err)
			}
			if tc != d {
				t.Fatalf("ParseTimecode(%q) = %s, expected %s", s, tc, d)
			}
		})
	}
}

func TestDecode(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		opt     DecodeOptions
		tags    []Tag
		phrases []Phrase
	}{
		{
			name:    "BOM",
			in:      "\xEF\xBB\xBF[ar:Someone]\n[00:01.00]x",
			tags:    []Tag{{TagArtist, "Someone"}},
			phrases: []Phrase{{Timecode: time.Second, Lines: []string{"x"}}},
		},
		{
			name:    "tag after header is text",
			in:      "[ti:Title]\n[00:01.00]a\n[note:x]",
			tags:    []Tag{{TagTitle, "Title"}},
			phrases: []Phrase{{Timecode: time.Second, Lines: []string{"a", "[note:x]"}}},
		},
		{
			name: "text before first timecode",
			in:   "intro\n[00:01.00]a",
			tags: nil,
			phrases: []Phrase{
				{Lines: []string{"intro"}},
				{Timecode: time.Second, Lines: []string{"a"}},
			},
		},
		{
			name: "repeated timecodes",
			in:   "[00:21.10][00:45.10]chorus\nmore\n[00:30.00]verse",
			tags: nil,
			phrases: []Phrase{
				{Timecode: 21100 * time.Millisecond, Lines: []string{"chorus", "more"}},
				{Timecode: 45100 * time.Millisecond, Lines: []string{"chorus", "more"}},
				{Timecode: 30 * time.Second, Lines: []string{"verse"}},
			},
		},
		{
			name: "timecodes separated by text",
			in:   "[00:18.07]you[00:18.15] write",
			tags: nil,
			phrases: []Phrase{
				{Timecode: 18070 * time.Millisecond, Lines: []string{"you"}},
				{Timecode: 18150 * time.Millisecond, Lines: []string{"write"}},
			},
		},
		{
			name: "timecode forms",
			in:   "[00:01]a\n[00:02:50]b",
			tags: nil,
			phrases: []Phrase{
				{Timecode: time.Second, Lines: []string{"a"}},
				{Timecode: 2500 * time.Millisecond, Lines: []string{"b"}},
			},
		},
		{
			name:    "encoding tag",
			in:      "[Encoding:utf-8]\n[00:01.00]a",
			tags:    []Tag{{TagEncoding, "utf-8"}},
			phrases: []Phrase{{Timecode: time.Second, Lines: []string{"a"}}},
		},
		{
			name: "word timing",
			in:   "[00:01.00] <00:01.10>Hel<00:01.20>lo <00:01.50>world <00:02.00>\nmore",
			phrases: []Phrase{{
				Timecode: time.Second,
				Lines:    []string{"Hello world", "more"},
				Words: []Word{
					{1100 * time.Millisecond, "Hel"},
					{1200 * time.Millisecond, "lo "},
					{1500 * time.Millisecond, "world "},
					{2 * time.Second, "\nmore"},
				},
			}},
		},
		{
			name: "word timing on a later line",
			in:   "[00:01.00]a\nb<00:02.00>c",
			phrases: []Phrase{{
				Timecode: time.Second,
				Lines:    []string{"a", "bc"},
				Words:    []Word{{time.Second, "a\nb"}, {2 * time.Second, "c"}},
			}},
		},
		{
			name: "voices",
			in:   "[00:01.00]a\n[00:02.00]F: b\n[00:03.00]c\n[00:04.00]M:\n[00:05.00]D:e",
			opt:  DecodeOptions{Voices: true},
			phrases: []Phrase{
				{Timecode: 1 * time.Second, Lines: []string{"a"}},
				{Timecode: 2 * time.Second, Voice: VoiceFemale, Lines: []string{"b"}},
				{Timecode: 3 * time.Second, Voice: VoiceFemale, Lines: []string{"c"}},
				{Timecode: 4 * time.Second, Voice: VoiceMale},
				{Timecode: 5 * time.Second, Voice: VoiceDuet, Lines: []string{"e"}},
			},
		},
		{
			name:    "voices disabled",
			in:      "[00:01.00]F: a",
			phrases: []Phrase{{Timecode: time.Second, Lines: []string{"F: a"}}},
		},
		{
			name: "lenient",
			in:   "[00:01.00]a[99999999999:01.00]b",
			opt:  DecodeOptions{Lenient: true},
			phrases: []Phrase{
				{Timecode: time.Second, Lines: []string{"a[99999999999:01.00]b"}},
			},
		},
		{
			name: "repeated timecodes with words",
			in:   "[00:10.00][00:04.00]<00:10.00>a <00:10.50>b\nc<00:11.00>d",
			phrases: []Phrase{
				{
					Timecode: 10 * time.Second,
					Lines:    []string{"a b", "cd"},
					Words:    []Word{{10 * time.Second, "a "}, {10500 * time.Millisecond, "b\nc"}, {11 * time.Second, "d"}},
				},
				{
					Timecode: 4 * time.Second,
					Lines:    []string{"a b", "cd"},
					Words:    []Word{{4 * time.Second, "a "}, {4500 * time.Millisecond, "b\nc"}, {5 * time.Second, "d"}},
				},
			},
		},
		{
			name: "word before a repeated timecode is clamped",
			in:   "[00:05.00][00:01.00]<00:02.00>a",
			phrases: []Phrase{
				{Timecode: 5 * time.Second, Lines: []string{"a"}, Words: []Word{{2 * time.Second, "a"}}},
				{Timecode: time.Second, Lines: []string{"a"}, Words: []Word{{0, "a"}}},
			},
		},
		{
			name:    "CR line endings",
			in:      "[ti:T]\r[00:01.00]a\rb\r\n[00:02.00]c\r",
			tags:    []Tag{{TagTitle, "T"}},
			phrases: []Phrase{{Timecode: time.Second, Lines: []string{"a", "b"}}, {Timecode: 2 * time.Second, Lines: []string{"c"}}},
		},
		{
			name: "duplicate tags",
			in:   "[ar:a]\n[ti:t]\n[ar:b]",
			tags: []Tag{{TagArtist, "a"}, {TagTitle, "t"}, {TagArtist, "b"}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lyr, err := Decode(strings.NewReader(c.in), c.opt)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(lyr.Tags, c.tags) {
				t.Errorf("Tags = %v, expected %v", lyr.Tags, c.tags)
			}
			if !reflect.DeepEqual(lyr.Phrases, c.phrases) {
				t.Errorf("Phrases = %s, expected %s", formatPhrases(lyr.Phrases), formatPhrases(c.phrases))
			}
		})
	}
}

func formatPhrases(phrases []Phrase) string {
	var b strings.Builder
	for _, p := range phrases {
		fmt.Fprintf(&b, "\n\t%s %q %q %v", p.Timecode, p.Voice, p.Lines, p.Words)
	}
	return b.String()
}

func TestDecodeSyntaxError(t *testing.T) {
	_, err := Decode(strings.NewReader("[00:01.00]a\n\n[99999999999:01.00]b"), DecodeOptions{})
	var se *SyntaxError
	if !errors.As(err, &se) {
		t.Fatalf("expected *SyntaxError, got %v", err)
	}
	if se.Line != 3 {
		t.Fatalf("Line = %d, expected 3", se.Line)
	}
}

func TestDecodeLongLine(t *testing.T) {
	text := strings.Repeat("a", 100_000)
	lyr, err := Decode(strings.NewReader("[00:01.00]"+text), DecodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(lyr.Phrases) != 1 || lyr.Phrases[0].Lines[0] != text {
		t.Fatal("long line not decoded")
	}
}

func TestDecoding(t *testing.T) {
	b, err := os.ReadFile("testdata/example-in.lrc")
	if err != nil {
		t.Fatal(err)
	}
	lyr, err := Decode(bytes.NewReader(b), DecodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	expTags := []Tag{
		{TagArtist, "Example Artist"},
		{TagAlbum, "Example Album"},
		{TagTitle, "Example Song"},
		{TagAuthor, "Example Writer"},
		{TagLength, "82:05"},
		{TagBy, "timedtext tests"},
	}
	if !reflect.DeepEqual(lyr.Tags, expTags) {
		t.Errorf("Tags = %v, expected %v", lyr.Tags, expTags)
	}
	ms := func(n int) time.Duration { return time.Duration(n) * time.Millisecond }
	chorus := []string{"a chorus shared by", "two adjacent timecodes"}
	exp := []Phrase{
		{Timecode: ms(30), Lines: []string{"an opening line", "with an untimed second line"}},
		{Timecode: ms(3500), Lines: []string{"a timecode alone on its line", "followed by two lines of text"}},
		{Timecode: ms(7370), Lines: []string{"text after the timecode", "and an untimed line after it"}},
		{Timecode: ms(13960), Lines: chorus},
		{Timecode: ms(13970), Lines: chorus},
		{Timecode: ms(18070), Lines: []string{"one"}},
		{Timecode: ms(18150), Lines: []string{"two on the same line", "and the line after them"}},
		{
			Timecode: ms(24030),
			Lines:    []string{"timed words here"},
			Words:    []Word{{ms(24030), "timed "}, {ms(24500), "words "}, {ms(25000), "here"}, {ms(25400), ""}},
		},
		{Timecode: ms(26100)},
		{Timecode: ms(30500), Lines: []string{"centiseconds after a colon"}},
		{Timecode: ms(35125), Lines: []string{"a fraction rounded to centiseconds"}},
		{Timecode: time.Hour + 2*time.Minute + 3*time.Second, Lines: []string{"three fields read as hours"}},
		{Timecode: 40 * time.Second, Lines: []string{"tabs and spaces around text"}},
		{
			Timecode: 45 * time.Second,
			Lines:    []string{"repeated words"},
			Words:    []Word{{ms(45000), "repeated "}, {ms(45500), "words"}},
		},
		{
			Timecode: 47 * time.Second,
			Lines:    []string{"repeated words"},
			Words:    []Word{{ms(47000), "repeated "}, {ms(47500), "words"}},
		},
		{Timecode: 48 * time.Second},
		{Timecode: 59*time.Minute + 58*time.Second, Lines: []string{"just before an hour"}},
		{Timecode: 75 * time.Minute, Lines: []string{"minutes past sixty"}},
		{Timecode: time.Hour + 20*time.Minute},
	}
	if !reflect.DeepEqual(lyr.Phrases, exp) {
		t.Errorf("Phrases = %s, expected %s", formatPhrases(lyr.Phrases), formatPhrases(exp))
	}
}

func TestTagKeyName(t *testing.T) {
	cases := map[TagKey]string{
		TagArtist: "Artist",
		"foo":     "Foo",
		"":        "",
	}
	for k, exp := range cases {
		if n := k.Name(); n != exp {
			t.Errorf("TagKey(%q).Name() = %q, expected %q", k, n, exp)
		}
	}
}

func TestTags(t *testing.T) {
	lyr := &Lyrics{Tags: []Tag{{TagArtist, "a"}, {TagTitle, "t"}, {TagArtist, "b"}, {TagLength, "3:25.5"}}}
	if v := lyr.Artist(); v != "b" {
		t.Errorf("Artist() = %q, expected %q", v, "b")
	}
	if v := lyr.Title(); v != "t" {
		t.Errorf("Title() = %q, expected %q", v, "t")
	}
	if d, err := lyr.Length(); err != nil || d != 3*time.Minute+25500*time.Millisecond {
		t.Errorf("Length() = %s, %v", d, err)
	}
	lyr.SetTag(TagArtist, "c")
	lyr.SetTag(TagAlbum, "al")
	exp := []Tag{{TagArtist, "c"}, {TagTitle, "t"}, {TagLength, "3:25.5"}, {TagAlbum, "al"}}
	if !reflect.DeepEqual(lyr.Tags, exp) {
		t.Errorf("Tags after SetTag = %v, expected %v", lyr.Tags, exp)
	}
	lyr.DeleteTag(TagTitle)
	exp = []Tag{{TagArtist, "c"}, {TagLength, "3:25.5"}, {TagAlbum, "al"}}
	if !reflect.DeepEqual(lyr.Tags, exp) {
		t.Errorf("Tags after DeleteTag = %v, expected %v", lyr.Tags, exp)
	}
	lyr.SetTag(TagLength, "x")
	if _, err := lyr.Length(); err == nil {
		t.Error("expected error for invalid length")
	}
}

func TestSort(t *testing.T) {
	lyr := &Lyrics{Phrases: []Phrase{
		{Timecode: 2 * time.Second, Lines: []string{"b"}},
		{Timecode: time.Second, Lines: []string{"a1"}},
		{Timecode: time.Second, Lines: []string{"a2"}},
	}}
	lyr.Sort()
	var got []string
	for _, p := range lyr.Phrases {
		got = append(got, p.Lines[0])
	}
	if exp := []string{"a1", "a2", "b"}; !reflect.DeepEqual(got, exp) {
		t.Errorf("sorted = %q, expected %q", got, exp)
	}
}

func TestMarshalText(t *testing.T) {
	const in = "[ti: T]\n\n[00:01.00]a\n"
	var lyr Lyrics
	if err := lyr.UnmarshalText([]byte(in)); err != nil {
		t.Fatal(err)
	}
	b, err := lyr.MarshalText()
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != in {
		t.Errorf("MarshalText() = %q, expected %q", b, in)
	}
}

func TestOffset(t *testing.T) {
	cases := map[string]time.Duration{
		"":     0,
		"500":  500 * time.Millisecond,
		"+500": 500 * time.Millisecond,
		"-200": -200 * time.Millisecond,
	}
	for v, exp := range cases {
		lyr := &Lyrics{Tags: []Tag{{TagOffset, v}}}
		off, err := lyr.Offset()
		if err != nil {
			t.Fatalf("Offset() with %q failed: %s", v, err)
		}
		if off != exp {
			t.Errorf("Offset() with %q = %s, expected %s", v, off, exp)
		}
	}
	for _, v := range []string{"abc", "9223372036855", "-9223372036855", "99999999999999999999"} {
		lyr := &Lyrics{Tags: []Tag{{TagOffset, v}}}
		if _, err := lyr.Offset(); err == nil {
			t.Errorf("expected error for offset %q", v)
		}
	}
}

func TestApplyOffset(t *testing.T) {
	lyr := &Lyrics{
		Tags: []Tag{{TagOffset, "+500"}},
		Phrases: []Phrase{
			{Timecode: 200 * time.Millisecond},
			{Timecode: 2 * time.Second, Words: []Word{{2 * time.Second, "a"}}},
		},
	}
	if err := lyr.ApplyOffset(); err != nil {
		t.Fatal(err)
	}
	if len(lyr.Tags) != 0 {
		t.Error("offset tag not removed")
	}
	if tc := lyr.Phrases[0].Timecode; tc != 0 {
		t.Errorf("Phrases[0].Timecode = %s, expected 0", tc)
	}
	if tc := lyr.Phrases[1].Timecode; tc != 1500*time.Millisecond {
		t.Errorf("Phrases[1].Timecode = %s, expected 1.5s", tc)
	}
	if tc := lyr.Phrases[1].Words[0].Timecode; tc != 1500*time.Millisecond {
		t.Errorf("Phrases[1].Words[0].Timecode = %s, expected 1.5s", tc)
	}
}
