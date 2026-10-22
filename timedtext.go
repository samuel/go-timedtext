// Package timedtext defines a format-neutral model for timed text such as lyrics and subtitles.
// Subpackages read and write specific formats and convert to and from this model. Conversions may lose
// format-specific details.
package timedtext

import (
	"cmp"
	"slices"
	"time"
)

// DefaultCueDuration is how long a cue is shown when neither its end nor the next cue's start is known.
const DefaultCueDuration = 5 * time.Second

// Track is a sequence of timed cues plus metadata. Cues may overlap. Styling and positioning are not part
// of the model.
type Track struct {
	Metadata Metadata
	Cues     []Cue
}

// Metadata describes a track. Empty fields are unknown.
type Metadata struct {
	Title    string
	Artist   string
	Album    string
	Author   string // Lyricist or writer
	Language string
	Duration time.Duration
}

// Voice identifies who sings or speaks a cue. Formats may use other values, such as speaker names or
// TTML agent IDs.
type Voice string

// Voices with a standard meaning across formats.
const (
	VoiceMale   Voice = "male"
	VoiceFemale Voice = "female"
	VoiceDuet   Voice = "duet"
)

// Cue is a timed piece of text. Text may contain newlines when a cue spans several display lines.
type Cue struct {
	ID    string // Empty when the format has none
	Start time.Duration
	End   time.Duration // Zero when unknown
	Text  string
	Voice Voice
	Spans []Span // Timing within the cue, if known
}

// Span is a timed part of a cue, such as a word or syllable. Text includes any surrounding whitespace, so
// concatenating the spans of a cue gives its text.
type Span struct {
	Start time.Duration
	End   time.Duration // Zero when unknown
	Text  string
}

// CueEnd returns the end of cue i. When unknown, it is the start of the first following cue that starts
// later, as long as that's the next cue or follows only cues with the same start. Otherwise it's
// DefaultCueDuration after the cue's start.
func (t *Track) CueEnd(i int) time.Duration {
	c := t.Cues[i]
	if c.End > c.Start {
		return c.End
	}
	for _, next := range t.Cues[i+1:] {
		if next.Start > c.Start {
			return next.Start
		}
		if next.Start < c.Start {
			break
		}
	}
	return c.Start + DefaultCueDuration
}

// Sort sorts cues by start time, keeping the order of cues that start together.
func (t *Track) Sort() {
	slices.SortStableFunc(t.Cues, func(a, b Cue) int { return cmp.Compare(a.Start, b.Start) })
}
