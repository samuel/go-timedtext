package lrc

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"samuelks.com/timedtext"
)

var (
	voiceToTrack = map[Voice]timedtext.Voice{
		VoiceMale:   timedtext.VoiceMale,
		VoiceFemale: timedtext.VoiceFemale,
		VoiceDuet:   timedtext.VoiceDuet,
	}
	voiceFromTrack = map[timedtext.Voice]Voice{
		timedtext.VoiceMale:   VoiceMale,
		timedtext.VoiceFemale: VoiceFemale,
		timedtext.VoiceDuet:   VoiceDuet,
	}
)

// ToTrack converts the lyrics to the format-neutral model with the offset tag applied. Phrases are sorted
// by timecode. Each phrase becomes a cue, words become spans, and a phrase with no text ends the cue
// before it. An invalid offset tag is an error, and an invalid length tag is ignored.
func (l *Lyrics) ToTrack() (*timedtext.Track, error) {
	off, err := l.Offset()
	if err != nil {
		return nil, err
	}
	// An invalid length only loses metadata.
	length, _ := l.Length()
	phrases := slices.Clone(l.Phrases)
	sortPhrases(phrases)
	shift := func(tc time.Duration) time.Duration { return max(tc-off, 0) }
	t := &timedtext.Track{
		Metadata: timedtext.Metadata{
			Title:    l.Tag(TagTitle),
			Artist:   l.Tag(TagArtist),
			Album:    l.Tag(TagAlbum),
			Author:   l.Tag(TagAuthor),
			Language: l.Tag(TagLanguage),
			Duration: length,
		},
	}
	for _, p := range phrases {
		start := shift(p.Timecode)
		if len(p.Lines) == 0 && len(p.Words) == 0 {
			if n := len(t.Cues); n != 0 && t.Cues[n-1].End == 0 && start > t.Cues[n-1].Start {
				t.Cues[n-1].End = start
			}
			continue
		}
		cue := timedtext.Cue{
			Start: start,
			Text:  strings.Join(p.Lines, "\n"),
			Voice: voiceToTrack[p.Voice],
		}
		for i, w := range p.Words {
			if w.Text == "" {
				continue
			}
			span := timedtext.Span{Start: shift(w.Timecode), Text: w.Text}
			if i+1 < len(p.Words) {
				span.End = shift(p.Words[i+1].Timecode)
			}
			cue.Spans = append(cue.Spans, span)
		}
		t.Cues = append(t.Cues, cue)
	}
	return t, nil
}

// FromTrack converts a track from the format-neutral model. A cue end that isn't the start of the next cue
// is written as a phrase with no text. Voices other than male, female, and duet are dropped.
func FromTrack(t *timedtext.Track) *Lyrics {
	l := &Lyrics{}
	m := t.Metadata
	for _, tag := range []Tag{
		{TagTitle, m.Title},
		{TagArtist, m.Artist},
		{TagAlbum, m.Album},
		{TagAuthor, m.Author},
		{TagLanguage, m.Language},
	} {
		if tag.Value != "" {
			l.Tags = append(l.Tags, tag)
		}
	}
	if m.Duration > 0 {
		sec := int64(m.Duration.Round(time.Second) / time.Second)
		l.Tags = append(l.Tags, Tag{TagLength, fmt.Sprintf("%d:%02d", sec/60, sec%60)})
	}
	for i, c := range t.Cues {
		p := Phrase{
			Timecode: c.Start,
			Voice:    voiceFromTrack[c.Voice],
		}
		if c.Text != "" {
			p.Lines = strings.Split(c.Text, "\n")
		}
		for j, s := range c.Spans {
			p.Words = append(p.Words, Word{Timecode: s.Start, Text: s.Text})
			if s.End > s.Start && (j+1 == len(c.Spans) || c.Spans[j+1].Start != s.End) {
				p.Words = append(p.Words, Word{Timecode: s.End})
			}
		}
		l.Phrases = append(l.Phrases, p)
		if c.End > c.Start && (i+1 == len(t.Cues) || t.Cues[i+1].Start > c.End) {
			l.Phrases = append(l.Phrases, Phrase{Timecode: c.End, Voice: p.Voice})
		}
	}
	return l
}
