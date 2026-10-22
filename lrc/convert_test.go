package lrc

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"samuelks.com/timedtext"
)

func TestToTrack(t *testing.T) {
	const in = "[ti:T]\n[ar:A]\n[length:3:05]\n[offset:500]\n" +
		"[00:01.00]M: <00:01.00>a <00:01.50>b<00:02.00>\nc\n[00:03.00]\n[00:04.00]d"
	lyr, err := Decode(strings.NewReader(in), DecodeOptions{Voices: true})
	if err != nil {
		t.Fatal(err)
	}
	s, err := lyr.ToTrack()
	if err != nil {
		t.Fatal(err)
	}
	ms := func(n int) time.Duration { return time.Duration(n) * time.Millisecond }
	exp := &timedtext.Track{
		Metadata: timedtext.Metadata{Title: "T", Artist: "A", Duration: 3*time.Minute + 5*time.Second},
		Cues: []timedtext.Cue{
			{
				Start: ms(500),
				End:   ms(2500),
				Text:  "a b\nc",
				Voice: timedtext.VoiceMale,
				Spans: []timedtext.Span{
					{Start: ms(500), End: ms(1000), Text: "a "},
					{Start: ms(1000), End: ms(1500), Text: "b"},
					{Start: ms(1500), Text: "\nc"},
				},
			},
			{Start: ms(3500), Text: "d", Voice: timedtext.VoiceMale},
		},
	}
	if !reflect.DeepEqual(s, exp) {
		t.Errorf("ToTrack() = %+v, expected %+v", s, exp)
	}
}

func TestToTrackSorts(t *testing.T) {
	const in = "[00:01.00][00:30.00]chorus\n[00:05.00]verse\n[00:10.00]\n[00:31.00]\n"
	lyr, err := Decode(strings.NewReader(in), DecodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	tr, err := lyr.ToTrack()
	if err != nil {
		t.Fatal(err)
	}
	exp := []timedtext.Cue{
		{Start: time.Second, Text: "chorus"},
		{Start: 5 * time.Second, End: 10 * time.Second, Text: "verse"},
		{Start: 30 * time.Second, End: 31 * time.Second, Text: "chorus"},
	}
	if !reflect.DeepEqual(tr.Cues, exp) {
		t.Errorf("ToTrack().Cues = %+v, expected %+v", tr.Cues, exp)
	}
	if end := tr.CueEnd(0); end != 5*time.Second {
		t.Errorf("CueEnd(0) = %s, expected 5s", end)
	}
	if lyr.Phrases[1].Timecode != 30*time.Second {
		t.Error("ToTrack sorted the lyrics' phrases")
	}
}

func TestToTrackTags(t *testing.T) {
	lyr := &Lyrics{Tags: []Tag{{TagLength, "x"}}, Phrases: []Phrase{{Lines: []string{"a"}}}}
	tr, err := lyr.ToTrack()
	if err != nil {
		t.Fatalf("ToTrack() with an invalid length failed: %s", err)
	}
	if tr.Metadata.Duration != 0 || len(tr.Cues) != 1 {
		t.Errorf("ToTrack() = %+v", tr)
	}
	lyr.SetTag(TagOffset, "x")
	if _, err := lyr.ToTrack(); err == nil {
		t.Error("expected error for an invalid offset")
	}
}

func TestFromTrackEmptyText(t *testing.T) {
	l := FromTrack(&timedtext.Track{Cues: []timedtext.Cue{{Start: time.Second}}})
	exp := []Phrase{{Timecode: time.Second}}
	if !reflect.DeepEqual(l.Phrases, exp) {
		t.Errorf("Phrases = %s, expected %s", formatPhrases(l.Phrases), formatPhrases(exp))
	}
}

func TestFromTrack(t *testing.T) {
	s := &timedtext.Track{
		Metadata: timedtext.Metadata{Title: "T", Duration: 65 * time.Second},
		Cues: []timedtext.Cue{
			{
				Start: time.Second,
				End:   3 * time.Second,
				Text:  "a b",
				Voice: timedtext.VoiceFemale,
				Spans: []timedtext.Span{
					{Start: time.Second, End: 2 * time.Second, Text: "a "},
					{Start: 2 * time.Second, End: 3 * time.Second, Text: "b"},
				},
			},
			{Start: 4 * time.Second, End: 5 * time.Second, Text: "c\nd", Voice: "v1"},
			{Start: 5 * time.Second, Text: "e"},
		},
	}
	var b strings.Builder
	if err := Encode(&b, FromTrack(s), EncodeOptions{}); err != nil {
		t.Fatal(err)
	}
	exp := "[ti: T]\n[length: 1:05]\n\n" +
		"[00:01.00]F: <00:01.00>a <00:02.00>b<00:03.00>\n" +
		"[00:03.00]\n" +
		"[00:04.00]c d\n" +
		"[00:05.00]e\n"
	assertSameLines(t, b.String(), exp)
}
