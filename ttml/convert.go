package ttml

import (
	"fmt"
	"strings"

	"samuelks.com/timedtext"
)

// ToTrack converts the document to the format-neutral model. Each paragraph becomes a cue, timed spans
// become spans, and a paragraph's voice is its agent's name, or the agent ID when it has no name. The
// songwriters become the author.
func (d *Document) ToTrack() *timedtext.Track {
	names := make(map[string]string, len(d.Agents))
	for _, a := range d.Agents {
		if a.Name != "" {
			names[a.ID] = a.Name
		}
	}
	t := &timedtext.Track{
		Metadata: timedtext.Metadata{
			Title:    d.Title,
			Author:   strings.Join(d.Songwriters, ", "),
			Language: d.Lang,
			Duration: d.Duration,
		},
	}
	for _, div := range d.Divs {
		for _, p := range div.Paragraphs {
			cue := timedtext.Cue{ID: p.ID, Start: p.Begin, End: p.End, Text: p.Text()}
			if p.Agent != "" {
				cue.Voice = timedtext.Voice(p.Agent)
				if n, ok := names[p.Agent]; ok {
					cue.Voice = timedtext.Voice(n)
				}
			}
			// Untimed text joins the span before it, or the first span when there is none.
			var lead strings.Builder
			for _, sp := range p.Spans {
				switch {
				case sp.Timed:
					cue.Spans = append(cue.Spans, timedtext.Span{Start: sp.Begin, End: sp.End, Text: sp.Text})
				case len(cue.Spans) != 0:
					cue.Spans[len(cue.Spans)-1].Text += sp.Text
				default:
					lead.WriteString(sp.Text)
				}
			}
			if n := len(cue.Spans); n != 0 {
				cue.Spans[0].Text = lead.String() + cue.Spans[0].Text
				if cue.Start == 0 && cue.End == 0 {
					cue.Start, cue.End = cue.Spans[0].Start, cue.Spans[n-1].End
				}
			}
			t.Cues = append(t.Cues, cue)
		}
	}
	return t
}

// FromTrack converts a track from the format-neutral model into a single div. Cues without text are
// skipped, each distinct voice becomes a person agent, and the author becomes a songwriter.
func FromTrack(t *timedtext.Track) *Document {
	d := &Document{
		Lang:     t.Metadata.Language,
		Title:    t.Metadata.Title,
		Duration: t.Metadata.Duration,
	}
	if t.Metadata.Author != "" {
		d.Songwriters = []string{t.Metadata.Author}
	}
	agents := make(map[timedtext.Voice]string)
	var div Div
	for i, c := range t.Cues {
		if strings.TrimSpace(c.Text) == "" {
			continue
		}
		p := Paragraph{ID: c.ID, Begin: c.Start, End: t.CueEnd(i)}
		if c.Voice != "" {
			id, ok := agents[c.Voice]
			if !ok {
				id = fmt.Sprintf("v%d", len(agents)+1)
				agents[c.Voice] = id
				d.Agents = append(d.Agents, Agent{ID: id, Type: "person", Name: string(c.Voice)})
			}
			p.Agent = id
		}
		if len(c.Spans) == 0 {
			p.Spans = []Span{{Text: c.Text}}
		}
		for _, s := range c.Spans {
			// Whitespace around a span goes outside it, as a line break if it holds one.
			core := strings.TrimSpace(s.Text)
			if core == "" {
				continue
			}
			lead, _, _ := strings.Cut(s.Text, core)
			p.addSeparator(lead)
			p.Spans = append(p.Spans, Span{Timed: true, Begin: s.Start, End: s.End, Text: core})
			p.addSeparator(s.Text[len(lead)+len(core):])
		}
		if n := len(p.Spans); n != 0 && p.Spans[n-1].Text == " " {
			p.Spans = p.Spans[:n-1]
		}
		div.Paragraphs = append(div.Paragraphs, p)
	}
	if n := len(div.Paragraphs); n != 0 {
		div.Begin, div.End = div.Paragraphs[0].Begin, div.Paragraphs[n-1].End
		d.Divs = []Div{div}
	}
	return d
}

// addSeparator appends a space or line break span for whitespace between words. A line break replaces a
// space before it.
func (p *Paragraph) addSeparator(ws string) {
	if ws == "" || len(p.Spans) == 0 {
		return
	}
	last := &p.Spans[len(p.Spans)-1]
	switch {
	case strings.Contains(ws, "\n"):
		if last.Text == " " {
			last.Text = "\n"
		} else if last.Text != "\n" {
			p.Spans = append(p.Spans, Span{Text: "\n"})
		}
	case last.Text != " " && last.Text != "\n":
		p.Spans = append(p.Spans, Span{Text: " "})
	}
}
